package funk

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Loop control signals, propagated as errors and caught by the enclosing loop.
var (
	errBreak    = errors.New("break")
	errContinue = errors.New("continue")
)

// FnValue is a first-class function: an introspectable reference (an address),
// not an opaque closure. The agent can read what it is and improve it — the
// black box stays open (docs/01: the plan is data the agent reasons over).
type FnValue struct {
	Ref string `json:"fn"`
}

// secretSet holds the resolved values of `secret` needs so the trace can mask
// them. Self-observation (docs/01 #4) must not turn into credential exposure:
// the RunReport is data the agent — and anyone it shares the trace with — reads,
// so any secret that surfaces in it is redacted to "***". The function's actual
// return value is left intact (it is the result the caller asked for).
type secretSet struct{ vals []string }

func (s *secretSet) add(v string) {
	if s == nil || v == "" {
		return
	}
	for _, e := range s.vals {
		if e == v {
			return
		}
	}
	s.vals = append(s.vals, v)
}

// redact replaces every occurrence of a secret value inside x (walking strings,
// lists, and maps) with "***".
func (s *secretSet) redact(x interface{}) interface{} {
	if s == nil || len(s.vals) == 0 {
		return x
	}
	switch t := x.(type) {
	case string:
		for _, sec := range s.vals {
			t = strings.ReplaceAll(t, sec, "***")
		}
		return t
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, v := range t {
			out[i] = s.redact(v)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, v := range t {
			out[k] = s.redact(v)
		}
		return out
	default:
		return x
	}
}

func (s *secretSet) redactStr(x string) string {
	if v, ok := s.redact(x).(string); ok {
		return v
	}
	return x
}

// registerSecrets records the resolved values of a function's `secret` needs.
func registerSecrets(f *Fn, res map[string]map[string]interface{}, set *secretSet) {
	if set == nil || res == nil {
		return
	}
	for _, n := range f.Needs {
		if n.Kind != "secret" {
			continue
		}
		if kind, ok := res[n.Kind]; ok {
			if v, ok := kind[n.Alias].(string); ok {
				set.add(v)
			}
		}
	}
}

// Run executes a function by reference; a live stream result is drained to a List.
func Run(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts) ExecResult {
	v, cancel, err := evalTop(lib, ref, inputs, opts, nil)
	defer cancel()
	if err != nil {
		return ExecResult{Error: err.Error()}
	}
	if s, ok := v.(Stream); ok {
		v = drain(s)
	}
	return ExecResult{OK: true, Value: v}
}

// RunWithReport runs a function and returns a structured RunReport — the run as
// data (docs/01 property #4: dynamic self-observability). Tracing captures the
// eager evaluation (finite composites); lazy stream items are not traced.
func RunWithReport(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts) (ExecResult, *RunReport) {
	var events []TraceEvent
	v, cancel, err := evalTop(lib, ref, inputs, opts, &events)
	defer cancel()
	rep := &RunReport{Ref: ref, Events: events}
	if err != nil {
		rep.Error = err.Error()
		return ExecResult{Error: err.Error()}, rep
	}
	if s, ok := v.(Stream); ok {
		v = drain(s)
	}
	rep.Events = events
	rep.OK = true
	rep.Value = v
	return ExecResult{OK: true, Value: v}, rep
}

// RunStreaming executes a function and emits each stream item live (docs/03:
// a run may be a long-lived pipeline). Atomic/value results emit once.
func RunStreaming(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts, emit func(interface{})) ExecResult {
	v, cancel, err := evalTop(lib, ref, inputs, opts, nil)
	defer cancel()
	if err != nil {
		return ExecResult{Error: err.Error()}
	}
	if s, ok := v.(Stream); ok {
		for item := range s {
			emit(item)
		}
		return ExecResult{OK: true}
	}
	emit(v)
	return ExecResult{OK: true, Value: v}
}

// evalTop resolves and runs a function, returning the raw body value (which may
// be a live Stream) and a cancel function the caller must invoke when done.
func evalTop(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts, trace *[]TraceEvent) (interface{}, context.CancelFunc, error) {
	noop := func() {}
	f, ok := lib.Lookup(ref)
	if !ok {
		return nil, noop, fmt.Errorf("unknown function %q", ref)
	}
	if !f.Composite() {
		res := Exec(f, inputs, opts)
		if trace != nil {
			ev := TraceEvent{Fn: f.Name, Kind: "call", Value: res.Value, Error: res.Error}
			s := &secretSet{}
			registerSecrets(f, resolveResources(f, opts), s)
			ev.Value = s.redact(ev.Value)
			ev.Error = s.redactStr(ev.Error)
			*trace = append(*trace, ev)
		}
		if !res.OK {
			return nil, noop, fmt.Errorf("%s", res.Error)
		}
		return res.Value, noop, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	res := resolveResources(f, opts)
	var secrets *secretSet
	if trace != nil {
		secrets = &secretSet{}
		registerSecrets(f, res, secrets)
	}
	e := &evalEnv{lib: lib, opts: opts, vars: map[string]interface{}{}, ctx: ctx, cancel: cancel,
		resources: res, trace: trace, secrets: secrets}
	for k, v := range inputs {
		e.vars[k] = v
	}
	v, err := e.eval(f.Body)
	if err != nil {
		cancel()
		return nil, noop, err
	}
	return v, cancel, nil
}

type evalEnv struct {
	lib       *Library
	opts      ExecOpts
	vars      map[string]interface{}
	ctx       context.Context
	cancel    context.CancelFunc
	resources map[string]map[string]interface{} // needs: kind → alias → value
	trace     *[]TraceEvent                      // nil ⇒ no tracing
	secrets   *secretSet                         // resolved secret values, masked in the trace
	yieldTo   Stream                             // the enclosing each's output (for yield)
}

func (e *evalEnv) child() *evalEnv {
	c := &evalEnv{lib: e.lib, opts: e.opts, vars: map[string]interface{}{}, ctx: e.ctx, cancel: e.cancel, resources: e.resources, trace: e.trace, secrets: e.secrets, yieldTo: e.yieldTo}
	for k, v := range e.vars {
		c.vars[k] = v
	}
	return c
}

func (e *evalEnv) emit(ev TraceEvent) {
	if e.trace == nil {
		return
	}
	if e.secrets != nil {
		ev.Value = e.secrets.redact(ev.Value)
		ev.Error = e.secrets.redactStr(ev.Error)
		ev.Detail = e.secrets.redactStr(ev.Detail)
	}
	*e.trace = append(*e.trace, ev)
}

func (e *evalEnv) eval(n Node) (interface{}, error) {
	switch t := n.(type) {
	case Atom:
		return e.evalAtom(t)
	case Form:
		return e.evalForm(t)
	}
	return nil, fmt.Errorf("cannot evaluate node")
}

func (e *evalEnv) evalAtom(a Atom) (interface{}, error) {
	switch a.Kind {
	case "num":
		f, err := strconv.ParseFloat(a.Value, 64)
		if err != nil {
			return nil, err
		}
		return numFmt(f), nil
	case "str":
		return a.Value, nil
	case "id":
		switch a.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null", "nil":
			return nil, nil
		}
		// resource access: needs.<kind>.<alias> (docs/05)
		if strings.HasPrefix(a.Value, "needs.") {
			parts := strings.SplitN(a.Value, ".", 3)
			if len(parts) == 3 {
				if kind, ok := e.resources[parts[1]]; ok {
					if v, ok := kind[parts[2]]; ok {
						return v, nil
					}
				}
				return "", nil
			}
		}
		if v, ok := e.vars[a.Value]; ok {
			return v, nil
		}
		// a bare function name in value position → a first-class function value
		if _, ok := e.lib.Lookup(a.Value); ok {
			return FnValue{Ref: a.Value}, nil
		}
		return nil, fmt.Errorf("unknown identifier %q", a.Value)
	}
	return nil, fmt.Errorf("bad atom")
}

func (e *evalEnv) evalForm(f Form) (interface{}, error) {
	switch f.Head {
	case "do":
		return e.evalDo(f.Args)
	case "let":
		return e.evalLet(f.Args)
	case "if":
		return e.evalIf(f.Args)
	case "return":
		e.emit(TraceEvent{Kind: "terminal", Detail: "return"})
		if len(f.Args) == 0 {
			return nil, nil
		}
		return e.eval(f.Args[0])
	case "exit":
		e.emit(TraceEvent{Kind: "terminal", Detail: "exit"})
		if len(f.Args) == 0 {
			return nil, nil
		}
		return e.eval(f.Args[0]) // v1: exit yields its value (a terminal)
	case "for-each":
		return e.evalForEach(f.Args)
	case "while":
		return e.evalWhile(f.Args)
	case "on-error":
		return e.evalOnError(f.Args)
	case "retry":
		return e.evalRetry(f.Args)
	case "break":
		return nil, errBreak
	case "continue":
		return nil, errContinue
	case "window":
		return e.evalWindow(f.Args)
	case "range":
		return e.evalRange(f.Args)
	case "nats":
		return e.evalNats(f.Args)
	case "tick":
		return e.evalTick(f.Args)
	case "repeat":
		return e.evalRepeat(f.Args)
	case "take":
		return e.evalTake(f.Args)
	case "collect":
		return e.evalCollect(f.Args)
	case "fold":
		return e.evalFold(f.Args)
	case "each":
		return e.evalEach(f.Args)
	case "yield":
		return e.evalYield(f.Args)
	case "scan":
		return e.evalScan(f.Args)
	case "merge":
		return e.evalMerge(f.Args)
	default:
		return e.evalCall(f)
	}
}

func (e *evalEnv) evalDo(args []Node) (interface{}, error) {
	var last interface{}
	for _, a := range args {
		v, err := e.eval(a)
		if err != nil {
			return nil, err
		}
		last = v
	}
	return last, nil
}

// (let (name expr) body)
func (e *evalEnv) evalLet(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("let: expected (let (name expr) body)")
	}
	bind, ok := args[0].(Form)
	if !ok || len(bind.Args) != 1 {
		return nil, fmt.Errorf("let: first arg must be (name expr)")
	}
	val, err := e.eval(bind.Args[0])
	if err != nil {
		return nil, err
	}
	c := e.child()
	c.vars[bind.Head] = val
	return c.eval(args[1])
}

// (if cond then else)
func (e *evalEnv) evalIf(args []Node) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("if: expected (if cond then [else])")
	}
	cond, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	if truthy(cond) {
		e.emit(TraceEvent{Kind: "branch", Detail: "then"})
		return e.eval(args[1])
	}
	if len(args) >= 3 {
		e.emit(TraceEvent{Kind: "branch", Detail: "else"})
		return e.eval(args[2])
	}
	e.emit(TraceEvent{Kind: "branch", Detail: "else (empty)"})
	return nil, nil
}

// (for-each coll (item) body) — consume by effect; returns nil.
func (e *evalEnv) evalForEach(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("for-each: expected (for-each coll (item) body)")
	}
	coll, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	bind, ok := args[1].(Form)
	if !ok {
		return nil, fmt.Errorf("for-each: second arg must be (item)")
	}
	for _, item := range asList(coll) {
		c := e.child()
		c.vars[bind.Head] = item
		if _, err := c.eval(args[2]); err != nil {
			if errors.Is(err, errContinue) {
				continue
			}
			if errors.Is(err, errBreak) {
				break
			}
			return nil, err
		}
	}
	return nil, nil
}

// (while (s init) cond step) — stateful loop; s carries state across iterations.
func (e *evalEnv) evalWhile(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("while: (while (s init) cond step)")
	}
	bind, ok := args[0].(Form)
	if !ok || len(bind.Args) != 1 {
		return nil, fmt.Errorf("while: first arg must be (s init)")
	}
	val, err := e.eval(bind.Args[0])
	if err != nil {
		return nil, err
	}
	c := e.child()
	c.vars[bind.Head] = val
	for {
		cond, err := c.eval(args[1])
		if err != nil {
			return nil, err
		}
		if !truthy(cond) {
			break
		}
		nv, err := c.eval(args[2])
		if err != nil {
			if errors.Is(err, errContinue) {
				continue
			}
			if errors.Is(err, errBreak) {
				break
			}
			return nil, err
		}
		c.vars[bind.Head] = nv
	}
	return c.vars[bind.Head], nil
}

// (on-error <body> (e) <handler>) — run body; if it errors, bind the error
// message to e and run handler (a fallback / substitute value). Loop signals
// (break/continue) are not caught — they belong to the enclosing loop.
func (e *evalEnv) evalOnError(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("on-error: expected (on-error body (e) handler)")
	}
	v, err := e.eval(args[0])
	if err == nil {
		return v, nil
	}
	if errors.Is(err, errBreak) || errors.Is(err, errContinue) {
		return nil, err
	}
	bind, ok := args[1].(Form)
	if !ok || bind.Head == "" {
		return nil, fmt.Errorf("on-error: second arg must be (errName)")
	}
	e.emit(TraceEvent{Kind: "recover", Detail: "on-error", Error: err.Error()})
	c := e.child()
	c.vars[bind.Head] = err.Error()
	return c.eval(args[2])
}

// (retry <body> <n> [backoff <dur>]) — re-run body up to n attempts; optional
// backoff waits between attempts (cancellable). Returns the last error if every
// attempt fails; loop signals propagate immediately.
func (e *evalEnv) evalRetry(args []Node) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("retry: expected (retry body n [backoff dur])")
	}
	nv, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	n, err := toNum(nv)
	if err != nil {
		return nil, fmt.Errorf("retry: n must be a number: %s", err)
	}
	attempts := int(n)
	if attempts < 1 {
		attempts = 1
	}
	var backoff time.Duration
	for i := 2; i+1 < len(args); i++ {
		if kw, ok := args[i].(Atom); ok && kw.Value == "backoff" {
			if d, ok := args[i+1].(Atom); ok {
				if sec, ok := parseDuration(d.Value); ok {
					backoff = time.Duration(sec * float64(time.Second))
				}
			}
		}
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		v, err := e.eval(args[0])
		if err == nil {
			return v, nil
		}
		if errors.Is(err, errBreak) || errors.Is(err, errContinue) {
			return nil, err
		}
		lastErr = err
		e.emit(TraceEvent{Kind: "retry", Detail: fmt.Sprintf("attempt %d/%d", attempt, attempts), Error: err.Error()})
		if attempt < attempts && backoff > 0 {
			select {
			case <-e.ctx.Done():
				return nil, e.ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, lastErr
}

// (window stream size [every slide] [by field] …) — windowing (docs/04 §7).
//   - size a duration (5s) → event-time tumbling window (by <field>, default "time")
//   - size a number, stream input → tumbling count window (a stream of Lists)
//   - size a number, list input  → last `size` items (a scoped collect)
func (e *evalEnv) evalWindow(args []Node) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("window: expected (window stream size …)")
	}
	coll, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	// event-time window: size is a duration atom like `5s`.
	if a, ok := args[1].(Atom); ok {
		if dur, ok := parseDuration(a.Value); ok {
			field := "time"
			for i := 2; i+1 < len(args); i++ {
				if kw, ok := args[i].(Atom); ok && kw.Value == "by" {
					if fld, ok := args[i+1].(Atom); ok {
						field = fld.Value
					}
				}
			}
			return e.timeWindow(e.asStream(coll), dur, field), nil
		}
	}
	size, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	n, err := toNum(size)
	if err != nil {
		return nil, err
	}
	if s, ok := coll.(Stream); ok {
		if slide, ok := e.windowEvery(args); ok {
			return e.slideCountWindow(s, int(n), slide), nil
		}
		return e.countWindow(s, int(n)), nil
	}
	l := asList(coll)
	if k := int(n); k >= 0 && k < len(l) {
		l = l[len(l)-k:]
	}
	return l, nil
}

// windowEvery scans a window form's args for `every <slide>` (a sliding count
// window). Event-time sliding is not yet wired — see docs/04 §6a.
func (e *evalEnv) windowEvery(args []Node) (int, bool) {
	for i := 2; i+1 < len(args); i++ {
		if kw, ok := args[i].(Atom); ok && kw.Value == "every" {
			if v, err := e.eval(args[i+1]); err == nil {
				if f, err := toNum(v); err == nil {
					return int(f), true
				}
			}
		}
	}
	return 0, false
}

// A plain call: (fn arg…). Args map positionally to the callee's `in` ports.
func (e *evalEnv) evalCall(f Form) (interface{}, error) {
	// the head may be a function-valued variable (a first-class function passed in)
	ref := f.Head
	if v, ok := e.vars[f.Head]; ok {
		fv, ok := v.(FnValue)
		if !ok {
			return nil, fmt.Errorf("%q is not callable", f.Head)
		}
		ref = fv.Ref
	}
	callee, ok := e.lib.Lookup(ref)
	if !ok {
		return nil, fmt.Errorf("unknown function %q", ref)
	}
	inputs := map[string]interface{}{}
	for i, arg := range f.Args {
		v, err := e.eval(arg)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("_%d", i)
		if i < len(callee.In) {
			name = callee.In[i].Name
		}
		inputs[name] = v
	}
	// Composite calls run INLINE — sharing this scope's context (so cancellation
	// propagates through funk-defined stream operators) and returning a live
	// stream (not drained). Atomic calls execute directly.
	if callee.Composite() {
		v, err := e.runInline(callee, inputs)
		if err != nil {
			e.emit(TraceEvent{Fn: ref, Kind: "call", Error: err.Error()})
			return nil, fmt.Errorf("%s: %s", ref, err.Error())
		}
		e.emit(TraceEvent{Fn: ref, Kind: "call", Value: v})
		return v, nil
	}
	res := Exec(callee, inputs, e.opts)
	if !res.OK {
		e.emit(TraceEvent{Fn: ref, Kind: "call", Error: res.Error})
		return nil, fmt.Errorf("%s: %s", ref, res.Error)
	}
	e.emit(TraceEvent{Fn: ref, Kind: "call", Value: res.Value})
	return res.Value, nil
}

// runInline evaluates a composite in a child scope that SHARES this env's
// context, cancel, and trace (isolating only variables to the callee's inputs).
func (e *evalEnv) runInline(f *Fn, inputs map[string]interface{}) (interface{}, error) {
	res := resolveResources(f, e.opts)
	registerSecrets(f, res, e.secrets)
	c := &evalEnv{lib: e.lib, opts: e.opts, vars: map[string]interface{}{},
		ctx: e.ctx, cancel: e.cancel, resources: res, trace: e.trace, secrets: e.secrets}
	for k, v := range inputs {
		c.vars[k] = v
	}
	return c.eval(f.Body)
}

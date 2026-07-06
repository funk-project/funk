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
	v, cancel, err := evalTop(lib, ref, inputs, opts, nil, nil)
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
	v, cancel, err := evalTop(lib, ref, inputs, opts, &events, nil)
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
	v, cancel, err := evalTop(lib, ref, inputs, opts, nil, nil)
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

// RunLive runs a function and streams trace events (per node — including "enter"
// glow signals that fire *before* a node executes) and stream values as they
// happen, then returns the final RunReport. It powers funkd's animated,
// self-observable trace (docs/01 #4): the plan lights up *as* it runs, not only
// after. The "enter" events are live-only — they are not recorded in the
// RunReport, so the batch report keeps its exact shape.
func RunLive(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts,
	onEvent func(TraceEvent), onValue func(interface{})) (ExecResult, *RunReport) {
	var events []TraceEvent
	var sink func(TraceEvent)
	if onEvent != nil {
		sink = onEvent
	}
	v, cancel, err := evalTop(lib, ref, inputs, opts, &events, sink)
	defer cancel()
	rep := &RunReport{Ref: ref, Events: events}
	if err != nil {
		rep.Error = err.Error()
		return ExecResult{Error: err.Error()}, rep
	}
	if s, ok := v.(Stream); ok {
		for item := range s {
			if onValue != nil {
				onValue(item)
			}
		}
		rep.Events = events
		rep.OK = true
		return ExecResult{OK: true}, rep
	}
	if onValue != nil {
		onValue(v)
	}
	rep.Events = events
	rep.OK = true
	rep.Value = v
	return ExecResult{OK: true, Value: v}, rep
}

// evalTop resolves and runs a function, returning the raw body value (which may
// be a live Stream) and a cancel function the caller must invoke when done.
// A non-nil sink receives every trace event live, as it is produced (funkd's
// animated trace); trace, if non-nil, additionally accumulates them for the
// batch RunReport.
func evalTop(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts, trace *[]TraceEvent, sink func(TraceEvent)) (interface{}, context.CancelFunc, error) {
	noop := func() {}
	f, ok := lib.Lookup(ref)
	if !ok {
		return nil, noop, fmt.Errorf("unknown function %q", ref)
	}
	// fill defaults for optional entry ports, then validate the specs (docs/07 §2.2)
	fillEntryDefaults(f, inputs)
	if err := validateInputs(f, inputs); err != nil {
		return nil, noop, err
	}
	// Start the per-run broker if the function has brokered integrations
	// (docs/06): the body reaches them via FUNK_BROKER, never holding the token.
	stopBroker := func() {}
	if cfg, ok := brokerConfigFor(lib, f, opts); ok {
		if base, stop, err := startBroker(cfg); err == nil {
			opts.BrokerURL = base
			stopBroker = stop
		}
	}
	if !f.Composite() {
		if sink != nil {
			sink(TraceEvent{Fn: f.Name, Kind: "enter"})
		}
		res := Exec(f, inputs, opts)
		defer stopBroker()
		if trace != nil || sink != nil {
			ev := TraceEvent{Fn: f.Name, Kind: "call", Value: res.Value, Error: res.Error}
			s := &secretSet{}
			registerSecrets(f, resolveResources(f, opts), s)
			ev.Value = s.redact(ev.Value)
			ev.Error = s.redactStr(ev.Error)
			if trace != nil {
				*trace = append(*trace, ev)
			}
			if sink != nil {
				sink(ev)
			}
		}
		if !res.OK {
			return nil, noop, fmt.Errorf("%s", res.Error)
		}
		return res.Value, noop, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	res := resolveResources(f, opts)
	var secrets *secretSet
	if trace != nil || sink != nil {
		secrets = &secretSet{}
		registerSecrets(f, res, secrets)
	}
	frame := &outFrame{staged: map[string]interface{}{}, outs: f.Out}
	e := &evalEnv{lib: lib, opts: opts, vars: map[string]interface{}{}, ctx: ctx, cancel: cancel,
		resources: res, trace: trace, secrets: secrets, sink: sink, out: frame, curFn: f}
	frame.e = e
	for k, v := range inputs {
		e.vars[k] = v
	}
	v, err := e.eval(f.Body)
	if err != nil {
		cancel()
		stopBroker()
		return nil, noop, err
	}
	// Derive the returned value from flushes (docs/07 §3): 1 ⇒ that tuple, N ⇒ a
	// finite stream; 0 ⇒ the body value (a legacy `return`, or a stream a body
	// operator produced directly).
	switch len(frame.emitted) {
	case 1:
		v = frame.emitted[0]
	default:
		if len(frame.emitted) > 1 {
			v = e.asStream(frame.emitted)
		}
	}
	// the caller runs cancel when the (possibly live) stream is done — stop the
	// broker then too.
	return v, func() { cancel(); stopBroker() }, nil
}

type evalEnv struct {
	lib       *Library
	opts      ExecOpts
	vars      map[string]interface{}
	ctx       context.Context
	cancel    context.CancelFunc
	resources map[string]map[string]interface{} // needs: kind → alias → value
	trace     *[]TraceEvent                     // nil ⇒ not accumulating the batch report
	sink      func(TraceEvent)                  // nil ⇒ no live delivery (funkd animated trace)
	secrets   *secretSet                        // resolved secret values, masked in the trace
	yieldTo   Stream                            // the enclosing each's output (for yield, legacy)
	out       *outFrame                         // the current body's named-output frame (set/flush)
	curFn     *Fn                               // the function whose body is running (scopes `use … as` resolution)
}

// outFrame is a function body's output staging (docs/07 §3). `set` stages a named
// output; `flush` emits a tuple of the staged values — either appended to a finite
// list (scalar call) or sent live to a stream (reactive call). Staged values
// persist across flushes so a source can vary only some ports between emits.
type outFrame struct {
	staged  map[string]interface{}
	outs    []Port        // the function's out-ports (for unwrapping)
	live    Stream        // non-nil ⇒ flush sends here; nil ⇒ collect in emitted
	e       *evalEnv      // for cancellable send
	emitted []interface{} // finite collection (scalar/finite call)
}

// unwrap turns the staged map into the emitted value: a single out-port yields its
// scalar; multiple out-ports yield the map (destructured by `let` at the caller).
func (o *outFrame) unwrap() interface{} {
	if len(o.outs) == 1 {
		return o.staged[o.outs[0].Name]
	}
	m := make(map[string]interface{}, len(o.outs))
	for _, p := range o.outs {
		m[p.Name] = o.staged[p.Name]
	}
	return m
}

// doFlush emits the current staged tuple; returns false if a live send was
// cancelled (scope done).
func (o *outFrame) doFlush() bool {
	v := o.unwrap()
	if o.live != nil {
		return o.e.send(o.live, v)
	}
	o.emitted = append(o.emitted, v)
	return true
}

func (e *evalEnv) child() *evalEnv {
	c := &evalEnv{lib: e.lib, opts: e.opts, vars: map[string]interface{}{}, ctx: e.ctx, cancel: e.cancel, resources: e.resources, trace: e.trace, sink: e.sink, secrets: e.secrets, yieldTo: e.yieldTo, out: e.out, curFn: e.curFn}
	for k, v := range e.vars {
		c.vars[k] = v
	}
	return c
}

func (e *evalEnv) emit(ev TraceEvent) {
	if e.trace == nil && e.sink == nil {
		return
	}
	if e.secrets != nil {
		ev.Value = e.secrets.redact(ev.Value)
		ev.Error = e.secrets.redactStr(ev.Error)
		ev.Detail = e.secrets.redactStr(ev.Detail)
	}
	if e.trace != nil {
		*e.trace = append(*e.trace, ev)
	}
	if e.sink != nil {
		e.sink(ev)
	}
}

// live delivers an event to the live sink only (never the batch RunReport) —
// for animation-only signals like a node's "enter" glow.
func (e *evalEnv) live(ev TraceEvent) {
	if e.sink != nil {
		e.sink(ev)
	}
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
		// a function name in value position → a first-class function value. Resolve
		// through the current scope (so `alias.fn` works and hidden bare names fail),
		// but store the fully-qualified address so it invokes from any scope.
		if fn, ok := e.lib.ResolveIn(e.curFn, a.Value); ok {
			return FnValue{Ref: fn.Address()}, nil
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
		return e.evalIf(f)
	case "return":
		// Removed (docs/07 §3): functions emit named outputs via flush, not return.
		return nil, fmt.Errorf("`return` has been removed — emit a named output with (flush (%s value)) (docs/07 §3)", firstOutName(e))
	case "exit":
		if len(f.Args) == 0 {
			e.emit(TraceEvent{Kind: "terminal", Detail: "exit", Node: f.Pos.String()})
			return nil, nil
		}
		v, err := e.eval(f.Args[0]) // v1: exit yields its value (a terminal)
		if err == nil {
			e.emit(TraceEvent{Kind: "terminal", Detail: "exit", Node: f.Pos.String()})
		}
		return v, err
	case "set":
		return e.evalSet(f.Args)
	case "flush":
		return e.evalFlush(f)
	case "for-each":
		return e.evalForEach(f.Args)
	case "while":
		return e.evalWhile(f.Args)
	case "on-error":
		return e.evalOnError(f.Args)
	case "retry":
		return e.evalRetry(f.Args)
	case "with":
		return e.evalWith(f)
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

// (let (name expr) body) — bind a name; or (let (n1 n2 … expr) body) to
// destructure a multi-output call's named outputs positionally (docs/07 §4).
func (e *evalEnv) evalLet(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("let: expected (let (name… expr) body)")
	}
	bind, ok := args[0].(Form)
	if !ok || len(bind.Args) < 1 {
		return nil, fmt.Errorf("let: first arg must be (name… expr)")
	}
	// names = the head + every arg except the last; the last arg is the expression.
	names := []string{bind.Head}
	for i := 0; i < len(bind.Args)-1; i++ {
		a, ok := bind.Args[i].(Atom)
		if !ok {
			return nil, fmt.Errorf("let: binding names must be identifiers")
		}
		names = append(names, a.Value)
	}
	val, err := e.eval(bind.Args[len(bind.Args)-1])
	if err != nil {
		return nil, err
	}
	c := e.child()
	if len(names) == 1 {
		c.vars[names[0]] = val
	} else {
		m, ok := val.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("let: cannot destructure %d names from a single value (call must have named outputs)", len(names))
		}
		for _, n := range names {
			c.vars[n] = m[n]
		}
	}
	return c.eval(args[1])
}

// fillEntryDefaults injects literal defaults for optional entry ports the caller
// omitted (docs/07 §2.3). Entry inputs bypass bindInputs, so an omitted optional
// like `(y Num (default 1))` would otherwise be an unbound identifier.
func fillEntryDefaults(f *Fn, inputs map[string]interface{}) {
	for _, p := range f.In {
		if _, ok := inputs[p.Name]; ok || !p.Optional || p.Default == nil {
			continue
		}
		a, ok := p.Default.(Atom)
		if !ok {
			continue
		}
		switch a.Kind {
		case "num":
			if v, err := strconv.ParseFloat(a.Value, 64); err == nil {
				inputs[p.Name] = numFmt(v)
			}
		case "str":
			inputs[p.Name] = a.Value
		case "id":
			switch a.Value {
			case "true":
				inputs[p.Name] = true
			case "false":
				inputs[p.Name] = false
			}
		}
	}
}

// firstOutName is the first output port name of the current frame (for a helpful
// message when someone writes the removed `return`); "r" when unknown.
func firstOutName(e *evalEnv) string {
	if e.out != nil && len(e.out.outs) > 0 {
		return e.out.outs[0].Name
	}
	return "r"
}

// (set name expr) — stage a named output in the current frame (docs/07 §3).
func (e *evalEnv) evalSet(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("set: expected (set name expr)")
	}
	name, ok := args[0].(Atom)
	if !ok {
		return nil, fmt.Errorf("set: first arg must be an output name")
	}
	v, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	if e.out == nil {
		return nil, fmt.Errorf("set %q: no output frame (set is only valid inside a function body)", name.Value)
	}
	e.out.staged[name.Value] = v
	return nil, nil
}

// (flush) — emit a tuple of the staged outputs; (flush (r v) (q w) …) stages
// those ports first, then emits (docs/07 §3). Replaces return/yield.
func (e *evalEnv) evalFlush(f Form) (interface{}, error) {
	if e.out == nil {
		return nil, fmt.Errorf("flush: no output frame (flush is only valid inside a function body)")
	}
	for _, a := range f.Args {
		pair, ok := a.(Form)
		if !ok || pair.Head == "" || len(pair.Args) != 1 {
			return nil, fmt.Errorf("flush: each arg must be (port value)")
		}
		v, err := e.eval(pair.Args[0])
		if err != nil {
			return nil, err
		}
		e.out.staged[pair.Head] = v
	}
	// enforce output specs (post-conditions, docs/07 §3) before emitting
	if err := validatePorts(e.out.outs, e.out.staged, "output"); err != nil {
		return nil, err
	}
	if !e.out.doFlush() {
		return nil, e.ctx.Err()
	}
	e.emit(TraceEvent{Kind: "terminal", Detail: "flush", Node: f.Pos.String()})
	return nil, nil
}

// (if cond then else)
func (e *evalEnv) evalIf(f Form) (interface{}, error) {
	args := f.Args
	id := f.Pos.String()
	if len(args) < 2 {
		return nil, fmt.Errorf("if: expected (if cond then [else])")
	}
	cond, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	if truthy(cond) {
		e.emit(TraceEvent{Kind: "branch", Detail: "then", Node: id})
		return e.eval(args[1])
	}
	if len(args) >= 3 {
		e.emit(TraceEvent{Kind: "branch", Detail: "else", Node: id})
		return e.eval(args[2])
	}
	e.emit(TraceEvent{Kind: "branch", Detail: "else (empty)", Node: id})
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

// (with (kind.alias value)… body) — run body with extra/overridden resource
// bindings (docs/05: per-call binding). Each binding is a `(kind.alias value)`
// form; the last arg is the body evaluated with those bindings merged over the
// current ones.
func (e *evalEnv) evalWith(f Form) (interface{}, error) {
	if len(f.Args) < 1 {
		return nil, fmt.Errorf("with: (with (kind.alias value)… body)")
	}
	binds, body := f.Args[:len(f.Args)-1], f.Args[len(f.Args)-1]
	merged := map[string]string{}
	for k, v := range e.opts.Bindings {
		merged[k] = v
	}
	for _, b := range binds {
		pair, ok := b.(Form)
		if !ok || pair.Head == "" || len(pair.Args) != 1 {
			return nil, fmt.Errorf("with: each binding must be (kind.alias value)")
		}
		v, err := e.eval(pair.Args[0])
		if err != nil {
			return nil, err
		}
		merged[pair.Head] = fmt.Sprint(v)
	}
	c := e.child()
	c.opts = e.opts
	c.opts.Bindings = merged
	return c.eval(body)
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
			var slide, lateness float64
			for i := 2; i+1 < len(args); i++ {
				kw, ok := args[i].(Atom)
				if !ok {
					continue
				}
				switch kw.Value {
				case "by":
					if fld, ok := args[i+1].(Atom); ok {
						field = fld.Value
					}
				case "every":
					if d, ok := args[i+1].(Atom); ok {
						if s, ok := parseDuration(d.Value); ok {
							slide = s
						}
					}
				case "lateness":
					if d, ok := args[i+1].(Atom); ok {
						if l, ok := parseDuration(d.Value); ok {
							lateness = l
						}
					}
				}
			}
			if slide > 0 { // sliding event-time window
				return e.slideTimeWindow(e.asStream(coll), dur, slide, lateness, field), nil
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
	callee, ok := e.lib.ResolveIn(e.curFn, ref)
	if !ok {
		return nil, fmt.Errorf("unknown function %q", ref)
	}
	// node id = this call site's position (matches `funk graph --json`), so the
	// IDE maps the event to the exact node even under repeated calls.
	id := f.Pos.String()
	// glow: signal the node is about to run, before its inputs resolve.
	e.live(TraceEvent{Fn: ref, Kind: "enter", Node: id})
	vals := make([]interface{}, len(f.Args))
	for i, arg := range f.Args {
		v, err := e.eval(arg)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return e.invoke(callee, ref, id, vals)
}

// invoke dispatches a resolved call. If any argument is a live Stream bound to a
// *scalar* input port, the call fires **per item** (docs/07 §5 — the function
// wakes on each input). Otherwise it runs once (the scalar/length-1 case, which
// includes passing a whole Stream to a Stream-typed port, e.g. map/window).
func (e *evalEnv) invoke(callee *Fn, ref, id string, vals []interface{}) (interface{}, error) {
	driving := false
	for i, v := range vals {
		if _, ok := v.(Stream); ok && scalarPort(callee, i) {
			driving = true
			break
		}
	}
	if driving {
		return e.callReactive(callee, ref, id, vals)
	}
	inputs, err := e.bindInputs(callee, vals)
	if err != nil {
		e.emit(TraceEvent{Fn: ref, Kind: "call", Error: err.Error(), Node: id})
		return nil, fmt.Errorf("%s: %s", ref, err)
	}
	return e.callOnce(callee, ref, id, inputs)
}

// scalarPort reports whether the callee's i-th input port expects a single scalar
// element (Num/Str/Bool/Time/Bytes) — the case where a Stream argument drives
// per-item firing (the reactive lift, docs/07 §5). Stream/List/Json/Any and
// untyped ports consume the whole argument, so a Stream flows in as-is.
func scalarPort(callee *Fn, i int) bool {
	if i >= len(callee.In) {
		return false
	}
	switch callee.In[i].Type {
	case "Num", "Str", "Bool", "Time", "Bytes":
		return true
	}
	return false
}

// bindInputs maps positional args to port names, fills defaults for optional
// ports not supplied, and validates the per-port specs (docs/07 §2.2).
func (e *evalEnv) bindInputs(callee *Fn, vals []interface{}) (map[string]interface{}, error) {
	inputs := map[string]interface{}{}
	for i, v := range vals {
		name, ptype := fmt.Sprintf("_%d", i), ""
		if i < len(callee.In) {
			name, ptype = callee.In[i].Name, callee.In[i].Type
		}
		// A stream flowing into a non-Stream port (List / Json / scalar / Any) is
		// materialized once here, so it serializes to an atomic engine and can be
		// read more than once inside the body (a bare stream is single-use).
		if s, ok := v.(Stream); ok && !strings.HasPrefix(ptype, "Stream") {
			v = drain(s)
		}
		inputs[name] = v
	}
	e.fillDefaults(callee, inputs)
	if err := validateInputs(callee, inputs); err != nil {
		return nil, err
	}
	return inputs, nil
}

// callOnce runs the body once for a materialized (scalar) input set and returns
// its value (derived from flushes, or a legacy `return`).
func (e *evalEnv) callOnce(callee *Fn, ref, id string, inputs map[string]interface{}) (interface{}, error) {
	if callee.Composite() {
		v, err := e.runComposite(callee, inputs, nil)
		if err != nil {
			e.emit(TraceEvent{Fn: ref, Kind: "call", Error: err.Error(), Node: id})
			return nil, fmt.Errorf("%s: %s", ref, err.Error())
		}
		e.emit(TraceEvent{Fn: ref, Kind: "call", Value: traceVal(v), Node: id})
		return v, nil
	}
	res := Exec(callee, inputs, e.opts)
	if !res.OK {
		e.emit(TraceEvent{Fn: ref, Kind: "call", Error: res.Error, Node: id})
		return nil, fmt.Errorf("%s: %s", ref, res.Error)
	}
	e.emit(TraceEvent{Fn: ref, Kind: "call", Value: res.Value, Node: id})
	return res.Value, nil
}

// runComposite evaluates a composite body in a child scope that SHARES this env's
// context/cancel/trace (isolating variables to the callee's inputs) but gets its
// OWN output frame. With live != nil, flushes stream there (reactive per-item)
// and it returns nil; otherwise the value is derived from the collected flushes:
// 0 ⇒ the body value (legacy return / sink), 1 ⇒ that tuple, N ⇒ a finite stream.
func (e *evalEnv) runComposite(f *Fn, inputs map[string]interface{}, live Stream) (interface{}, error) {
	res := resolveResources(f, e.opts)
	registerSecrets(f, res, e.secrets)
	frame := &outFrame{staged: map[string]interface{}{}, outs: f.Out, live: live}
	c := &evalEnv{lib: e.lib, opts: e.opts, vars: map[string]interface{}{},
		ctx: e.ctx, cancel: e.cancel, resources: res, trace: e.trace, sink: e.sink, secrets: e.secrets, out: frame, curFn: f}
	frame.e = c
	for k, v := range inputs {
		c.vars[k] = v
	}
	bodyVal, err := c.eval(f.Body)
	if err != nil {
		return nil, err
	}
	if live != nil {
		if len(frame.emitted) == 0 && bodyVal != nil { // legacy return, no flush
			c.send(live, bodyVal)
		}
		return nil, nil
	}
	switch len(frame.emitted) {
	case 0:
		return bodyVal, nil
	case 1:
		return frame.emitted[0], nil
	default:
		return e.asStream(frame.emitted), nil
	}
}

// traceVal keeps a live Stream out of the trace (a channel is not JSON-encodable).
func traceVal(v interface{}) interface{} {
	if _, ok := v.(Stream); ok {
		return nil
	}
	return v
}

// validateInputs enforces (min n)/(max n) refinements on numeric input ports
// (docs/07 §2.2): a violation cancels the scope (propagate-and-cancel).
func validateInputs(callee *Fn, inputs map[string]interface{}) error {
	return validatePorts(callee.In, inputs, "input")
}

// validatePorts checks (min)/(max) on the given ports against the supplied values
// — used for both input ports (on arrival) and output ports (post-conditions on
// flush). Non-numeric / absent values are skipped.
func validatePorts(ports []Port, vals map[string]interface{}, what string) error {
	for _, p := range ports {
		v, ok := vals[p.Name]
		if !ok || (p.Min == nil && p.Max == nil) {
			continue
		}
		n, err := toNum(v)
		if err != nil {
			continue
		}
		if p.Min != nil && n < *p.Min {
			return fmt.Errorf("%s %q = %v is below min %v", what, p.Name, n, *p.Min)
		}
		if p.Max != nil && n > *p.Max {
			return fmt.Errorf("%s %q = %v is above max %v", what, p.Name, n, *p.Max)
		}
	}
	return nil
}

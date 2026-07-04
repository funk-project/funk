package funk

import (
	"fmt"
	"strings"
)

// Issue is a static problem found by Check. Warn marks a non-fatal advisory
// (a security caution) — it is reported but does not fail the check.
type Issue struct {
	Fn   string
	Msg  string
	File string // source file, if known
	Pos  Pos    // position of the offending node, if known
	Warn bool   // advisory, not an error
}

// String formats as "path:line:col: fn: message" when a location is known (a
// File Watcher / LSP can navigate to it), degrading gracefully otherwise.
func (i Issue) String() string {
	switch {
	case i.File != "" && i.Pos.Line > 0:
		return fmt.Sprintf("%s:%d:%d: %s: %s", i.File, i.Pos.Line, i.Pos.Col, i.Fn, i.Msg)
	case i.Pos.Line > 0:
		return fmt.Sprintf("%d:%d: %s: %s", i.Pos.Line, i.Pos.Col, i.Fn, i.Msg)
	default:
		return fmt.Sprintf("%s: %s", i.Fn, i.Msg)
	}
}

var coreForms = map[string]bool{
	"do": true, "let": true, "if": true, "exit": true,
	"set": true, "flush": true,
	"for-each": true, "while": true, "window": true, "break": true, "continue": true,
	"range": true, "nats": true, "repeat": true,
	"take": true, "collect": true, "tick": true, "scan": true, "merge": true,
	"each": true, "yield": true, "fold": true,
	"on-error": true, "retry": true,
}

// Check statically validates every composite function in the library:
// unresolved calls, arity mismatches, and unknown identifiers.
func Check(lib *Library) []Issue {
	var issues []Issue
	for _, f := range lib.Fns {
		// Security advisory (docs/06 §6): a body that holds a raw secret AND has a
		// declared network egress could exfiltrate it — redaction is not a guarantee.
		if secretWithNet(lib, f) {
			issues = append(issues, Issue{Fn: f.Name, File: f.File, Pos: f.Pos, Warn: true,
				Msg: "raw secret + network egress — the body could exfiltrate the secret (docs/06 §6); prefer a brokered integration, or drop the net effect"})
		}
		if !f.Composite() {
			continue
		}
		scope := map[string]bool{}
		for _, p := range f.In {
			scope[p.Name] = true
		}
		issues = append(issues, checkNode(lib, f, f.Body, scope)...)
		issues = append(issues, checkTypes(lib, f)...)
	}
	return issues
}

// secretWithNet reports whether f itself holds a raw `secret` need and can reach
// the network (a `net` effect anywhere in what it transitively calls).
func secretWithNet(lib *Library, f *Fn) bool {
	ownSecret := false
	for _, n := range f.Needs {
		if n.Kind == "secret" {
			ownSecret = true
		}
	}
	if !ownSecret {
		return false
	}
	_, effects := aggregateResources(lib, f, map[string]bool{})
	for _, e := range effects {
		if e.Kind == "net" {
			return true
		}
	}
	return false
}

// issueAt builds an Issue located at node n's position, within fn.
func issueAt(fn *Fn, p Pos, msg string) Issue {
	// fall back to the fn's own position if the node lost its location
	if p.Line == 0 {
		p = fn.Pos
	}
	return Issue{Fn: fn.Name, File: fn.File, Pos: p, Msg: msg}
}

func checkNode(lib *Library, fn *Fn, n Node, scope map[string]bool) []Issue {
	switch t := n.(type) {
	case Atom:
		if t.Kind == "id" {
			switch {
			case t.Value == "true", t.Value == "false", t.Value == "null", t.Value == "nil":
			case strings.HasPrefix(t.Value, "needs."): // resource access (docs/05)
			case scope[t.Value]: // a bound variable
			default:
				// otherwise it must name a function (a first-class function value)
				if _, ok := lib.Lookup(t.Value); !ok {
					return []Issue{issueAt(fn, t.Pos, fmt.Sprintf("unknown identifier %q", t.Value))}
				}
			}
		}
		return nil
	case Form:
		return checkForm(lib, fn, t, scope)
	}
	return nil
}

func checkForm(lib *Library, fn *Fn, f Form, scope map[string]bool) []Issue {
	var issues []Issue
	child := func() map[string]bool {
		c := map[string]bool{}
		for k := range scope {
			c[k] = true
		}
		return c
	}
	switch f.Head {
	case "return":
		// Removed (docs/07 §3) — flag it clearly for anyone migrating.
		issues = append(issues, issueAt(fn, f.Pos, "`return` has been removed — emit a named output with (flush (port value)) (docs/07 §3)"))
		for _, a := range f.Args {
			issues = append(issues, checkNode(lib, fn, a, scope)...)
		}
	case "do", "exit", "if", "break", "continue",
		"range", "nats", "repeat", "take", "collect", "merge", "yield":
		for _, a := range f.Args {
			issues = append(issues, checkNode(lib, fn, a, scope)...)
		}
	case "each":
		// (each stream (item) body) — binds item in the body scope
		if len(f.Args) == 3 {
			issues = append(issues, checkNode(lib, fn, f.Args[0], scope)...)
			sc := child()
			if bind, ok := f.Args[1].(Form); ok {
				sc[bind.Head] = true
			}
			issues = append(issues, checkNode(lib, fn, f.Args[2], sc)...)
		}
	case "scan", "fold":
		// (scan|fold stream fn init)
		if len(f.Args) == 3 {
			issues = append(issues, checkNode(lib, fn, f.Args[0], scope)...)
			if name, ok := fnRefName(f.Args[1]); ok {
				if _, ok := lib.Lookup(name); !ok {
					issues = append(issues, issueAt(fn, f.Pos, fmt.Sprintf("scan references unknown function %q", name)))
				}
			}
			issues = append(issues, checkNode(lib, fn, f.Args[2], scope)...)
		}
	case "window":
		// (window stream size [every s] [by field] …) — only the stream is checked;
		// size/by/field are literals (durations, keywords).
		if len(f.Args) >= 1 {
			issues = append(issues, checkNode(lib, fn, f.Args[0], scope)...)
		}
	case "tick":
		// (tick <duration>) — a duration literal, nothing to check.
	case "let":
		// (let (name… expr) body) — bind the head + all-but-last args as names; the
		// last arg is the expression (a multi-output call, when destructuring).
		if len(f.Args) == 2 {
			if bind, ok := f.Args[0].(Form); ok && len(bind.Args) >= 1 {
				issues = append(issues, checkNode(lib, fn, bind.Args[len(bind.Args)-1], scope)...)
				sc := child()
				sc[bind.Head] = true
				for i := 0; i < len(bind.Args)-1; i++ {
					if a, ok := bind.Args[i].(Atom); ok {
						sc[a.Value] = true
					}
				}
				issues = append(issues, checkNode(lib, fn, f.Args[1], sc)...)
			}
		}
	case "set":
		// (set name expr) — name is an output port; only the expr is checked.
		if len(f.Args) == 2 {
			issues = append(issues, checkNode(lib, fn, f.Args[1], scope)...)
		}
	case "flush":
		// (flush (port value)…) — heads are output names, not calls; check values.
		for _, a := range f.Args {
			if pair, ok := a.(Form); ok && len(pair.Args) == 1 {
				issues = append(issues, checkNode(lib, fn, pair.Args[0], scope)...)
			}
		}
	case "for-each":
		if len(f.Args) == 3 {
			issues = append(issues, checkNode(lib, fn, f.Args[0], scope)...)
			sc := child()
			if bind, ok := f.Args[1].(Form); ok {
				sc[bind.Head] = true
			}
			issues = append(issues, checkNode(lib, fn, f.Args[2], sc)...)
		}
	case "while":
		if len(f.Args) == 3 {
			if bind, ok := f.Args[0].(Form); ok && len(bind.Args) == 1 {
				issues = append(issues, checkNode(lib, fn, bind.Args[0], scope)...) // init
				sc := child()
				sc[bind.Head] = true
				issues = append(issues, checkNode(lib, fn, f.Args[1], sc)...) // cond
				issues = append(issues, checkNode(lib, fn, f.Args[2], sc)...) // step
			}
		}
	case "on-error":
		// (on-error body (e) handler) — e is bound in the handler scope
		if len(f.Args) == 3 {
			issues = append(issues, checkNode(lib, fn, f.Args[0], scope)...)
			sc := child()
			if bind, ok := f.Args[1].(Form); ok {
				sc[bind.Head] = true
			}
			issues = append(issues, checkNode(lib, fn, f.Args[2], sc)...)
		}
	case "retry":
		// (retry body n [backoff dur]) — body + n checked; backoff/dur are literals
		if len(f.Args) >= 2 {
			issues = append(issues, checkNode(lib, fn, f.Args[0], scope)...)
			issues = append(issues, checkNode(lib, fn, f.Args[1], scope)...)
		}
	default:
		// a call. If the head is a bound variable it holds a function value —
		// its target and arity are known only at runtime, so skip that check.
		if !scope[f.Head] {
			callee, ok := lib.Lookup(f.Head)
			if !ok {
				issues = append(issues, issueAt(fn, f.Pos, fmt.Sprintf("unknown function %q", f.Head)))
			} else if len(f.Args) != len(callee.In) {
				issues = append(issues, issueAt(fn, f.Pos, fmt.Sprintf("%q expects %d input(s), got %d", f.Head, len(callee.In), len(f.Args))))
			}
		}
		for _, a := range f.Args {
			issues = append(issues, checkNode(lib, fn, a, scope)...)
		}
	}
	return issues
}

// checkTypes verifies the *connections* in a composite body (docs/07): when an
// output feeds an input port, the types must fit. It is deliberately lenient —
// it only flags a clear scalar mismatch (e.g. a Str wired into a Num port), and
// stays silent whenever a stream, list, Json, Any, or unknown type is involved
// (those are handled at runtime, incl. the reactive per-item lift). So it never
// cries wolf on valid funk; it catches the wiring mistakes that actually bite.
func checkTypes(lib *Library, f *Fn) []Issue {
	env := map[string]string{}
	for _, p := range f.In {
		env[p.Name] = p.Type
	}
	var issues []Issue
	inferType(lib, f, f.Body, env, &issues)
	return issues
}

// baseScalar returns the type if it is a plain scalar element type, else "".
// Stream<…>, List, Json, Any, and "" are NOT plain scalars — we don't judge them.
func baseScalar(t string) string {
	switch t {
	case "Num", "Str", "Bool", "Time", "Bytes":
		return t
	}
	return ""
}

// inferType returns the (best-effort) type a node produces, and appends a
// connection issue whenever a call receives a scalar arg whose type clearly
// clashes with the input port. Unknown ("") propagates and disables judgement.
func inferType(lib *Library, fn *Fn, n Node, env map[string]string, issues *[]Issue) string {
	switch t := n.(type) {
	case Atom:
		switch t.Kind {
		case "num":
			return "Num"
		case "str":
			return "Str"
		case "id":
			if t.Value == "true" || t.Value == "false" {
				return "Bool"
			}
			return env[t.Value] // "" if unknown
		}
		return ""
	case Form:
		return inferForm(lib, fn, t, env, issues)
	}
	return ""
}

func inferForm(lib *Library, fn *Fn, f Form, env map[string]string, issues *[]Issue) string {
	switch f.Head {
	case "range", "nats", "repeat", "tick":
		return "Stream<Num>"
	case "collect", "window":
		for _, a := range f.Args {
			inferType(lib, fn, a, env, issues)
		}
		return "List"
	case "do":
		last := ""
		for _, a := range f.Args {
			last = inferType(lib, fn, a, env, issues)
		}
		return last
	case "let":
		if len(f.Args) == 2 {
			if bind, ok := f.Args[0].(Form); ok && len(bind.Args) >= 1 {
				expr := bind.Args[len(bind.Args)-1]
				ct := inferType(lib, fn, expr, env, issues)
				child := map[string]string{}
				for k, v := range env {
					child[k] = v
				}
				if len(bind.Args) == 1 { // single bind: the expr's type
					child[bind.Head] = ct
				} else { // destructure: element types unknown here
					child[bind.Head] = ""
					for i := 0; i < len(bind.Args)-1; i++ {
						if a, ok := bind.Args[i].(Atom); ok {
							child[a.Value] = ""
						}
					}
				}
				return inferType(lib, fn, f.Args[1], child, issues)
			}
		}
		return ""
	case "if":
		for _, a := range f.Args {
			inferType(lib, fn, a, env, issues)
		}
		return ""
	case "flush", "set", "exit", "for-each", "while", "on-error", "retry",
		"map", "filter", "take", "merge", "scan", "fold", "each", "yield":
		for _, a := range f.Args {
			inferType(lib, fn, a, env, issues)
		}
		return ""
	default:
		// a call: infer args, check each against the callee's input port type.
		callee, ok := lib.Lookup(f.Head)
		if !ok || env[f.Head] != "" { // unknown, or head is a passed-in Fn value
			for _, a := range f.Args {
				inferType(lib, fn, a, env, issues)
			}
			return ""
		}
		for i, a := range f.Args {
			at := inferType(lib, fn, a, env, issues)
			if i >= len(callee.In) {
				continue
			}
			prod, cons := baseScalar(at), baseScalar(callee.In[i].Type)
			if prod != "" && cons != "" && prod != cons {
				*issues = append(*issues, issueAt(fn, f.Pos, fmt.Sprintf(
					"connection type mismatch: %q input %q wants %s but gets %s",
					f.Head, callee.In[i].Name, cons, prod)))
			}
		}
		if len(callee.Out) == 1 {
			return callee.Out[0].Type
		}
		return ""
	}
}

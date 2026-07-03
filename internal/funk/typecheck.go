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
	"do": true, "let": true, "if": true, "return": true, "exit": true,
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
	case "do", "return", "exit", "if", "break", "continue",
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
		if len(f.Args) == 2 {
			if bind, ok := f.Args[0].(Form); ok {
				if len(bind.Args) == 1 {
					issues = append(issues, checkNode(lib, fn, bind.Args[0], scope)...)
				}
				sc := child()
				sc[bind.Head] = true
				issues = append(issues, checkNode(lib, fn, f.Args[1], sc)...)
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

package funk

import (
	"fmt"
	"strings"
)

// Issue is a static problem found by Check.
type Issue struct {
	Fn  string
	Msg string
}

func (i Issue) String() string { return fmt.Sprintf("%s: %s", i.Fn, i.Msg) }

var coreForms = map[string]bool{
	"do": true, "let": true, "if": true, "return": true, "exit": true,
	"for-each": true, "while": true, "window": true, "break": true, "continue": true,
	"range": true, "nats": true, "repeat": true,
	"take": true, "collect": true, "tick": true, "scan": true, "merge": true,
	"each": true, "yield": true, "fold": true,
}

// Check statically validates every composite function in the library:
// unresolved calls, arity mismatches, and unknown identifiers.
func Check(lib *Library) []Issue {
	var issues []Issue
	for _, f := range lib.Fns {
		if !f.Composite() {
			continue
		}
		scope := map[string]bool{}
		for _, p := range f.In {
			scope[p.Name] = true
		}
		issues = append(issues, checkNode(lib, f.Name, f.Body, scope)...)
	}
	return issues
}

func checkNode(lib *Library, fn string, n Node, scope map[string]bool) []Issue {
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
					return []Issue{{fn, fmt.Sprintf("unknown identifier %q", t.Value)}}
				}
			}
		}
		return nil
	case Form:
		return checkForm(lib, fn, t, scope)
	}
	return nil
}

func checkForm(lib *Library, fn string, f Form, scope map[string]bool) []Issue {
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
					issues = append(issues, Issue{fn, fmt.Sprintf("scan references unknown function %q", name)})
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
	default:
		// a call. If the head is a bound variable it holds a function value —
		// its target and arity are known only at runtime, so skip that check.
		if !scope[f.Head] {
			callee, ok := lib.Lookup(f.Head)
			if !ok {
				issues = append(issues, Issue{fn, fmt.Sprintf("unknown function %q", f.Head)})
			} else if len(f.Args) != len(callee.In) {
				issues = append(issues, Issue{fn, fmt.Sprintf("%q expects %d input(s), got %d", f.Head, len(callee.In), len(f.Args))})
			}
		}
		for _, a := range f.Args {
			issues = append(issues, checkNode(lib, fn, a, scope)...)
		}
	}
	return issues
}

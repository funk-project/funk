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
	"on-error": true, "retry": true, "with": true,
}

// Check statically validates every composite function in the library:
// unresolved calls, arity mismatches, and unknown identifiers.
func Check(lib *Library) []Issue {
	var issues []Issue
	// every `use` must declare an alias (docs/03): a plain import would ask for an
	// implicit global namespace, which no longer exists. Report once per file.
	usesReported := map[string]bool{}
	for _, f := range lib.Fns {
		if usesReported[f.File] {
			continue
		}
		for _, u := range f.Uses {
			if u.Alias == "" {
				usesReported[f.File] = true
				issues = append(issues, issueAt(f, u.Pos, fmt.Sprintf("`use %q` must declare an alias: `use %q as <name>` (then call it as <name>.fn)", u.Pkg, u.Pkg)))
			}
		}
	}
	for _, f := range lib.Fns {
		// Security advisory (docs/06 §6): a body that holds a raw secret AND has a
		// declared network egress could exfiltrate it — redaction is not a guarantee.
		if secretWithNet(lib, f) {
			issues = append(issues, Issue{Fn: f.Name, File: f.File, Pos: f.Pos, Warn: true,
				Msg: "raw secret + network egress — the body could exfiltrate the secret (docs/06 §6); prefer a brokered integration, or drop the net effect"})
		}
		// port type names must resolve (builtin or a declared type) — applies to
		// atomic and composite fns alike, so run it before the composite gate.
		issues = append(issues, checkTypeNames(lib, f)...)
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
				if _, ok := lib.ResolveIn(fn, t.Value); !ok {
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
				if _, ok := lib.ResolveIn(fn, name); !ok {
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
	case "with":
		// (with (kind.alias value)… body) — check binding values + the body; the
		// binding heads are resource keys, not calls.
		if len(f.Args) >= 1 {
			for _, b := range f.Args[:len(f.Args)-1] {
				if pair, ok := b.(Form); ok && len(pair.Args) == 1 {
					issues = append(issues, checkNode(lib, fn, pair.Args[0], scope)...)
				}
			}
			issues = append(issues, checkNode(lib, fn, f.Args[len(f.Args)-1], scope)...)
		}
	default:
		// a call. If the head is a bound variable it holds a function value —
		// its target and arity are known only at runtime, so skip that check.
		if !scope[f.Head] {
			callee, ok := lib.ResolveIn(fn, f.Head)
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

// builtinTypeOrder is the set of built-in value type names, in the canonical
// order the diagnostics list them (docs/04 §3). A port type's base must be one of
// these or a declared type name.
var builtinTypeOrder = []string{"Num", "Str", "Bool", "List", "Json", "Time", "Any", "Bytes", "Fn", "Stream"}

var builtinTypeSet = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range builtinTypeOrder {
		m[t] = true
	}
	return m
}()

// checkTypeNames validates that every port type in f names a real type: a builtin
// value type or a type declared in the library. It handles unions (`Num|Str`) and
// parameterized types (`List<Num>`, `Stream<Json>`) by validating each union part's
// base and recursing into the element. An unknown name is a real error (Warn:false)
// — the checker is the language's central promise, and a typo like `Number` for
// `Num` must not slip through.
func checkTypeNames(lib *Library, f *Fn) []Issue {
	var issues []Issue
	for _, p := range f.In {
		issues = append(issues, checkTypeName(lib, f, p.Name, p.Type)...)
	}
	for _, p := range f.Out {
		issues = append(issues, checkTypeName(lib, f, p.Name, p.Type)...)
	}
	return issues
}

func checkTypeName(lib *Library, f *Fn, port, t string) []Issue {
	if strings.TrimSpace(t) == "" {
		return nil // "" ⇒ Any, always valid
	}
	var issues []Issue
	for _, part := range strings.Split(t, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		base, elem := splitType(part)
		if !validTypeName(lib, base) {
			issues = append(issues, Issue{Fn: f.Name, File: f.File, Pos: f.Pos, Warn: false,
				Msg: unknownTypeMsg(base, port)})
		}
		if elem != "" {
			issues = append(issues, checkTypeName(lib, f, port, elem)...)
		}
	}
	return issues
}

// validTypeName reports whether name is a builtin value type or a declared type
// (by bare name or fully-qualified address — Library.Types keys both).
func validTypeName(lib *Library, name string) bool {
	if builtinTypeSet[name] {
		return true
	}
	if lib != nil && lib.Types != nil {
		if _, ok := lib.Types[name]; ok {
			return true
		}
	}
	return false
}

// unknownTypeMsg builds the diagnostic for an unresolved type base. A trivial
// did-you-mean fires only when the unknown name has a valid builtin as a prefix
// (e.g. "Number" → "Num"); otherwise the suggestion is omitted.
func unknownTypeMsg(name, port string) string {
	suggest := ""
	for _, b := range builtinTypeOrder {
		if b != name && strings.HasPrefix(name, b) {
			suggest = b
			break
		}
	}
	valid := strings.Join(builtinTypeOrder, " ") + " or a declared type"
	if suggest != "" {
		return fmt.Sprintf("unknown type %q on port %q (did you mean %q? valid: %s)", name, port, suggest, valid)
	}
	return fmt.Sprintf("unknown type %q on port %q (valid: %s)", name, port, valid)
}

// baseScalar returns the type if it is a plain scalar element type, else "".
// Stream<…>, List, Json, Any, and "" are NOT plain scalars — we don't judge them.
// splitType parses a rendered type into its outer constructor and element:
// "List<Num>" → ("List","Num"); "Num" → ("Num",""); "List" → ("List","").
func splitType(t string) (base, elem string) {
	if i := strings.IndexByte(t, '<'); i >= 0 && strings.HasSuffix(t, ">") {
		return t[:i], t[i+1 : len(t)-1]
	}
	return t, ""
}

// connMismatch reports a type mismatch on a connection (a producer's type flowing
// into a consumer input port), conservatively — an unknown/`Any` side is always
// compatible, and the reactive lift (a `Stream<E>` driving a scalar `E` port) is
// honoured. It flags: scalar↔scalar mismatch; a lift whose element differs from the
// scalar port; and same-constructor collections with differing concrete elements.
func connMismatch(prod, cons string) (want, got string, bad bool) {
	if prod == "" || cons == "" || prod == "Any" || cons == "Any" {
		return "", "", false
	}
	pb, pe := splitType(prod)
	cb, ce := splitType(cons)
	concrete := func(s string) bool { return s != "" && s != "Any" }
	switch {
	case pb == "Stream" && baseScalar(cb) != "" && ce == "": // reactive lift into a scalar port
		if concrete(pe) && pe != cb {
			return cb, pe, true
		}
	case baseScalar(pb) != "" && baseScalar(cb) != "" && pe == "" && ce == "": // scalar ↔ scalar
		if pb != cb {
			return cb, pb, true
		}
	case pb == cb && concrete(pe) && concrete(ce): // same collection, compare elements
		if pe != ce {
			return cons, prod, true
		}
	}
	return "", "", false
}

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
	case "flush", "set", "exit", "for-each", "while", "on-error", "retry", "with",
		"map", "filter", "take", "merge", "scan", "fold", "each", "yield":
		for _, a := range f.Args {
			inferType(lib, fn, a, env, issues)
		}
		return ""
	default:
		// a call: infer args, check each against the callee's input port type.
		callee, ok := lib.ResolveIn(fn, f.Head)
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
			if want, got, bad := connMismatch(at, callee.In[i].Type); bad {
				*issues = append(*issues, issueAt(fn, f.Pos, fmt.Sprintf(
					"connection type mismatch: %q input %q wants %s but gets %s",
					f.Head, callee.In[i].Name, want, got)))
			}
		}
		if len(callee.Out) == 1 {
			return callee.Out[0].Type
		}
		return ""
	}
}

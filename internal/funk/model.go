package funk

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Port is a typed input/output of a function: `(name Type spec…)`.
// Input ports carry a firing Policy + Group (docs/07 §2): ports in the same group
// fire together — "zip" (paired 1:1) or "latest" (combineLatest). Specs (min/max/
// default) are the per-port contract (docs/07 §2.2), checked on arrival (inputs)
// or as post-conditions (outputs). Presence of a default ⇒ the port is optional.
type Port struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`               // "Num", "Str", "Stream<Num>", … ("" ⇒ Any)
	Doc      string   `json:"doc,omitempty"`      // (doc "…") — optional human description
	Policy   string   `json:"policy,omitempty"`   // in-ports: "zip" | "latest"
	Group    int      `json:"group,omitempty"`    // in-ports: firing group index
	Min      *float64 `json:"min,omitempty"`      // (min n) refinement
	Max      *float64 `json:"max,omitempty"`      // (max n) refinement
	Default  Node     `json:"-"`                  // (default v) value expr; nil ⇒ required
	Optional bool     `json:"optional,omitempty"` // has a default ⇒ optional
}

// Need is one resource a function receives: `<kind> <alias> [schema]` in the
// `needs {}` block (docs/05). Kind is an integration type or secret|config|volume|env.
type Need struct {
	Kind   string `json:"kind"`
	Alias  string `json:"alias"`
	Schema string `json:"schema,omitempty"`
}

func (n Need) String() string {
	if n.Schema != "" {
		return n.Kind + " " + n.Alias + " " + n.Schema
	}
	return n.Kind + " " + n.Alias
}

// Effect is one declared capability from the `effects {}` block (docs/04 §10):
// `net <host>` / `fs read|write <path>`.
type Effect struct {
	Kind string   `json:"kind"`
	Args []string `json:"args"`
}

func (e Effect) String() string { return e.Kind + " " + strings.Join(e.Args, " ") }

// TestCase is an inline assertion: `test (call…) expected`.
type TestCase struct {
	Call   Node
	Expect Node
}

// Fn is a function: atomic (Engine+Src) or composite (Body). Never both.
type Fn struct {
	Name     string
	Display  string // optional `name` field — a human label for display only
	Package  string // e.g. "funk/std/maths"
	Doc      string // markdown description (supports triple-quoted multi-line)
	Examples string // optional `examples` field — markdown usage examples
	In       []Port
	Out      []Port
	Engine   string // atomic: "builtin" | "python" | "go" | "claude" | "codex"
	Src      string // atomic: body code, or a builtin primitive key
	Requires []string
	Needs    []Need
	Effects  []Effect
	Tests    []TestCase
	Body     Node      // composite: the single composing expression (nil ⇒ atomic)
	File     string    // source file this fn was loaded from ("" if from a string)
	Uses     []UseSpec // the `use` imports of this fn's file (scopes call resolution)
	Main     bool      // marked `main` — a runnable entry point (`funk run file.funk`)
	Pos      Pos       // position of the `fn` keyword, for diagnostics
}

// UseSpec is one `use "pkg" [as alias]` import declared in a file's package block.
// A plain import (Alias == "") exposes the package's functions by bare name; an
// aliased import requires callers to qualify them as `alias.fn` and hides the bare
// name (docs/03 — a reference is an address; `as` gives it a local handle).
type UseSpec struct {
	Pkg   string // e.g. "funk/std/maths"
	Alias string // "" ⇒ plain import (an error — every `use` must be aliased); else the qualifier, e.g. "maths"
	Pos   Pos    // position of the `use` field, for diagnostics
}

// Composite reports whether the function is composed of other functions.
func (f *Fn) Composite() bool { return f.Body != nil }

// DisplayName is the human label for the function: the optional `name` field, or
// the function's identifier when none is given. For display only (never used for
// resolution/addressing).
func (f *Fn) DisplayName() string {
	if f.Display != "" {
		return f.Display
	}
	return f.Name
}

// Address returns the fully-qualified address, e.g. "funk/std/maths/add".
func (f *Fn) Address() string {
	if f.Package == "" {
		return f.Name
	}
	return f.Package + "/" + f.Name
}

// portFromForm builds one port from a `(name Type spec…)` form, tagging it with
// the given firing policy/group. Returns false for `()` / non-forms.
func portFromForm(n Node, policy string, group int) (Port, bool) {
	form, ok := n.(Form)
	if !ok || form.Head == "" {
		return Port{}, false
	}
	p := Port{Name: form.Head, Policy: policy, Group: group}
	if len(form.Args) > 0 {
		p.Type = nodeTypeString(form.Args[0])
	}
	for i := 1; i < len(form.Args); i++ {
		sf, ok := form.Args[i].(Form)
		if !ok {
			continue
		}
		switch sf.Head {
		case "min":
			if v, ok := numArg(sf); ok {
				p.Min = &v
			}
		case "max":
			if v, ok := numArg(sf); ok {
				p.Max = &v
			}
		case "default":
			if len(sf.Args) > 0 {
				p.Default = sf.Args[0]
				p.Optional = true
			}
		case "doc":
			if len(sf.Args) > 0 {
				p.Doc = atomStr(sf.Args[0])
			}
		}
	}
	return p, true
}

func numArg(f Form) (float64, bool) {
	if len(f.Args) == 0 {
		return 0, false
	}
	a, ok := f.Args[0].(Atom)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(a.Value, 64)
	return v, err == nil
}

// portsFromField reads out-ports: bare `(name Type spec…)` forms, no groups.
func portsFromField(f Field) []Port {
	var ports []Port
	for _, v := range f.Values {
		if p, ok := portFromForm(v, "", 0); ok {
			ports = append(ports, p)
		}
	}
	return ports
}

// inPortsFromField reads input ports with firing groups (docs/07 §2): a value
// whose head is `zip`/`latest` is a group of ports; bare port forms collapse into
// a single implicit `zip` group. `zip` is the default policy.
func inPortsFromField(f Field) []Port {
	var ports []Port
	next, implicit := 0, -1
	for _, v := range f.Values {
		form, ok := v.(Form)
		if !ok || form.Head == "" {
			continue
		}
		if form.Head == "zip" || form.Head == "latest" {
			gi := next
			next++
			for _, a := range form.Args {
				if p, ok := portFromForm(a, form.Head, gi); ok {
					ports = append(ports, p)
				}
			}
			continue
		}
		// a bare port → the implicit zip group
		if implicit < 0 {
			implicit = next
			next++
		}
		if p, ok := portFromForm(v, "zip", implicit); ok {
			ports = append(ports, p)
		}
	}
	return ports
}

func atomStr(n Node) string {
	if a, ok := n.(Atom); ok {
		return a.Value
	}
	return ""
}

// nodeTypeString renders a type node. A parameterized type is written homoiconically
// as a prefix form — `(List Num)`, `(Stream Num)`, `(Map Str Num)` — and renders in
// the familiar angle-bracket form `List<Num>`. A no-arg form (e.g. a bare `(Num|Str)`
// union) renders as just its head; an already-atomic `List<Num>`/`Num|Str` passes
// through verbatim.
func nodeTypeString(n Node) string {
	switch t := n.(type) {
	case Atom:
		return t.Value
	case Form:
		if len(t.Args) == 0 {
			return t.Head
		}
		var parts []string
		for _, a := range t.Args {
			parts = append(parts, nodeTypeString(a))
		}
		return t.Head + "<" + strings.Join(parts, ",") + ">"
	}
	return "Any"
}

// FnFromBlock builds an Fn from a parsed `fn` block.
func FnFromBlock(b Block, pkg string) (*Fn, error) {
	if b.Head != "fn" {
		return nil, &ParseError{Pos: b.Pos, Msg: fmt.Sprintf("not an fn block: %q", b.Head)}
	}
	f := &Fn{Name: b.Name, Package: pkg, Pos: b.Pos}
	f.Display = b.FieldStr("name") // optional display label; defaults to Name via DisplayName()
	f.Doc = b.FieldStr("doc")
	f.Examples = b.FieldStr("examples")
	_, f.Main = b.Field("main") // presence of a bare `main` field ⇒ runnable entry point
	f.Engine = b.FieldStr("engine")
	if in, ok := b.Field("in"); ok {
		f.In = inPortsFromField(in)
	}
	if out, ok := b.Field("out"); ok {
		f.Out = portsFromField(out)
	}
	if src, ok := b.Field("src"); ok && len(src.Values) > 0 {
		if a, ok := src.Values[0].(Atom); ok {
			f.Src = a.Value
		}
	}
	if body, ok := b.Field("body"); ok && len(body.Values) > 0 {
		f.Body = body.Values[0]
	}
	if req, ok := b.Field("requires"); ok {
		for _, v := range req.Values {
			if form, ok := v.(Form); ok {
				f.Requires = append(f.Requires, form.Head)
			}
		}
	}
	if nf, ok := b.Field("needs"); ok {
		for _, sf := range nf.Sub {
			n := Need{Kind: sf.Key}
			if len(sf.Values) > 0 {
				n.Alias = atomStr(sf.Values[0])
			}
			if len(sf.Values) > 1 {
				n.Schema = atomStr(sf.Values[1])
			}
			f.Needs = append(f.Needs, n)
		}
	}
	if ef, ok := b.Field("effects"); ok {
		for _, sf := range ef.Sub {
			e := Effect{Kind: sf.Key}
			for _, v := range sf.Values {
				e.Args = append(e.Args, atomStr(v))
			}
			f.Effects = append(f.Effects, e)
		}
	}
	for _, fld := range b.Fields {
		// test is a single form `(is <call> <expected>)`.
		if fld.Key == "test" && len(fld.Values) >= 1 {
			if form, ok := fld.Values[0].(Form); ok && len(form.Args) >= 2 {
				f.Tests = append(f.Tests, TestCase{Call: form.Args[0], Expect: form.Args[1]})
			}
		}
	}
	if f.Src != "" && f.Body != nil {
		return nil, &ParseError{Pos: b.Pos, Msg: fmt.Sprintf("fn %q has both src and body (must be exactly one)", f.Name)}
	}
	return f, nil
}

// TypeDef is a named type / schema: `type User { name Str  age Num }`.
type TypeDef struct {
	Name    string
	Package string
	Fields  []Port
}

// Address returns the fully-qualified type address.
func (t *TypeDef) Address() string {
	if t.Package == "" {
		return t.Name
	}
	return t.Package + "/" + t.Name
}

// TypeFromBlock builds a TypeDef from a parsed `type` block.
func TypeFromBlock(b Block, pkg string) *TypeDef {
	td := &TypeDef{Name: b.Name, Package: pkg}
	for _, f := range b.Fields {
		p := Port{Name: f.Key}
		if len(f.Values) > 0 {
			p.Type = nodeTypeString(f.Values[0])
		}
		td.Fields = append(td.Fields, p)
	}
	return td
}

// Library is a set of loaded functions and types.
type Library struct {
	byAddr map[string]*Fn
	byName map[string]*Fn
	Fns    []*Fn
	Types  map[string]*TypeDef
}

// NewLibrary returns an empty library.
func NewLibrary() *Library {
	return &Library{byAddr: map[string]*Fn{}, byName: map[string]*Fn{}, Types: map[string]*TypeDef{}}
}

// Add registers a function (address wins on conflict; bare name is best-effort).
func (l *Library) Add(f *Fn) {
	l.byAddr[f.Address()] = f
	l.byName[f.Name] = f
	l.Fns = append(l.Fns, f)
}

// ResolveIn resolves a call reference from within the function `cur` (docs/03).
// The rules, for a *packaged* caller:
//   - `alias.fn` resolves against that file's `use … as alias` import;
//   - a fully-qualified address ("pkg/fn") always resolves;
//   - a bare name resolves ONLY within the caller's own package.
//
// There is no implicit global cross-package namespace: to call another package's
// function you must import it `as` and qualify the call. When cur is nil (a
// top-level entry ref or a first-class Fn value's address) or unpackaged (ad-hoc /
// REPL code with no package block), it degrades to the flat global Lookup.
func (l *Library) ResolveIn(cur *Fn, ref string) (*Fn, bool) {
	if cur != nil {
		if i := strings.IndexByte(ref, '.'); i > 0 {
			alias, member := ref[:i], ref[i+1:]
			for _, u := range cur.Uses {
				if u.Alias != "" && u.Alias == alias {
					if f, ok := l.byAddr[u.Pkg+"/"+member]; ok {
						return f, true
					}
					return nil, false // known alias, unknown member — do not fall through
				}
			}
		}
	}
	// a fully-qualified address is always allowed
	if f, ok := l.byAddr[ref]; ok {
		return f, true
	}
	// within a package, a bare name resolves only to that same package
	if cur != nil && cur.Package != "" && !strings.ContainsAny(ref, "./") {
		if f, ok := l.byAddr[cur.Package+"/"+ref]; ok {
			return f, true
		}
		return nil, false
	}
	// unpackaged / top-level code keeps the flat global namespace
	return l.Lookup(ref)
}

// Lookup resolves a reference by address, then by bare name.
func (l *Library) Lookup(ref string) (*Fn, bool) {
	if f, ok := l.byAddr[ref]; ok {
		return f, true
	}
	if f, ok := l.byName[ref]; ok {
		return f, true
	}
	// tolerate a trailing name after a package path, e.g. maths/add → add
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		if f, ok := l.byName[ref[i+1:]]; ok {
			return f, true
		}
	}
	return nil, false
}

// parseUses reads the `use "pkg" [as alias]` imports from a package block. The
// parser splits `use "pkg" as alias` into two ordered fields (`use` then `as`,
// since a bare atom starts a new field), so an `as` field immediately following a
// `use` binds the alias to it.
func parseUses(pkg Block) []UseSpec {
	var out []UseSpec
	fields := pkg.Fields
	for i := 0; i < len(fields); i++ {
		if fields[i].Key != "use" || len(fields[i].Values) == 0 {
			continue
		}
		spec := UseSpec{Pkg: atomStr(fields[i].Values[0]), Pos: fields[i].Pos}
		if i+1 < len(fields) && fields[i+1].Key == "as" && len(fields[i+1].Values) == 1 {
			spec.Alias = atomStr(fields[i+1].Values[0])
			i++
		}
		out = append(out, spec)
	}
	return out
}

// LoadString parses .funk source and adds its fn and type blocks.
func (l *Library) LoadString(src string) error { return l.loadString(src, "") }

// loadString parses src (from file path, "" if none) and adds its blocks. When a
// path is known, parse/build errors are prefixed as "path:line:col: message" so a
// File Watcher / LSP can navigate to them; the position rides on the ParseError.
func (l *Library) loadString(src, path string) error {
	withPath := func(err error) error {
		if err == nil || path == "" {
			return err
		}
		return fmt.Errorf("%s:%s", path, err)
	}
	prog, err := Parse(src)
	if err != nil {
		return withPath(err)
	}
	pkg := ""
	var uses []UseSpec
	for i := range prog {
		if prog[i].Head == "package" {
			pkg = prog[i].Name
			uses = parseUses(prog[i])
		}
	}
	for _, b := range prog {
		switch b.Head {
		case "fn":
			f, err := FnFromBlock(b, pkg)
			if err != nil {
				return withPath(err)
			}
			f.File = path
			f.Uses = uses
			l.Add(f)
		case "type":
			td := TypeFromBlock(b, pkg)
			l.Types[td.Name] = td
			l.Types[td.Address()] = td
		}
	}
	return nil
}

// LoadFile parses a .funk file and adds its blocks to the library.
func (l *Library) LoadFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return l.loadString(string(src), path)
}

// LoadDir walks a directory tree and loads every .funk file.
func (l *Library) LoadDir(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".funk") {
			return nil
		}
		return l.LoadFile(path)
	})
}

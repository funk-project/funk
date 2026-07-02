package funk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Port is a typed input/output of a function: `(name Type)`.
type Port struct {
	Name string `json:"name"`
	Type string `json:"type"` // "Num", "Str", "Stream<Num>", … ("" ⇒ Any)
}

// Fn is a function: atomic (Engine+Src) or composite (Body). Never both.
type Fn struct {
	Name     string
	Package  string // e.g. "funk/std/maths"
	Doc      string
	In       []Port
	Out      []Port
	Engine   string // atomic: "builtin" | "python" | "go" | "claude" | "codex"
	Src      string // atomic: body code, or a builtin primitive key
	Requires []string
	Body     Node // composite: the single composing expression (nil ⇒ atomic)
}

// Composite reports whether the function is composed of other functions.
func (f *Fn) Composite() bool { return f.Body != nil }

// Address returns the fully-qualified address, e.g. "funk/std/maths/add".
func (f *Fn) Address() string {
	if f.Package == "" {
		return f.Name
	}
	return f.Package + "/" + f.Name
}

func portsFromField(f Field) []Port {
	var ports []Port
	for _, v := range f.Values {
		form, ok := v.(Form)
		if !ok {
			continue
		}
		if form.Head == "" { // `in ()` — no ports
			continue
		}
		p := Port{Name: form.Head}
		if len(form.Args) > 0 {
			p.Type = nodeTypeString(form.Args[0])
		}
		ports = append(ports, p)
	}
	return ports
}

// nodeTypeString renders a type node like `Num` or `Stream<Num>` (a form).
func nodeTypeString(n Node) string {
	switch t := n.(type) {
	case Atom:
		return t.Value
	case Form:
		// e.g. (Stream Num) or already-atomic "Stream<Num>"
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
		return nil, fmt.Errorf("not an fn block: %q", b.Head)
	}
	f := &Fn{Name: b.Name, Package: pkg}
	f.Doc = b.FieldStr("doc")
	f.Engine = b.FieldStr("engine")
	if in, ok := b.Field("in"); ok {
		f.In = portsFromField(in)
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
	if f.Src != "" && f.Body != nil {
		return nil, fmt.Errorf("fn %q has both src and body (must be exactly one)", f.Name)
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

// LoadString parses .funk source and adds its fn and type blocks.
func (l *Library) LoadString(src string) error {
	prog, err := Parse(src)
	if err != nil {
		return err
	}
	pkg := ""
	for _, b := range prog {
		if b.Head == "package" {
			pkg = b.Name
		}
	}
	for _, b := range prog {
		switch b.Head {
		case "fn":
			f, err := FnFromBlock(b, pkg)
			if err != nil {
				return err
			}
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
	if err := l.LoadString(string(src)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
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

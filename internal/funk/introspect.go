package funk

// Introspection is a function's structure as data — the self-observable view
// (docs/01, property #4). The agent reads this about itself.
type Introspection struct {
	Name       string   `json:"name"`
	Address    string   `json:"address"`
	Doc        string   `json:"doc,omitempty"`
	Kind       string   `json:"kind"` // "atomic" | "composite"
	Engine     string   `json:"engine,omitempty"`
	In         []Port   `json:"in"`
	Out        []Port   `json:"out"`
	Requires   []string `json:"requires,omitempty"`
	Calls      []string `json:"calls,omitempty"`      // functions a composite calls
	Unresolved []string `json:"unresolved,omitempty"` // calls that do not resolve
}

// Introspect returns the structure of a function by reference.
func Introspect(lib *Library, ref string) (*Introspection, bool) {
	f, ok := lib.Lookup(ref)
	if !ok {
		return nil, false
	}
	in := &Introspection{
		Name:     f.Name,
		Address:  f.Address(),
		Doc:      f.Doc,
		In:       f.In,
		Out:      f.Out,
		Requires: f.Requires,
	}
	if f.Composite() {
		in.Kind = "composite"
		seen := map[string]bool{}
		collectCalls(f.Body, seen)
		for name := range seen {
			in.Calls = append(in.Calls, name)
			if _, ok := lib.Lookup(name); !ok {
				in.Unresolved = append(in.Unresolved, name)
			}
		}
	} else {
		in.Kind = "atomic"
		in.Engine = f.Engine
	}
	return in, true
}

// collectCalls walks a body, recording every non-core call head.
func collectCalls(n Node, seen map[string]bool) {
	f, ok := n.(Form)
	if !ok {
		return
	}
	if f.Head != "" && !coreForms[f.Head] {
		seen[f.Head] = true
	}
	// skip a let/for-each binder form (it is not a call)
	for i, a := range f.Args {
		if (f.Head == "let" || f.Head == "for-each") && i == 0 {
			if bind, ok := a.(Form); ok {
				for _, ba := range bind.Args {
					collectCalls(ba, seen)
				}
			}
			continue
		}
		collectCalls(a, seen)
	}
}

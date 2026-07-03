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
	Needs      []Need   `json:"needs,omitempty"`      // aggregated resources (docs/05)
	Effects    []Effect `json:"effects,omitempty"`    // aggregated capabilities
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
	// Declaration bubbles up (docs/05): a composite's needs/effects are the
	// union of its own and everything it (transitively) calls.
	in.Needs, in.Effects = aggregateResources(lib, f, map[string]bool{})
	return in, true
}

func aggregateResources(lib *Library, f *Fn, seen map[string]bool) ([]Need, []Effect) {
	if seen[f.Address()] {
		return nil, nil
	}
	seen[f.Address()] = true
	needs := append([]Need{}, f.Needs...)
	effects := append([]Effect{}, f.Effects...)
	if f.Composite() {
		calls := map[string]bool{}
		collectCalls(f.Body, calls)
		for name := range calls {
			if c, ok := lib.Lookup(name); ok {
				cn, ce := aggregateResources(lib, c, seen)
				needs = append(needs, cn...)
				effects = append(effects, ce...)
			}
		}
	}
	return dedupNeeds(needs), dedupEffects(effects)
}

func dedupNeeds(ns []Need) []Need {
	seen := map[string]bool{}
	var out []Need
	for _, n := range ns {
		if k := n.String(); !seen[k] {
			seen[k] = true
			out = append(out, n)
		}
	}
	return out
}

func dedupEffects(es []Effect) []Effect {
	seen := map[string]bool{}
	var out []Effect
	for _, e := range es {
		if k := e.String(); !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
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

package funk

import "testing"

// deriveByName parses src, finalizes the library, and derives the named fn.
func deriveByName(t *testing.T, src, name string) *Graph {
	t.Helper()
	lib := NewLibrary()
	if err := lib.LoadString(src); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := lib.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	f, ok := lib.Lookup(name)
	if !ok {
		t.Fatalf("fn %q not found", name)
	}
	g, err := DeriveGraph(lib, f)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	return g
}

func nodeByIDT(g *Graph, id string) *GraphNode {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func hasEdge(g *Graph, source, target string) bool {
	for _, e := range g.Edges {
		if e.Source == source && e.Target == target {
			return true
		}
	}
	return false
}

// A simple chain: (add (mul x 2) 1) — mul reads x, add reads mul, add implicitly
// flows to the output r.
func TestDeriveGraphChain(t *testing.T) {
	src := `fn chain { in (x Num) out (r Num) body (add (mul x 2) 1) }`
	g := deriveByName(t, src, "chain")

	if n := nodeByIDT(g, "in.0"); n == nil || n.Kind != "input" || n.Fn != "x" {
		t.Fatalf("input node wrong: %+v", n)
	}
	if n := nodeByIDT(g, "out.0"); n == nil || n.Kind != "output" || n.Fn != "r" {
		t.Fatalf("output node wrong: %+v", n)
	}
	if n := nodeByIDT(g, "b"); n == nil || n.Kind != "call" || n.Fn != "add" {
		t.Fatalf("add node wrong: %+v", n)
	}
	if n := nodeByIDT(g, "b.0"); n == nil || n.Kind != "call" || n.Fn != "mul" {
		t.Fatalf("mul node wrong: %+v", n)
	}
	if !hasEdge(g, "in.0", "b.0") {
		t.Fatal("expected x -> mul edge")
	}
	if !hasEdge(g, "b.0", "b") {
		t.Fatal("expected mul -> add edge")
	}
	if !hasEdge(g, "b", "out.0") {
		t.Fatal("expected add -> output (implicit result) edge")
	}
}

// An if is drawn as a decision BOX (group) holding a cond diamond, two branches,
// and a merge; the input enters through the box's in:0 port.
func TestDeriveGraphIf(t *testing.T) {
	src := `fn bump { in (x Num) out (r Num) body (if (gt x 0) (return (add x 100)) (return x)) }`
	g := deriveByName(t, src, "bump")

	box := nodeByIDT(g, "b.box")
	if box == nil || box.Kind != "group" || box.Fn != "if" {
		t.Fatalf("box wrong: %+v", box)
	}
	dec := nodeByIDT(g, "b.cond")
	if dec == nil || dec.Kind != "cond" || dec.ParentNode != "b.box" {
		t.Fatalf("cond wrong: %+v", dec)
	}
	if dec.Label != "x > 0" {
		t.Fatalf("cond label = %q, want %q", dec.Label, "x > 0")
	}
	// true branch: the add computation; false branch: a bare terminal.
	if n := nodeByIDT(g, "b.t"); n == nil || n.Fn != "add" || n.Branch != "true" {
		t.Fatalf("true branch wrong: %+v", n)
	}
	if n := nodeByIDT(g, "b.e"); n == nil || n.Kind != "terminal" || n.Branch != "false" {
		t.Fatalf("false branch wrong: %+v", n)
	}
	if nodeByIDT(g, "b.merge") == nil {
		t.Fatal("expected a merge node")
	}
	// no `gt` node — the guard lives in the decision title.
	for _, n := range g.Nodes {
		if n.Fn == "gt" {
			t.Fatalf("gt should not be a node: %+v", n)
		}
	}
	// decision forks (control) to each branch entry.
	forks := 0
	for _, e := range g.Edges {
		if e.Source == "b.cond" && e.Kind == "control" {
			forks++
			if e.SourceHandle != "true" && e.SourceHandle != "false" {
				t.Fatalf("fork handle = %q", e.SourceHandle)
			}
		}
	}
	if forks != 2 {
		t.Fatalf("expected 2 forks, got %d", forks)
	}
	// input enters through box in:0, and the box pin:0 feeds every reader.
	if !hasEdge(g, "in.0", "b.box") {
		t.Fatal("expected x -> box edge")
	}
	readers := map[string]bool{}
	for _, e := range g.Edges {
		if e.Source == "b.box" && e.SourceHandle == "pin:0" {
			readers[e.Target] = true
		}
	}
	for _, want := range []string{"b.cond", "b.t", "b.e"} {
		if !readers[want] {
			t.Fatalf("expected box pin:0 -> %s", want)
		}
	}
}

// An atomic (engine) function draws as a single atom node wired input→op→output.
func TestDeriveGraphAtomic(t *testing.T) {
	src := `fn sq { in (x Num) out (r Num) engine builtin src "num.mul" }`
	g := deriveByName(t, src, "sq")
	b := nodeByIDT(g, "b")
	if b == nil || b.Kind != "atom" || b.Engine != "builtin" {
		t.Fatalf("atom node wrong: %+v", b)
	}
	if !hasEdge(g, "in.0", "b") || !hasEdge(g, "b", "out.0") {
		t.Fatal("expected in -> atom -> out edges")
	}
}

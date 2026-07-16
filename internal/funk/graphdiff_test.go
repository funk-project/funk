package funk

import "testing"

// diffFnSources loads two single-fn sources and diffs the named fn's graphs
// (old → new).
func diffFnSources(t *testing.T, oldSrc, newSrc, name string) *GraphDiff {
	t.Helper()
	load := func(src string) (*Library, *Fn) {
		lib := NewLibrary()
		if err := lib.LoadString(src); err != nil {
			t.Fatalf("load: %v", err)
		}
		f, ok := lib.Lookup(name)
		if !ok {
			t.Fatalf("fn %q not found", name)
		}
		return lib, f
	}
	oldLib, fa := load(oldSrc)
	newLib, fb := load(newSrc)
	ga, err := DeriveGraph(oldLib, fa)
	if err != nil {
		t.Fatalf("derive old: %v", err)
	}
	gb, err := DeriveGraph(newLib, fb)
	if err != nil {
		t.Fatalf("derive new: %v", err)
	}
	return DiffGraphs(ga, gb)
}

// Identical sources produce an identical diff (and node ids never surface as
// spurious changes).
func TestDiffGraphsIdentical(t *testing.T) {
	src := `fn f { in (n Num) out (r Num) body (while (s 1) (lte (mul s 2) n) (mul s 2)) }`
	d := diffFnSources(t, src, src, "f")
	if !d.Identical() {
		t.Fatalf("expected identical, got %+v", d)
	}
	if d.UnchangedNodes == 0 || d.UnchangedEdges == 0 {
		t.Fatalf("expected unchanged counts, got nodes=%d edges=%d", d.UnchangedNodes, d.UnchangedEdges)
	}
}

// Adding a call inside the loop reports ONE added node — the surviving `mul`
// matches by identity even though its AST-path id shifted (b.step → b.step.0),
// and the loop carry re-wires to the new step top.
func TestDiffGraphsAddNode(t *testing.T) {
	oldSrc := `fn f { in (n Num) out (r Num) body (while (s 1) (lte (mul s 2) n) (mul s 2)) }`
	newSrc := `fn f { in (n Num) out (r Num) body (while (s 1) (lte (mul s 2) n) (add (mul s 2) 1)) }`
	d := diffFnSources(t, oldSrc, newSrc, "f")
	if d.Identical() {
		t.Fatal("expected a difference")
	}
	if len(d.AddedNodes) != 1 || d.AddedNodes[0].Fn != "add" || d.AddedNodes[0].Kind != "call" {
		t.Fatalf("added nodes = %+v, want one call add", d.AddedNodes)
	}
	if d.AddedNodes[0].Context != "inside while box" {
		t.Fatalf("added node context = %q, want %q", d.AddedNodes[0].Context, "inside while box")
	}
	if len(d.RemovedNodes) != 0 {
		t.Fatalf("removed nodes = %+v, want none (mul must survive its id shift)", d.RemovedNodes)
	}
	// the loop carry (→ register) moved from mul to add: one rewired edge.
	if len(d.RewiredEdges) != 1 {
		t.Fatalf("rewired edges = %+v, want exactly one (the loop carry)", d.RewiredEdges)
	}
	re := d.RewiredEdges[0]
	if re.What != "source" || re.Source != "call add" || re.Was != "call mul" {
		t.Fatalf("rewired = %+v, want the carry now fed by call add, was call mul", re)
	}
	// plus the new mul → add data edge.
	if len(d.AddedEdges) != 1 || d.AddedEdges[0].Source != "call mul" || d.AddedEdges[0].Target != "call add" {
		t.Fatalf("added edges = %+v, want mul → add", d.AddedEdges)
	}
}

// Switching a call's argument from input a to input b reports a REWIRED edge
// (same reader, new source), not a remove+add pair.
func TestDiffGraphsRewireEdge(t *testing.T) {
	oldSrc := `fn g { in (a Num) (b Num) out (r Num) body (mul a 2) }`
	newSrc := `fn g { in (a Num) (b Num) out (r Num) body (mul b 2) }`
	d := diffFnSources(t, oldSrc, newSrc, "g")
	if len(d.AddedNodes)+len(d.RemovedNodes)+len(d.ChangedNodes) != 0 {
		t.Fatalf("expected no node changes, got %+v %+v %+v", d.AddedNodes, d.RemovedNodes, d.ChangedNodes)
	}
	if len(d.RewiredEdges) != 1 {
		t.Fatalf("rewired = %+v, want exactly one", d.RewiredEdges)
	}
	re := d.RewiredEdges[0]
	if re.What != "source" || re.Target != "call mul" || re.Source != "input b" || re.Was != "input a" {
		t.Fatalf("rewired = %+v, want call mul now fed by input b, was input a", re)
	}
	if len(d.AddedEdges) != 0 || len(d.RemovedEdges) != 0 {
		t.Fatalf("expected no plain edge adds/removes, got +%+v -%+v", d.AddedEdges, d.RemovedEdges)
	}
}

// Changing a guard rewrites the decision's LABEL — reported as a changed node
// (old → new label), not a remove+add, and nothing else moves.
func TestDiffGraphsLabelChange(t *testing.T) {
	oldSrc := `fn h { in (x Num) out (r Num) body (if (gt x 0) (return 1) (return 2)) }`
	newSrc := `fn h { in (x Num) out (r Num) body (if (gte x 0) (return 1) (return 2)) }`
	d := diffFnSources(t, oldSrc, newSrc, "h")
	if len(d.ChangedNodes) != 1 {
		t.Fatalf("changed nodes = %+v, want exactly one", d.ChangedNodes)
	}
	cn := d.ChangedNodes[0]
	if cn.Kind != "cond" || cn.OldLabel != "x > 0" || cn.Label != "x ≥ 0" {
		t.Fatalf("changed = %+v, want cond label 'x > 0' → 'x ≥ 0'", cn)
	}
	if len(d.AddedNodes)+len(d.RemovedNodes) != 0 {
		t.Fatalf("expected no node adds/removes, got +%+v -%+v", d.AddedNodes, d.RemovedNodes)
	}
	if len(d.AddedEdges)+len(d.RemovedEdges)+len(d.RewiredEdges) != 0 {
		t.Fatalf("expected no edge changes, got +%+v -%+v ~%+v", d.AddedEdges, d.RemovedEdges, d.RewiredEdges)
	}
}

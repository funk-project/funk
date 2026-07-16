package funk

// Semantic graph diff — compares two DERIVED graphs (DeriveGraph output) as a
// picture-level delta, for reviewing AI-written changes: which nodes appeared /
// disappeared, which edges were re-wired, which labels (guards, types) changed.
//
// Nodes are matched by STABLE IDENTITY, not raw ids: a node's id is its AST
// path ("b", "b.0", "b.t"), which shifts whenever code moves — wrapping a call
// in another call renumbers everything under it. So identity is (kind, fn,
// label) plus the PARENT-CHAIN context (the chain of enclosing boxes' kind|fn);
// raw ids only break ties between otherwise-identical candidates. A second,
// label-blind pass then pairs leftovers that kept (kind, fn, context) but
// changed their label — a guard tweak or a type change reads as a CHANGED node,
// not a remove+add.

import (
	"sort"
	"strings"
)

// NodeDelta is one node-level change.
type NodeDelta struct {
	Op       string `json:"op"` // added | removed | changed
	ID       string `json:"id"` // raw AST-path id (new side for added/changed, old side for removed)
	Kind     string `json:"kind"`
	Fn       string `json:"fn,omitempty"`
	Label    string `json:"label,omitempty"`
	OldLabel string `json:"oldLabel,omitempty"` // changed only: the previous label
	Context  string `json:"context,omitempty"`  // e.g. "inside while box"
}

// EdgeDelta is one edge-level change. Source/Target are human descriptions of
// the endpoints ("input n", "call mul"), not raw ids.
type EdgeDelta struct {
	Op           string `json:"op"` // added | removed | rewired
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
	TargetHandle string `json:"targetHandle,omitempty"`
	Type         string `json:"type,omitempty"`
	Kind         string `json:"kind"`
	Access       string `json:"access,omitempty"`
	Was          string `json:"was,omitempty"`  // rewired only: the previous endpoint
	What         string `json:"what,omitempty"` // rewired only: which end moved — "source" | "target"
}

// GraphDiff is the full semantic delta between two graphs.
type GraphDiff struct {
	AddedNodes     []NodeDelta `json:"addedNodes,omitempty"`
	RemovedNodes   []NodeDelta `json:"removedNodes,omitempty"`
	ChangedNodes   []NodeDelta `json:"changedNodes,omitempty"`
	AddedEdges     []EdgeDelta `json:"addedEdges,omitempty"`
	RemovedEdges   []EdgeDelta `json:"removedEdges,omitempty"`
	RewiredEdges   []EdgeDelta `json:"rewiredEdges,omitempty"`
	UnchangedNodes int         `json:"unchangedNodes"`
	UnchangedEdges int         `json:"unchangedEdges"`
}

// Identical reports whether the two graphs are semantically the same picture.
func (d *GraphDiff) Identical() bool {
	return len(d.AddedNodes) == 0 && len(d.RemovedNodes) == 0 && len(d.ChangedNodes) == 0 &&
		len(d.AddedEdges) == 0 && len(d.RemovedEdges) == 0 && len(d.RewiredEdges) == 0
}

// ── one side of the diff ─────────────────────────────────────────────────────

type diffSide struct {
	g    *Graph
	byID map[string]*GraphNode
	ctx  map[string]string // node id → ancestor chain of "kind|fn", outermost first
}

func newDiffSide(g *Graph) *diffSide {
	s := &diffSide{g: g, byID: map[string]*GraphNode{}, ctx: map[string]string{}}
	for _, n := range g.Nodes {
		s.byID[n.ID] = n
	}
	for _, n := range g.Nodes {
		s.ctx[n.ID] = s.chain(n)
	}
	return s
}

// chain renders a node's parent-box chain — label-blind (kind|fn only), so a
// retitled box does not break the identity of everything inside it.
func (s *diffSide) chain(n *GraphNode) string {
	var parts []string
	seen := map[string]bool{}
	for p := s.byID[n.ParentNode]; p != nil && !seen[p.ID]; p = s.byID[p.ParentNode] {
		seen[p.ID] = true
		parts = append([]string{p.Kind + "|" + p.Fn}, parts...)
	}
	return strings.Join(parts, "/")
}

// context renders the human context of a node ("inside while box").
func (s *diffSide) context(n *GraphNode) string {
	p := s.byID[n.ParentNode]
	if p == nil {
		return ""
	}
	name := p.Fn
	if name == "" {
		name = p.Label
	}
	return "inside " + name + " box"
}

// describe renders a short human name for a node ("input n", "call mul").
func describeNode(n *GraphNode) string {
	name := n.Fn
	if name == "" {
		name = n.Label
	}
	if name == "" {
		return n.Kind
	}
	return n.Kind + " " + name
}

// ── node matching ────────────────────────────────────────────────────────────

type nodePair struct{ a, b *GraphNode }

// pairByKey pairs still-unmatched nodes of the two sides that share key(n).
// Within a key group, exact-id pairs match first (ids are the tiebreaker);
// the rest pair in graph order.
func pairByKey(
	sa, sb *diffSide,
	matchedA, matchedB map[string]bool,
	key func(s *diffSide, n *GraphNode) string,
) []nodePair {
	groupA := map[string][]*GraphNode{}
	groupB := map[string][]*GraphNode{}
	var keys []string
	seenKey := map[string]bool{}
	for _, n := range sa.g.Nodes {
		if matchedA[n.ID] {
			continue
		}
		k := key(sa, n)
		groupA[k] = append(groupA[k], n)
		if !seenKey[k] {
			seenKey[k] = true
			keys = append(keys, k)
		}
	}
	for _, n := range sb.g.Nodes {
		if matchedB[n.ID] {
			continue
		}
		k := key(sb, n)
		groupB[k] = append(groupB[k], n)
		if !seenKey[k] {
			seenKey[k] = true
			keys = append(keys, k)
		}
	}
	var pairs []nodePair
	for _, k := range keys {
		as, bs := groupA[k], groupB[k]
		if len(as) == 0 || len(bs) == 0 {
			continue
		}
		usedA := make([]bool, len(as))
		usedB := make([]bool, len(bs))
		// pass 1: exact id matches
		for i, an := range as {
			for j, bn := range bs {
				if !usedB[j] && an.ID == bn.ID {
					usedA[i], usedB[j] = true, true
					pairs = append(pairs, nodePair{an, bn})
					break
				}
			}
		}
		// pass 2: remaining, in graph order
		j := 0
		for i, an := range as {
			if usedA[i] {
				continue
			}
			for j < len(bs) && usedB[j] {
				j++
			}
			if j >= len(bs) {
				break
			}
			usedA[i], usedB[j] = true, true
			pairs = append(pairs, nodePair{an, bs[j]})
		}
		for i, an := range as {
			if usedA[i] {
				matchedA[an.ID] = true
			}
		}
		for j, bn := range bs {
			if usedB[j] {
				matchedB[bn.ID] = true
			}
		}
	}
	return pairs
}

// DiffGraphs computes the semantic delta from old graph `a` to new graph `b`.
func DiffGraphs(a, b *Graph) *GraphDiff {
	sa, sb := newDiffSide(a), newDiffSide(b)
	d := &GraphDiff{}
	matchedA, matchedB := map[string]bool{}, map[string]bool{}

	// Pass 1: full identity — context :: kind|fn|label.
	fullKey := func(s *diffSide, n *GraphNode) string {
		return s.ctx[n.ID] + "::" + n.Kind + "|" + n.Fn + "|" + n.Label
	}
	pairs := pairByKey(sa, sb, matchedA, matchedB, fullKey)

	// Pass 2: label-blind — context :: kind|fn. Leftovers pairing here CHANGED
	// their label (a guard/type edit), not their place in the picture.
	softKey := func(s *diffSide, n *GraphNode) string {
		return s.ctx[n.ID] + "::" + n.Kind + "|" + n.Fn
	}
	changedPairs := pairByKey(sa, sb, matchedA, matchedB, softKey)
	pairs = append(pairs, changedPairs...)
	for _, p := range changedPairs {
		if p.a.Label != p.b.Label {
			d.ChangedNodes = append(d.ChangedNodes, NodeDelta{
				Op: "changed", ID: p.b.ID, Kind: p.b.Kind, Fn: p.b.Fn,
				Label: p.b.Label, OldLabel: p.a.Label, Context: sb.context(p.b),
			})
		}
	}
	d.UnchangedNodes = len(pairs) - len(d.ChangedNodes)

	// Unmatched nodes are the adds/removes.
	for _, n := range a.Nodes {
		if !matchedA[n.ID] {
			d.RemovedNodes = append(d.RemovedNodes, NodeDelta{
				Op: "removed", ID: n.ID, Kind: n.Kind, Fn: n.Fn, Label: n.Label, Context: sa.context(n),
			})
		}
	}
	for _, n := range b.Nodes {
		if !matchedB[n.ID] {
			d.AddedNodes = append(d.AddedNodes, NodeDelta{
				Op: "added", ID: n.ID, Kind: n.Kind, Fn: n.Fn, Label: n.Label, Context: sb.context(n),
			})
		}
	}

	// Edge comparison: rewrite each edge's endpoints to the MATCH tokens (a
	// matched pair shares one token on both sides), so an edge survives its
	// nodes' id shifts. Unmatched endpoints get side-local tokens (never equal).
	tokA, tokB := map[string]string{}, map[string]string{}
	for i, p := range pairs {
		t := "m" + itoa(i)
		tokA[p.a.ID], tokB[p.b.ID] = t, t
	}
	tok := func(m map[string]string, side string, id string) string {
		if t, ok := m[id]; ok {
			return t
		}
		return side + ":" + id
	}
	edgeKey := func(m map[string]string, side string, e *GraphEdge) string {
		return tok(m, side, e.Source) + ">" + tok(m, side, e.Target) + "|" +
			e.SourceHandle + "|" + e.TargetHandle + "|" + e.Type + "|" + e.Kind + "|" + e.Access + "|" + e.Label
	}
	remaining := map[string][]*GraphEdge{}
	for _, e := range a.Edges {
		k := edgeKey(tokA, "a", e)
		remaining[k] = append(remaining[k], e)
	}
	var addedRaw []*GraphEdge
	for _, e := range b.Edges {
		k := edgeKey(tokB, "b", e)
		if q := remaining[k]; len(q) > 0 {
			remaining[k] = q[1:]
			d.UnchangedEdges++
			continue
		}
		addedRaw = append(addedRaw, e)
	}
	var removedRaw []*GraphEdge
	var remKeys []string
	for k := range remaining {
		remKeys = append(remKeys, k)
	}
	sort.Strings(remKeys) // deterministic order
	// keep original a-order for readability: collect in a.Edges order instead
	removedRaw = removedRaw[:0]
	stillA := map[*GraphEdge]bool{}
	for _, k := range remKeys {
		for _, e := range remaining[k] {
			stillA[e] = true
		}
	}
	for _, e := range a.Edges {
		if stillA[e] {
			removedRaw = append(removedRaw, e)
		}
	}

	// REWIRE detection: pair a removed edge with an added edge that kept one
	// end (same token + handle + kind) and moved the other.
	desc := func(s *diffSide, id string) string {
		if n := s.byID[id]; n != nil {
			return describeNode(n)
		}
		return id
	}
	usedAdd := map[*GraphEdge]bool{}
	for _, re := range removedRaw {
		var hit *GraphEdge
		what := ""
		for _, ae := range addedRaw {
			if usedAdd[ae] || ae.Kind != re.Kind || ae.Type != re.Type {
				continue
			}
			sameTarget := tok(tokA, "a", re.Target) == tok(tokB, "b", ae.Target) && re.TargetHandle == ae.TargetHandle
			sameSource := tok(tokA, "a", re.Source) == tok(tokB, "b", ae.Source) && re.SourceHandle == ae.SourceHandle
			if sameTarget && !sameSource {
				hit, what = ae, "source"
				break
			}
			if sameSource && !sameTarget {
				hit, what = ae, "target"
				break
			}
		}
		if hit == nil {
			d.RemovedEdges = append(d.RemovedEdges, EdgeDelta{
				Op: "removed", Source: desc(sa, re.Source), Target: desc(sa, re.Target),
				SourceHandle: re.SourceHandle, TargetHandle: re.TargetHandle,
				Type: re.Type, Kind: re.Kind, Access: re.Access,
			})
			continue
		}
		usedAdd[hit] = true
		was := desc(sa, re.Source)
		if what == "target" {
			was = desc(sa, re.Target)
		}
		d.RewiredEdges = append(d.RewiredEdges, EdgeDelta{
			Op: "rewired", Source: desc(sb, hit.Source), Target: desc(sb, hit.Target),
			SourceHandle: hit.SourceHandle, TargetHandle: hit.TargetHandle,
			Type: hit.Type, Kind: hit.Kind, Access: hit.Access,
			Was: was, What: what,
		})
	}
	for _, ae := range addedRaw {
		if usedAdd[ae] {
			continue
		}
		d.AddedEdges = append(d.AddedEdges, EdgeDelta{
			Op: "added", Source: desc(sb, ae.Source), Target: desc(sb, ae.Target),
			SourceHandle: ae.SourceHandle, TargetHandle: ae.TargetHandle,
			Type: ae.Type, Kind: ae.Kind, Access: ae.Access,
		})
	}
	return d
}

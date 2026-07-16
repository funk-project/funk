package funk

// Graph derivation — the Go port of funk-studio's build.ts. It derives a
// React-Flow-style node/edge graph from a function's PARSE structure (the same
// AST the `funk parse` JSON exposes and build.ts consumes). This is PHASE 1:
// UNEXPANDED graphs only — a call to another user function stays a leaf `call`
// node (no lib expansion). Node ids are AST paths ("b", "b.0", "b.t") so a live
// run event can map back to its node; the graph is DERIVED, never drawn.
//
// The reference spec is funk-studio/src/renderer/src/funk/build.ts; this mirrors
// its semantics (node ids/kinds, edge handles/types, data flags) so the two
// agree byte-for-byte on the structural fields.

import "strings"

// GraphNode is one derived node. Empty fields are omitted so the JSON matches
// build.ts's FlowNode.data (which never emits absent keys).
type GraphNode struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"` // call|cond|merge|terminal|atom|input|output|group|seed|item
	Label      string      `json:"label,omitempty"`
	Fn         string      `json:"fn,omitempty"`
	ParentNode string      `json:"parentNode,omitempty"`
	Engine     string      `json:"engine,omitempty"`
	OneSided   bool        `json:"oneSided,omitempty"`
	IsLoop     bool        `json:"isLoop,omitempty"`
	IsBranch   bool        `json:"isBranch,omitempty"`
	Branch     string      `json:"branch,omitempty"`
	LoopSource bool        `json:"loopSource,omitempty"`
	LoopTarget bool        `json:"loopTarget,omitempty"`
	InParams   []GraphPort `json:"inParams,omitempty"`
	OutParams  []GraphPort `json:"outParams,omitempty"`
}

// GraphPort is one box border port: `port` handle over its inner `node`.
type GraphPort struct {
	Port string `json:"port"`
	Node string `json:"node"`
}

// GraphEdge is one derived edge.
type GraphEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
	TargetHandle string `json:"targetHandle,omitempty"`
	Label        string `json:"label,omitempty"`
	Type         string `json:"type,omitempty"` // "loop" for a loop back-edge
	Kind         string `json:"kind"`           // data|control
	Access       string `json:"access"`         // call|ref
}

// Graph is the derived nodes + edges.
type Graph struct {
	Nodes []*GraphNode `json:"nodes"`
	Edges []*GraphEdge `json:"edges"`
}

// ── AST helpers mirroring build.ts's isForm/isAtom ──────────────────────────
// build.ts's isForm requires `Array.isArray(Args)`; a zero-arg form parses with
// Args == null (Go nil slice → JSON null), so it is NOT a form there. gForm
// preserves that: a Form with a nil Args slice is treated as a non-form.

func gForm(n Node) (Form, bool) {
	f, ok := n.(Form)
	return f, ok && f.Args != nil
}

func gAtom(n Node) (Atom, bool) {
	a, ok := n.(Atom)
	return a, ok
}

// asFormAny returns n as a Form regardless of its Args (for reading a binder's
// Head, matching build.ts's `'Head' in bind` checks on `(i)` binders).
func asFormAny(n Node) (Form, bool) {
	f, ok := n.(Form)
	return f, ok
}

func argAt(args []Node, i int) Node {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return nil
}

func lastSeg(head string) string {
	if i := strings.LastIndexByte(head, '.'); i >= 0 {
		return head[i+1:]
	}
	return head
}

// ── builder ─────────────────────────────────────────────────────────────────

type graphBuilder struct {
	lib   *Library
	fn    *Fn
	nodes []*GraphNode
	edges []*GraphEdge

	inputOrder []string          // input node ids, in port order
	inputIds   map[string]string // port name -> input node id
	locals     map[string]string // loop/let local name -> node id
	needsIds   map[string]string // needs.* path -> node id
	outByName  map[string]string // output port name -> output node id

	boxDepth         int
	compositeOutputs []string
	compOut          map[string]bool
}

type edgeOpts struct {
	label        string
	sourceHandle string
	targetHandle string
	typ          string
	access       string
}

func (b *graphBuilder) addNode(n *GraphNode) { b.nodes = append(b.nodes, n) }

func (b *graphBuilder) addEdge(source, target, kind string, opts edgeOpts) {
	tag := opts.label
	if tag == "" {
		tag = opts.typ
	}
	if tag == "" {
		tag = kind
	}
	access := opts.access
	if access == "" {
		access = "call"
	}
	b.edges = append(b.edges, &GraphEdge{
		ID:           "e:" + source + "->" + target + ":" + tag,
		Source:       source,
		Target:       target,
		SourceHandle: opts.sourceHandle,
		TargetHandle: opts.targetHandle,
		Label:        opts.label,
		Type:         opts.typ,
		Kind:         kind,
		Access:       access,
	})
}

func (b *graphBuilder) nodeByID(id string) *GraphNode {
	for _, n := range b.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (b *graphBuilder) removeEdgeAt(i int) {
	b.edges = append(b.edges[:i], b.edges[i+1:]...)
}

func (b *graphBuilder) resolveRef(name string) string {
	if id, ok := b.inputIds[name]; ok {
		return id
	}
	if id, ok := b.locals[name]; ok {
		return id
	}
	return ""
}

func (b *graphBuilder) needRef(name string) string {
	if id, ok := b.needsIds[name]; ok {
		return id
	}
	nid := "need." + itoa(len(b.needsIds))
	b.addNode(&GraphNode{ID: nid, Label: name, Kind: "input", Fn: name})
	b.needsIds[name] = nid
	return nid
}

func (b *graphBuilder) valueRef(name string) string {
	if id := b.resolveRef(name); id != "" {
		return id
	}
	if strings.HasPrefix(name, "needs.") {
		return b.needRef(name)
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// portLabel renders "name: type" (or just "name") for a port node label.
func portLabel(p Port) string {
	if p.Type != "" {
		return p.Name + ": " + p.Type
	}
	return p.Name
}

func (b *graphBuilder) addOutputs() []string {
	var ids []string
	for i, p := range b.fn.Out {
		id := "out." + itoa(i)
		b.addNode(&GraphNode{ID: id, Label: portLabel(p), Kind: "output", Fn: p.Name})
		b.outByName[p.Name] = id
		ids = append(ids, id)
	}
	return ids
}

// atomLabel renders an atom as a label token.
func atomLabel(n Node) string {
	if a, ok := n.(Atom); ok {
		if a.Kind == "str" {
			return "\"" + a.Value + "\""
		}
		return a.Value
	}
	return ""
}

var guardOps = map[string]string{
	"lt": "<", "lte": "≤", "gt": ">", "gte": "≥", "eq": "=", "neq": "≠",
	"add": "+", "sub": "−", "mul": "×", "div": "÷", "mod": "mod", "and": "and", "or": "or",
}

// renderGuard renders a condition/step expression as a compact title string.
func (b *graphBuilder) renderGuard(n Node, nested bool) string {
	if a, ok := n.(Atom); ok {
		return atomLabel(a)
	}
	f, ok := gForm(n)
	if !ok {
		return ""
	}
	op := guardOps[lastSeg(f.Head)]
	as := f.Args
	if op != "" && len(as) == 2 {
		s := b.renderGuard(as[0], true) + " " + op + " " + b.renderGuard(as[1], true)
		if nested {
			return "(" + s + ")"
		}
		return s
	}
	fn := lastSeg(f.Head)
	parts := make([]string, 0, len(as))
	for _, a := range as {
		parts = append(parts, b.renderGuard(a, false))
	}
	return fn + "(" + strings.Join(parts, ", ") + ")"
}

// orderedSet accumulates names in insertion order (mirrors a JS Set).
type orderedSet struct {
	seen  map[string]bool
	order []string
}

func newOrderedSet() *orderedSet { return &orderedSet{seen: map[string]bool{}} }
func (s *orderedSet) add(v string) {
	if !s.seen[v] {
		s.seen[v] = true
		s.order = append(s.order, v)
	}
}

// inputRefsIn collects function-input names referenced anywhere in an expr (a
// Form HEAD counts — `(f item)` calls THROUGH the Fn input `f`).
func (b *graphBuilder) inputRefsIn(n Node, acc *orderedSet) {
	if a, ok := gAtom(n); ok {
		if a.Kind == "id" {
			if _, has := b.inputIds[a.Value]; has {
				acc.add(a.Value)
			}
		}
		return
	}
	if f, ok := gForm(n); ok {
		if _, has := b.inputIds[f.Head]; has {
			acc.add(f.Head)
		}
		for _, a := range f.Args {
			b.inputRefsIn(a, acc)
		}
	}
}

// refsIn collects names that resolve to a node (input OR loop local).
func (b *graphBuilder) refsIn(n Node, acc *orderedSet) {
	if a, ok := gAtom(n); ok {
		if a.Kind == "id" && b.resolveRef(a.Value) != "" {
			acc.add(a.Value)
		}
		return
	}
	if f, ok := gForm(n); ok {
		if b.resolveRef(f.Head) != "" {
			acc.add(f.Head)
		}
		for _, a := range f.Args {
			b.refsIn(a, acc)
		}
	}
}

// dataArg wires an argument as data into parentId.
func (b *graphBuilder) dataArg(arg Node, path, parentID string, inBranch bool) {
	if _, ok := gForm(arg); ok {
		childID := b.walk(arg, path, inBranch)
		if childID != "" {
			b.addEdge(childID, parentID, "data", edgeOpts{})
		}
		return
	}
	if a, ok := gAtom(arg); ok && a.Kind == "id" {
		inID := b.valueRef(a.Value)
		if inID != "" {
			b.addEdge(inID, parentID, "data", edgeOpts{access: "ref"})
		}
	}
}

// ── DeriveGraph ─────────────────────────────────────────────────────────────

// DeriveGraph derives the (unexpanded) graph for a function. `lib` is accepted
// for parity with the reference (and the phase-2 lib-expansion path) but, in
// phase 1, calls to other user functions stay leaf `call` nodes.
func DeriveGraph(lib *Library, f *Fn) (*Graph, error) {
	b := &graphBuilder{
		lib:       lib,
		fn:        f,
		inputIds:  map[string]string{},
		locals:    map[string]string{},
		needsIds:  map[string]string{},
		outByName: map[string]string{},
		compOut:   map[string]bool{},
	}

	// Input ports become `input` source nodes.
	for i, p := range f.In {
		id := "in." + itoa(i)
		b.addNode(&GraphNode{ID: id, Label: portLabel(p), Kind: "input", Fn: p.Name})
		b.inputIds[p.Name] = id
		b.inputOrder = append(b.inputOrder, id)
	}

	// An `alias TARGET` fn delegates to another function. build.ts sees a
	// body-less block with an `alias` field; funk's Finalize() has already
	// rewritten Body to a call, so detect the alias by the retained Alias field.
	if f.Alias != "" {
		b.addNode(&GraphNode{ID: "alias", Label: "→ " + f.Alias, Kind: "call", Fn: f.Alias})
		for _, inID := range b.inputOrder {
			b.addEdge(inID, "alias", "data", edgeOpts{})
		}
		return &Graph{Nodes: b.nodes, Edges: b.edges}, nil
	}

	if f.Body == nil {
		// Body-less: atomic (engine + src) or a signature-only interface.
		outputIDs := b.addOutputs()
		if f.Engine != "" {
			b.addNode(&GraphNode{ID: "b", Label: f.Name, Kind: "atom", Fn: f.Name, Engine: f.Engine})
			for _, inID := range b.inputOrder {
				b.addEdge(inID, "b", "data", edgeOpts{})
			}
			for _, outID := range outputIDs {
				b.addEdge("b", outID, "data", edgeOpts{})
			}
		}
		return &Graph{Nodes: b.nodes, Edges: b.edges}, nil
	}

	// Composite: outputs exist here too. Derive the body, then wire results in.
	b.compositeOutputs = b.addOutputs()
	for _, o := range b.compositeOutputs {
		b.compOut[o] = true
	}

	rootID := b.walk(f.Body, "b", false)

	// Wire return/exit results into the output port(s): `return → r`. A terminal
	// inside a box (parentNode set) is a branch value — skip (the box emits).
	for _, n := range b.nodes {
		if n.Kind == "terminal" && (n.Fn == "return" || n.Fn == "exit") && n.ParentNode == "" {
			for _, o := range b.compositeOutputs {
				b.addEdge(n.ID, o, "data", edgeOpts{})
			}
		}
	}

	// IMPLICIT RESULT: a bare-expression body flows to the output implicitly —
	// wire the root in, but only when NO output got fed explicitly.
	if rootID != "" && len(b.compositeOutputs) > 0 {
		fed := map[string]bool{}
		for _, e := range b.edges {
			if e.Type != "loop" {
				fed[e.Target] = true
			}
		}
		anyFed := false
		for _, o := range b.compositeOutputs {
			if fed[o] {
				anyFed = true
				break
			}
		}
		if !anyFed {
			for _, o := range b.compositeOutputs {
				b.addEdge(rootID, o, "data", edgeOpts{})
			}
		}
	}

	// Drop a spurious `box → output` when the box's value is also consumed by a
	// real downstream node (the value reaches the output THROUGH the consumer).
	byIDFinal := map[string]*GraphNode{}
	for _, n := range b.nodes {
		byIDFinal[n.ID] = n
	}
	for _, g := range b.nodes {
		if g.Kind != "group" {
			continue
		}
		feedsConsumer := false
		for _, e := range b.edges {
			if e.Source == g.ID && e.Type != "loop" && e.SourceHandle != "fork" && !b.compOut[e.Target] {
				tn := byIDFinal[e.Target]
				if tn == nil || tn.ParentNode != g.ID {
					feedsConsumer = true
					break
				}
			}
		}
		if !feedsConsumer {
			continue
		}
		for k := len(b.edges) - 1; k >= 0; k-- {
			e := b.edges[k]
			if e.Source == g.ID && e.SourceHandle == "out" && b.compOut[e.Target] {
				b.removeEdgeAt(k)
			}
		}
	}

	// Composite-consumer out:0 post-pass (inert in phase 1 — nothing sets
	// OutParams without lib expansion, but kept for parity).
	for _, g := range b.nodes {
		if len(g.OutParams) == 0 {
			continue
		}
		for _, e := range b.edges {
			if e.Source == g.ID && e.SourceHandle == "" {
				e.SourceHandle = "out:0"
			}
		}
	}

	return &Graph{Nodes: b.nodes, Edges: b.edges}, nil
}

// finishLoopBox connects a loop box to the outputs, captures external reads
// through its `in` ball, and adopts its inner nodes.
func (b *graphBuilder) finishLoopBox(boxID string, boxStart int) {
	boxNodeIDs := map[string]bool{}
	for _, n := range b.nodes[boxStart:] {
		boxNodeIDs[n.ID] = true
	}
	emittedTo := newOrderedSet()
	for i := len(b.edges) - 1; i >= 0; i-- {
		e := b.edges[i]
		if e.Type != "loop" && boxNodeIDs[e.Source] && b.compOut[e.Target] {
			emittedTo.add(e.Target)
			b.removeEdgeAt(i)
		}
	}
	for _, o := range emittedTo.order {
		b.addEdge(boxID, o, "data", edgeOpts{sourceHandle: "out"})
	}

	// INPUT capture: an external → internal data edge must enter via the box.
	capturedFrom := newOrderedSet()
	for i := len(b.edges) - 1; i >= 0; i-- {
		e := b.edges[i]
		if e.Type == "loop" || e.Kind == "control" {
			continue
		}
		if boxNodeIDs[e.Target] && !boxNodeIDs[e.Source] && e.Source != boxID {
			capturedFrom.add(e.Source)
			b.removeEdgeAt(i)
		}
	}
	// capturedFrom is iterated in reverse-discovery order in build.ts? No — a JS
	// Set iterates in insertion order, and inserts happen as edges are visited
	// from the END. Mirror that: preserve the reverse-index insertion order.
	for _, src := range capturedFrom.order {
		exists := false
		for _, e := range b.edges {
			if e.Source == src && e.Target == boxID {
				exists = true
				break
			}
		}
		if !exists {
			b.addEdge(src, boxID, "data", edgeOpts{targetHandle: "in"})
		}
	}

	for k := boxStart; k < len(b.nodes); k++ {
		if b.nodes[k].ParentNode == "" {
			b.nodes[k].ParentNode = boxID
		}
	}
}

// branchNode derives an if-branch INSIDE the box, tagged true/false, returning
// the node representing the branch's produced value.
func (b *graphBuilder) branchNode(node Node, id string, which string) string {
	tag := func(nid string) string {
		if nid != "" {
			if n := b.nodeByID(nid); n != nil {
				n.Branch = which
			}
		}
		return nid
	}
	bare := func(val Node, nid, fn string) string {
		lbl := fn
		if val != nil {
			lbl = atomLabel(val)
		}
		b.addNode(&GraphNode{ID: nid, Label: lbl, Kind: "terminal", Fn: fn, Branch: which})
		if a, ok := gAtom(val); ok && a.Kind == "id" {
			if src := b.valueRef(a.Value); src != "" {
				b.addEdge(src, nid, "data", edgeOpts{access: "ref"})
			}
		}
		return nid
	}

	if f, ok := gForm(node); ok && f.Head == "flush" {
		var chosen *Form
		for _, x := range f.Args {
			if xf, ok := gForm(x); ok && len(xf.Args) == 1 {
				chosen = &xf
				break
			}
		}
		if chosen != nil {
			val := argAt(chosen.Args, 0)
			if _, ok := gForm(val); ok {
				return tag(b.walk(val, id+"."+chosen.Head, true))
			}
			return bare(val, id+"."+chosen.Head, chosen.Head)
		}
		return ""
	}
	if f, ok := gForm(node); ok && (f.Head == "return" || f.Head == "exit") {
		arg := argAt(f.Args, 0)
		if _, ok := gForm(arg); ok {
			return tag(b.walk(arg, id, true))
		}
		return bare(arg, id, f.Head)
	}
	// A control box (nested if / loop) AS a branch: the whole box is the branch.
	if f, ok := gForm(node); ok &&
		(f.Head == "if" || f.Head == "on-error" || f.Head == "while" || f.Head == "for-each" || f.Head == "each") {
		r := b.walk(node, id, true)
		boxID := id + ".box"
		if box := b.nodeByID(boxID); box != nil {
			box.Branch = which
			for k := len(b.edges) - 1; k >= 0; k-- {
				if b.edges[k].Source == boxID && b.compOut[b.edges[k].Target] {
					b.removeEdgeAt(k)
				}
			}
			return boxID
		}
		return r
	}
	// A bare passthrough (an outer local/input) — a small terminal.
	if a, ok := gAtom(node); ok {
		fn := "value"
		if a.Kind == "id" {
			fn = a.Value
		}
		return bare(node, id, fn)
	}
	return tag(b.walk(node, id, true))
}

// walk emits the node(s) for a Form and returns its node id ("" for non-forms).
func (b *graphBuilder) walk(node Node, id string, inBranch bool) string {
	f, ok := gForm(node)
	if !ok {
		return ""
	}
	head := f.Head
	args := f.Args

	if head == "if" || head == "on-error" {
		return b.walkIf(f, id, inBranch)
	}

	if head == "return" || head == "exit" {
		arg := argAt(args, 0)
		edgeToInput := false
		if a, ok := gAtom(arg); ok && a.Kind == "id" {
			if _, has := b.inputIds[a.Value]; has && !inBranch {
				edgeToInput = true
			}
		}
		label := head
		if a, ok := gAtom(arg); ok && !edgeToInput {
			label = head + " " + atomLabel(a)
		}
		b.addNode(&GraphNode{ID: id, Label: label, Kind: "terminal", Fn: head})
		if arg != nil {
			if _, isF := gForm(arg); isF || edgeToInput {
				b.dataArg(arg, id+".0", id, inBranch)
			}
		}
		return id
	}

	if head == "set" {
		nameNode := argAt(args, 0)
		port := ""
		if a, ok := gAtom(nameNode); ok {
			port = a.Value
		}
		if port != "" {
			if outID, has := b.outByName[port]; has {
				b.dataArg(argAt(args, 1), id+".v", outID, inBranch)
			}
		}
		return ""
	}

	if head == "flush" {
		for _, a := range args {
			if af, ok := gForm(a); ok && len(af.Args) == 1 {
				if outID, has := b.outByName[af.Head]; has {
					b.dataArg(argAt(af.Args, 0), id+"."+af.Head, outID, inBranch)
				}
			}
		}
		return ""
	}

	if head == "do" {
		b.addNode(&GraphNode{ID: id, Label: "do", Kind: "call", Fn: "do"})
		for i, a := range args {
			childID := b.walk(a, id+"."+itoa(i), inBranch)
			if childID != "" {
				b.addEdge(id, childID, "control", edgeOpts{})
			}
		}
		return id
	}

	if head == "let" {
		bind := argAt(args, 0)
		bodyExpr := argAt(args, 1)
		if bf, ok := gForm(bind); ok {
			parts := bf.Args
			var value Node
			if len(parts) > 0 {
				value = parts[len(parts)-1]
			}
			names := []string{bf.Head}
			if len(parts) > 0 {
				for _, a := range parts[:len(parts)-1] {
					if at, ok := gAtom(a); ok && at.Kind == "id" {
						names = append(names, at.Value)
					}
				}
			}
			srcID := ""
			if _, ok := gForm(value); ok {
				srcID = b.walk(value, id+".v", inBranch)
			} else if at, ok := gAtom(value); ok && at.Kind == "id" {
				srcID = b.valueRef(at.Value)
			}
			type shadow struct {
				name string
				old  string
				had  bool
			}
			shadows := make([]shadow, 0, len(names))
			for _, n := range names {
				old, had := b.locals[n]
				shadows = append(shadows, shadow{n, old, had})
			}
			if srcID != "" {
				for _, n := range names {
					b.locals[n] = srcID
				}
			}
			r := b.walk(bodyExpr, id+".b", inBranch)
			for _, sh := range shadows {
				if !sh.had {
					delete(b.locals, sh.name)
				} else {
					b.locals[sh.name] = sh.old
				}
			}
			return r
		}
		return b.walk(bodyExpr, id+".b", inBranch)
	}

	if head == "while" {
		return b.walkWhile(f, id, inBranch)
	}

	if head == "for-each" || head == "each" {
		return b.walkForEach(f, id, inBranch)
	}

	if head == "yield" {
		b.addNode(&GraphNode{ID: id, Label: "yield", Kind: "terminal", Fn: "yield"})
		b.dataArg(argAt(args, 0), id+".0", id, inBranch)
		for _, o := range b.compositeOutputs {
			b.addEdge(id, o, "data", edgeOpts{})
		}
		return id
	}

	// PHASE 1: a call to another user function stays a leaf `call` node (no lib
	// expansion). Generic call (f arg…): each Form arg feeds a data edge into f.
	b.addNode(&GraphNode{ID: id, Label: head, Kind: "call", Fn: head})
	fnSrc := b.resolveRef(head)
	if fnSrc != "" {
		_, isLocal := b.locals[head]
		if !inBranch || isLocal {
			b.addEdge(fnSrc, id, "data", edgeOpts{access: "ref"})
		}
	}
	for i, a := range args {
		b.dataArg(a, id+"."+itoa(i), id, inBranch)
	}
	return id
}

// walkIf derives an if/on-error as a decision box.
func (b *graphBuilder) walkIf(f Form, id string, inBranch bool) string {
	args := f.Args
	isErr := f.Head == "on-error"
	var cond Node
	var thenArg Node
	if isErr {
		thenArg = argAt(args, 0)
	} else {
		cond = argAt(args, 0)
		thenArg = argAt(args, 1)
	}
	boxID := id + ".box"
	boxLabel := "if"
	if isErr {
		boxLabel = "on error"
	}
	b.addNode(&GraphNode{ID: boxID, Label: boxLabel, Kind: "group", Fn: f.Head, IsBranch: true})
	boxStart := len(b.nodes)
	b.boxDepth++

	hasElse := len(args) > 2
	decID := id + ".cond"
	decLabel := "ok?"
	if !isErr {
		decLabel = b.renderGuard(cond, false)
		if decLabel == "" {
			decLabel = "if"
		}
	}
	b.addNode(&GraphNode{ID: decID, Label: decLabel, Kind: "cond", Fn: f.Head, OneSided: !hasElse})
	thenR := b.branchNode(thenArg, id+".t", "true")
	elseR := ""
	if hasElse {
		elseR = b.branchNode(argAt(args, 2), id+".e", "false")
	}
	mergeID := id + ".merge"
	if hasElse {
		b.addNode(&GraphNode{ID: mergeID, Label: "merge", Kind: "merge", Fn: "merge"})
	}
	b.boxDepth--

	// The condition reads its inputs: a data edge from the source to the decision.
	condRefs := newOrderedSet()
	b.refsIn(cond, condRefs)
	for _, name := range condRefs.order {
		if src := b.resolveRef(name); src != "" {
			b.addEdge(src, decID, "data", edgeOpts{})
		}
	}

	tPre := id + ".t"
	ePre := id + ".e"
	whichOf := func(nid string) string {
		if strings.HasPrefix(nid, tPre) {
			return "true"
		}
		if strings.HasPrefix(nid, ePre) {
			return "false"
		}
		return ""
	}
	entry := map[string]string{"true": thenR, "false": elseR}
	exit := map[string]string{"true": thenR, "false": elseR}
	boxNodeIDs := map[string]bool{}
	for _, n := range b.nodes[boxStart:] {
		boxNodeIDs[n.ID] = true
	}

	// EXIT: an inside → function-output edge — its source is the branch's exit.
	for k := len(b.edges) - 1; k >= 0; k-- {
		e := b.edges[k]
		if e.Type == "loop" || e.Kind == "control" {
			continue
		}
		if boxNodeIDs[e.Source] && b.compOut[e.Target] {
			if w := whichOf(e.Source); w != "" {
				exit[w] = e.Source
			}
			b.removeEdgeAt(k)
		}
	}

	// ENTRY: walk back from each branch's exit through same-branch data edges.
	for _, w := range []string{"true", "false"} {
		root := exit[w]
		if root == "" {
			continue
		}
		for guard := 0; guard <= len(boxNodeIDs); guard++ {
			predSrc := ""
			for _, e := range b.edges {
				if e.Kind == "control" || e.Type == "loop" {
					continue
				}
				if e.Target != root || e.TargetHandle == "join" {
					continue
				}
				if !boxNodeIDs[e.Source] || whichOf(e.Source) != w || e.Source == root {
					continue
				}
				if strings.HasPrefix(e.Source, root+"::") || strings.HasPrefix(root, e.Source+"::") {
					continue
				}
				predSrc = e.Source
				break
			}
			if predSrc == "" {
				break
			}
			root = predSrc
		}
		entry[w] = root
	}

	// Wire the diamond: decision forks to each entry; each exit flows to MERGE
	// (two-sided) or straight to the box (one-sided).
	for _, w := range []string{"true", "false"} {
		if entry[w] != "" {
			b.addEdge(decID, entry[w], "control", edgeOpts{sourceHandle: w})
		}
		if exit[w] != "" {
			if hasElse {
				b.addEdge(exit[w], mergeID, "data", edgeOpts{})
			} else {
				b.addEdge(exit[w], boxID, "data", edgeOpts{targetHandle: "join"})
			}
		}
	}
	if hasElse {
		b.addEdge(mergeID, boxID, "data", edgeOpts{targetHandle: "join"})
	}
	if !inBranch {
		for _, o := range b.compositeOutputs {
			b.addEdge(boxID, o, "data", edgeOpts{sourceHandle: "out"})
		}
	}

	// UNIFY: every external value a reader consumes enters through a per-value
	// input port — `source → box[in:i]`, then `box[pin:i] → reader`.
	inPortOf := map[string]string{}
	var portOrder []string
	for i := 0; i < len(b.edges); i++ {
		e := b.edges[i]
		if e.Kind == "control" || e.Type == "loop" {
			continue
		}
		if !boxNodeIDs[e.Target] || boxNodeIDs[e.Source] || e.Source == boxID {
			continue
		}
		port, has := inPortOf[e.Source]
		if !has {
			port = "in:" + itoa(len(inPortOf))
			inPortOf[e.Source] = port
			portOrder = append(portOrder, port)
			b.addEdge(e.Source, boxID, "data", edgeOpts{targetHandle: port})
		}
		e.Source = boxID
		e.SourceHandle = "p" + port
	}
	if len(inPortOf) > 0 {
		if box := b.nodeByID(boxID); box != nil {
			var params []GraphPort
			for _, port := range portOrder {
				node := ""
				// prefer the condition if it reads this value, else its first reader.
				for _, e := range b.edges {
					if e.Source == boxID && e.SourceHandle == "p"+port && e.Target == decID {
						node = e.Target
						break
					}
				}
				if node == "" {
					for _, e := range b.edges {
						if e.Source == boxID && e.SourceHandle == "p"+port {
							node = e.Target
							break
						}
					}
				}
				if node == "" {
					node = decID
				}
				params = append(params, GraphPort{Port: port, Node: node})
			}
			box.InParams = params
		}
	}

	for k := boxStart; k < len(b.nodes); k++ {
		if b.nodes[k].ParentNode == "" {
			b.nodes[k].ParentNode = boxID
		}
	}
	return boxID
}

// walkWhile derives a while loop as a labelled box.
func (b *graphBuilder) walkWhile(f Form, id string, inBranch bool) string {
	args := f.Args
	bind := argAt(args, 0)
	stateName := "s"
	var init Node
	if bf, ok := gForm(bind); ok {
		stateName = bf.Head
		init = argAt(bf.Args, 0)
	}
	cond := argAt(args, 1)
	boxID := id + ".box"
	label := strings.TrimSpace("while " + b.renderGuard(cond, false))
	b.addNode(&GraphNode{ID: boxID, Label: label, Kind: "group", Fn: "while", IsLoop: true})
	boxStart := len(b.nodes)

	regID := id + ".s"
	regLabel := stateName
	if a, ok := gAtom(init); ok {
		regLabel = stateName + " = " + atomLabel(a)
	}
	b.addNode(&GraphNode{ID: regID, Label: regLabel, Kind: "seed", Fn: stateName})
	if _, ok := gForm(init); ok {
		if c := b.walk(init, id+".i", inBranch); c != "" {
			b.addEdge(c, regID, "data", edgeOpts{})
		}
	}

	shadowed, had := b.locals[stateName]
	b.locals[stateName] = regID
	b.boxDepth++
	stepTop := b.walk(argAt(args, 2), id+".step", inBranch)
	b.boxDepth--
	if stepTop != "" {
		b.addEdge(stepTop, regID, "data", edgeOpts{
			label:        "next",
			typ:          "loop",
			sourceHandle: "loop-out",
			targetHandle: "loop-in",
		})
		if sn := b.nodeByID(stepTop); sn != nil {
			sn.LoopSource = true
		}
		if rn := b.nodeByID(regID); rn != nil {
			rn.LoopTarget = true
		}
	}
	if !had {
		delete(b.locals, stateName)
	} else {
		b.locals[stateName] = shadowed
	}

	condInputs := newOrderedSet()
	b.inputRefsIn(cond, condInputs)
	for _, name := range condInputs.order {
		if inID, ok := b.inputIds[name]; ok {
			b.addEdge(inID, boxID, "data", edgeOpts{targetHandle: "in"})
		}
	}

	for _, o := range b.compositeOutputs {
		b.addEdge(regID, o, "data", edgeOpts{})
	}

	b.finishLoopBox(boxID, boxStart)
	return boxID
}

// walkForEach derives a for-each/each loop as a labelled box.
func (b *graphBuilder) walkForEach(f Form, id string, inBranch bool) string {
	args := f.Args
	coll := argAt(args, 0)
	bind := argAt(args, 1)
	itemName := "item"
	if bf, ok := asFormAny(bind); ok && bf.Head != "" {
		itemName = bf.Head
	}
	renderColl := func(c Node) string {
		if cf, ok := gForm(c); ok && cf.Head == "range" && len(cf.Args) == 2 {
			return b.renderGuard(cf.Args[0], false) + ".." + b.renderGuard(cf.Args[1], false)
		}
		return b.renderGuard(c, false)
	}
	boxID := id + ".box"
	label := strings.TrimSpace("for each " + itemName + " in " + renderColl(coll))
	b.addNode(&GraphNode{ID: boxID, Label: label, Kind: "group", Fn: "for-each", IsLoop: true})
	boxStart := len(b.nodes)

	itemID := id + ".item"
	b.addNode(&GraphNode{ID: itemID, Label: itemName, Kind: "item", Fn: itemName})

	shadowed, had := b.locals[itemName]
	b.locals[itemName] = itemID
	b.boxDepth++
	b.walk(argAt(args, 2), id+".b", inBranch)
	b.boxDepth--
	if !had {
		delete(b.locals, itemName)
	} else {
		b.locals[itemName] = shadowed
	}

	// the "next" carry: source it from the node that feeds an output.
	boxNodeIDs := map[string]bool{}
	for _, n := range b.nodes[boxStart:] {
		boxNodeIDs[n.ID] = true
	}
	emitSource := ""
	for _, e := range b.edges {
		if b.compOut[e.Target] && boxNodeIDs[e.Source] && e.Source != itemID {
			emitSource = e.Source
			break
		}
	}
	if emitSource != "" {
		b.addEdge(emitSource, itemID, "data", edgeOpts{
			label:        "next",
			typ:          "loop",
			sourceHandle: "loop-out",
			targetHandle: "loop-in",
		})
		if sn := b.nodeByID(emitSource); sn != nil {
			sn.LoopSource = true
		}
		if itn := b.nodeByID(itemID); itn != nil {
			itn.LoopTarget = true
		}
	}

	collInputs := newOrderedSet()
	b.inputRefsIn(coll, collInputs)
	for _, name := range collInputs.order {
		if inID, ok := b.inputIds[name]; ok {
			b.addEdge(inID, boxID, "data", edgeOpts{targetHandle: "in"})
		}
	}

	b.finishLoopBox(boxID, boxStart)
	return boxID
}

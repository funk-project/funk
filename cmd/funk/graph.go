package main

import (
	"encoding/json"
	"fmt"

	"github.com/funk-project/funk/internal/funk"
)

// cmdGraph renders the graph a composite function derives from its code — you
// never draw it; funk derives it. Makes "the plan is a structured artifact"
// visible. `--json` emits the tree with stable node ids (the AST Pos), the same
// ids a live TraceEvent carries — so an IDE can map an event to its exact node.
func cmdGraph(args []string) error {
	files, rest := takeFlag(args, "-f")
	asJSON, rest := takeBool(rest, "--json")
	if len(rest) != 1 {
		return fmt.Errorf("graph: usage: funk graph [-f path] [--json] <fn>")
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	f, ok := lib.Lookup(rest[0])
	if !ok {
		return fmt.Errorf("unknown function %q", rest[0])
	}
	if asJSON {
		g := graphJSON{Fn: f.Name, Address: f.Address(), Atomic: !f.Composite()}
		if f.Composite() {
			n := jsonNode(f.Body)
			g.Body = &n
		}
		b, _ := json.MarshalIndent(g, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Println(signature(f))
	if !f.Composite() {
		fmt.Printf("  (atomic — engine %s, no derived graph)\n", f.Engine)
		return nil
	}
	renderNode(f.Body, "", true)
	return nil
}

// graphJSON is the machine-readable graph an IDE consumes (matches TraceEvent.Node).
type graphJSON struct {
	Fn      string     `json:"fn"`
	Address string     `json:"address"`
	Atomic  bool       `json:"atomic"`
	Body    *graphNode `json:"body,omitempty"`
}

type graphNode struct {
	ID       string      `json:"id"`   // stable node id = AST Pos "line:col"
	Kind     string      `json:"kind"` // "form" | "atom"
	Head     string      `json:"head,omitempty"`
	Value    string      `json:"value,omitempty"`
	Children []graphNode `json:"children,omitempty"`
}

func jsonNode(n funk.Node) graphNode {
	switch t := n.(type) {
	case funk.Form:
		g := graphNode{ID: t.Pos.String(), Kind: "form", Head: t.Head}
		for _, a := range t.Args {
			g.Children = append(g.Children, jsonNode(a))
		}
		return g
	case funk.Atom:
		return graphNode{ID: t.Pos.String(), Kind: "atom", Value: t.Value}
	}
	return graphNode{}
}

func renderNode(n funk.Node, prefix string, last bool) {
	branch, next := "├─ ", "│  "
	if last {
		branch, next = "└─ ", "   "
	}
	switch t := n.(type) {
	case funk.Form:
		head := t.Head
		if head == "" {
			head = "()"
		}
		fmt.Printf("%s%s%s\n", prefix, branch, label(head))
		for i, a := range t.Args {
			renderNode(a, prefix+next, i == len(t.Args)-1)
		}
	case funk.Atom:
		v := t.Value
		if t.Kind == "str" {
			v = "\"" + v + "\""
		}
		fmt.Printf("%s%s%s\n", prefix, branch, v)
	}
}

// label annotates the core forms so the structure reads clearly.
func label(head string) string {
	switch head {
	case "if":
		return "if ⟨cond⟩ ⟨then⟩ ⟨else⟩"
	case "flush", "exit", "break", "continue":
		return head + " ◂ terminal"
	case "set", "do", "let", "while", "for-each", "each", "yield",
		"map", "filter", "scan", "merge", "take", "collect", "window",
		"range", "nats", "tick", "repeat", "on-error", "retry":
		return head + " ◂ form"
	default:
		return head + "()"
	}
}

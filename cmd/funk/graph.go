package main

import (
	"encoding/json"
	"fmt"

	"github.com/funk-project/funk/internal/funk"
)

// cmdGraph renders the graph a function derives from its code — you never draw
// it; funk derives it. By default it prints the DERIVED node/edge graph as JSON
// ({"nodes":[…],"edges":[…]}), the Go port of the IDE's build.ts (node ids are
// AST paths — "b", "b.0", "b.t"). `--tree` keeps the older human-readable AST
// tree of the composing expression.
func cmdGraph(args []string) error {
	files, rest := takeFlag(args, "-f")
	asTree, rest := takeBool(rest, "--tree")
	if len(rest) != 1 {
		return fmt.Errorf("graph: usage: funk graph [-f path] [--tree] <fn>")
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	f, ok := lib.Lookup(rest[0])
	if !ok {
		return fmt.Errorf("unknown function %q", rest[0])
	}
	// Default: the DERIVED node/edge graph (the Go port of build.ts). `--tree`
	// keeps the older human-readable AST tree.
	if !asTree {
		g, err := funk.DeriveGraph(lib, f)
		if err != nil {
			return err
		}
		// Guarantee non-null arrays so consumers can iterate unconditionally.
		if g.Nodes == nil {
			g.Nodes = []*funk.GraphNode{}
		}
		if g.Edges == nil {
			g.Edges = []*funk.GraphEdge{}
		}
		b, _ := json.Marshal(g)
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

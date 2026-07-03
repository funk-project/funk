package main

import (
	"fmt"

	"github.com/funk-project/funk/internal/funk"
)

// cmdGraph renders the graph a composite function derives from its code — you
// never draw it; funk derives it. Makes "the plan is a structured artifact"
// visible.
func cmdGraph(args []string) error {
	files, rest := takeFlag(args, "-f")
	if len(rest) != 1 {
		return fmt.Errorf("graph: usage: funk graph [-f path] <fn>")
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	f, ok := lib.Lookup(rest[0])
	if !ok {
		return fmt.Errorf("unknown function %q", rest[0])
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
	case "return", "exit", "break", "continue":
		return head + " ◂ terminal"
	case "do", "let", "while", "for-each", "each", "yield",
		"map", "filter", "scan", "merge", "take", "collect", "window",
		"range", "nats", "tick", "repeat", "on-error", "retry":
		return head + " ◂ form"
	default:
		return head + "()"
	}
}

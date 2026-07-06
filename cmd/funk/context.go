package main

import (
	"fmt"
	"strings"
)

// cmdContext assembles a paste-ready brief for an AI to write funk for a task:
// the primer (the language) + the existing functions worth reusing (found by
// searching the library for the task). This is the "reuse before you write" loop
// made concrete — the AI supplies the semantics, funk supplies the rules and the
// inventory.
func cmdContext(args []string) error {
	files, rest := takeFlag(args, "-f")
	if len(rest) == 0 {
		return fmt.Errorf("context: usage: funk context [-f path] <task description>")
	}
	task := strings.Join(rest, " ")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}

	fmt.Print(primer)
	fmt.Printf("\n---\n\n# Your task\n\n%s\n\n", task)

	rows := searchResults(lib, task, 12)
	if len(rows) > 0 {
		fmt.Println("## Reuse these — don't reinvent them")
		fmt.Println()
		fmt.Println("The library already has these functions (found by searching for your task).")
		fmt.Println("Compose them with `use \"pkg\" as alias` before writing anything new:")
		fmt.Println()
		for _, r := range rows {
			label := r.Name
			if r.Display != "" && r.Display != r.Name {
				label = r.Display
			}
			fmt.Printf("- `%s` — %s\n  `%s`", r.Address, label, r.Signature)
			if r.Doc != "" {
				fmt.Printf(" · %s", r.Doc)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	fmt.Println("Write funk for the task, reusing the functions above wherever they fit.")
	fmt.Println("Then verify: `funk check` (types/wiring) and `funk test` (inline asserts).")
	return nil
}

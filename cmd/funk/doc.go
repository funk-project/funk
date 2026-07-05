package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/funk-project/funk/internal/funk"
)

// cmdDoc generates markdown reference docs for the loaded library — funk is
// self-describing, so the docs are derived, never hand-written.
func cmdDoc(args []string) error {
	files, rest := takeFlag(args, "-f")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	filter := ""
	if len(rest) > 0 {
		filter = rest[0]
	}

	byPkg := map[string][]*funk.Fn{}
	var pkgs []string
	for _, f := range lib.Fns {
		p := f.Package
		if p == "" {
			p = "(local)"
		}
		if _, ok := byPkg[p]; !ok {
			pkgs = append(pkgs, p)
		}
		byPkg[p] = append(byPkg[p], f)
	}
	sort.Strings(pkgs)

	fmt.Printf("# funk — standard library\n\n%d functions across %d packages.\n", len(lib.Fns), len(pkgs))
	for _, p := range pkgs {
		if filter != "" && !strings.Contains(p, filter) {
			continue
		}
		fns := byPkg[p]
		sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })
		fmt.Printf("\n## %s\n", p)
		for _, f := range fns {
			kind := "atomic · " + f.Engine
			if f.Composite() {
				kind = "composite"
			}
			// The doc carries its own `# Title` heading (and `### description`);
			// print it as the section header, falling back to the fn name.
			if f.Doc != "" {
				fmt.Printf("\n%s\n", f.Doc)
			} else {
				fmt.Printf("\n### `%s`\n", f.Name)
			}
			fmt.Printf("\n`%s` · *%s*\n", signature(f), kind)
			printPorts("Inputs", f.In)
			printPorts("Outputs", f.Out)
			if f.Examples != "" {
				fmt.Printf("\n**Examples**\n\n%s\n", f.Examples)
			}
		}
	}
	return nil
}

// printPorts renders a function's input/output ports as a markdown list, showing
// each port's type and (when present) its description.
func printPorts(label string, ports []funk.Port) {
	if len(ports) == 0 {
		return
	}
	fmt.Printf("\n**%s**\n\n", label)
	for _, p := range ports {
		t := p.Type
		if t == "" {
			t = "Any"
		}
		if p.Doc != "" {
			fmt.Printf("- `%s` `%s` — %s\n", p.Name, t, oneLine(p.Doc))
		} else {
			fmt.Printf("- `%s` `%s`\n", p.Name, t)
		}
	}
}

// oneLine flattens a (possibly multi-line) port description for a markdown bullet.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func signature(f *funk.Fn) string {
	ports := func(ps []funk.Port) string {
		var parts []string
		for _, p := range ps {
			t := p.Type
			if t == "" {
				t = "Any"
			}
			parts = append(parts, p.Name+" "+t)
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprintf("%s(%s) → %s", f.Name, ports(f.In), ports(f.Out))
}

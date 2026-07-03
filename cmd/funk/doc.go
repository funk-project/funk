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
		fmt.Printf("\n## %s\n\n", p)
		for _, f := range fns {
			kind := "atomic · " + f.Engine
			if f.Composite() {
				kind = "composite"
			}
			fmt.Printf("- **`%s`** — %s  \n  `%s` · *%s*\n", f.Name, f.Doc, signature(f), kind)
		}
	}
	return nil
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

// Command funk is the funk CLI: parse, check, and run .funk programs.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/funk-project/funk/internal/funk"
)

const version = "0.0.1-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "version", "-v", "--version":
		fmt.Println("funk", version)
	case "parse":
		err = cmdParse(args)
	case "run":
		err = cmdRun(args)
	case "list", "ls":
		err = cmdList(args)
	case "types":
		err = cmdTypes(args)
	case "check":
		err = cmdCheck(args)
	case "introspect", "inspect":
		err = cmdIntrospect(args)
	case "make":
		err = cmdMake(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "funk: unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "funk:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `funk — a language and protocol for defining workflows

usage:
  funk version               print the version
  funk parse <file>          parse a .funk file, print the AST (JSON)
  funk list [-f path]        list loaded functions
  funk types [-f path]       list loaded types
  funk check [-f path]       static-check every composite function
  funk introspect [-f path] <fn>    print a function's structure (JSON)
  funk run [-f path] <fn> [k=v …]   run a function with named inputs
  funk make "<task>" [name]  funk writes a new funk function (architect→
                             programmer→check→reflect), adds it to std/generated

env:
  FUNK_STD   path to the std library (default: ./std)
`)
}

// loadLibrary loads std/ plus any extra files/dirs passed with -f.
func loadLibrary(extra []string) (*funk.Library, error) {
	lib := funk.NewLibrary()
	std := os.Getenv("FUNK_STD")
	if std == "" {
		std = "std"
	}
	if fi, err := os.Stat(std); err == nil && fi.IsDir() {
		if err := lib.LoadDir(std); err != nil {
			return nil, err
		}
	}
	for _, p := range extra {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if fi.IsDir() {
			err = lib.LoadDir(p)
		} else {
			err = lib.LoadFile(p)
		}
		if err != nil {
			return nil, err
		}
	}
	return lib, nil
}

// takeFlag extracts a repeated `-f path` flag, returning the rest.
func takeFlag(args []string, name string) (vals []string, rest []string) {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			vals = append(vals, args[i+1])
			i++
			continue
		}
		rest = append(rest, args[i])
	}
	return
}

func cmdParse(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("parse: expected exactly one file")
	}
	src, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	prog, err := funk.Parse(string(src))
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(prog, "", "  ")
	fmt.Println(string(out))
	return nil
}

func cmdList(args []string) error {
	files, rest := takeFlag(args, "-f")
	_ = rest
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	for _, f := range lib.Fns {
		kind := "atomic:" + f.Engine
		if f.Composite() {
			kind = "composite"
		}
		fmt.Printf("%-28s %-12s %s\n", f.Address(), kind, f.Doc)
	}
	return nil
}

func cmdTypes(args []string) error {
	files, _ := takeFlag(args, "-f")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, td := range lib.Types {
		if seen[td.Address()] {
			continue
		}
		seen[td.Address()] = true
		fmt.Printf("%s\n", td.Address())
		for _, f := range td.Fields {
			fmt.Printf("    %-10s %s\n", f.Name, f.Type)
		}
	}
	return nil
}

func cmdIntrospect(args []string) error {
	files, rest := takeFlag(args, "-f")
	if len(rest) != 1 {
		return fmt.Errorf("introspect: usage: funk introspect [-f path] <fn>")
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	in, ok := funk.Introspect(lib, rest[0])
	if !ok {
		return fmt.Errorf("unknown function %q", rest[0])
	}
	out, _ := json.MarshalIndent(in, "", "  ")
	fmt.Println(string(out))
	return nil
}

func cmdCheck(args []string) error {
	files, _ := takeFlag(args, "-f")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	issues := funk.Check(lib)
	if len(issues) == 0 {
		fmt.Printf("ok — %d functions, %d types, no issues\n", len(lib.Fns), len(lib.Types)/2)
		return nil
	}
	for _, i := range issues {
		fmt.Fprintln(os.Stderr, i.String())
	}
	return fmt.Errorf("%d issue(s)", len(issues))
}

func cmdRun(args []string) error {
	files, rest := takeFlag(args, "-f")
	if len(rest) < 1 {
		return fmt.Errorf("run: usage: funk run [-f path] <fn> [k=v …]")
	}
	ref := rest[0]
	inputs := map[string]interface{}{}
	for _, kv := range rest[1:] {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			return fmt.Errorf("run: bad input %q (want k=v)", kv)
		}
		k, raw := kv[:i], kv[i+1:]
		var v interface{}
		if json.Unmarshal([]byte(raw), &v) == nil {
			inputs[k] = v
		} else {
			inputs[k] = raw
		}
	}

	// A bare .funk file may be passed as the ref target's source.
	if strings.HasSuffix(ref, ".funk") {
		files = append(files, ref)
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}

	// If ref was a file, run its single/last fn unless a name follows.
	target := ref
	if strings.HasSuffix(ref, ".funk") {
		if len(lib.Fns) == 0 {
			return fmt.Errorf("run: %s has no functions", ref)
		}
		target = lib.Fns[len(lib.Fns)-1].Name
	}

	res := funk.RunStreaming(lib, target, inputs, funk.ExecOpts{}, printValue)
	if !res.OK {
		return fmt.Errorf("%s", res.Error)
	}
	return nil
}

func printValue(v interface{}) {
	if s, ok := v.(string); ok {
		fmt.Println(s)
		return
	}
	out, _ := json.Marshal(v)
	fmt.Println(string(out))
}

var _ = filepath.Base

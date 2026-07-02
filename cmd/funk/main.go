// Command funk is the funk CLI: parse, check, and run .funk programs.
package main

import (
	"encoding/json"
	"fmt"
	"os"

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
  funk version           print the version
  funk parse <file>      parse a .funk file and print the AST (JSON)
`)
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
	out, err := json.MarshalIndent(prog, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

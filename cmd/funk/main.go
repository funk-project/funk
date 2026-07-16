// Command funk is the funk CLI: parse, check, and run .funk programs.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/funk-project/funk/internal/funk"
)

const version = "0.0.1-dev"

// primer teaches any AI to read and write funk — paste it into any chat.
//
//go:embed primer.md
var primer string

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
	case "prompt", "primer":
		fmt.Print(primer)
	case "parse":
		err = cmdParse(args)
	case "fmt", "format":
		err = cmdFmt(args)
	case "run":
		err = cmdRun(args)
	case "list", "ls":
		err = cmdList(args)
	case "search", "find":
		err = cmdSearch(args)
	case "index":
		err = cmdIndex(args)
	case "context":
		err = cmdContext(args)
	case "types":
		err = cmdTypes(args)
	case "check":
		err = cmdCheck(args)
	case "test":
		err = cmdTest(args)
	case "doc", "docs":
		err = cmdDoc(args)
	case "introspect", "inspect":
		err = cmdIntrospect(args)
	case "graph":
		err = cmdGraph(args)
	case "make":
		err = cmdMake(args)
	case "serve":
		err = cmdServe(args)
	case "get":
		err = cmdGet(args)
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
  funk prompt                print the primer — paste into any AI to teach it funk
  funk parse <file>          parse a .funk file, print the AST (JSON)
  funk fmt [-w] <file>       format a .funk file canonically (-w writes)
  funk list [-f path]        list loaded functions
  funk search [--json] [--semantic] <query | in… -> out>   find a function to reuse
                             — by text (name/doc/examples), signature (Num Num -> Bool),
                             or --semantic (embedding-ranked; needs funk index first)
  funk index [-f path]       build the semantic search index (embeds every function
                             via FUNK_EMBED_URL — e.g. a local Ollama, free)
  funk context <task>        print a paste-ready brief for an AI: the primer + the
                             existing functions worth reusing for the task
  funk types [-f path]       list loaded types
  funk check [-f path]       static-check every composite function
  funk test [-f path]        run inline 'test (is (call) expected)' assertions
  funk doc [pkg]             generate markdown reference for the stdlib
  funk introspect [-f path] <fn>    print a function's structure (JSON)
  funk graph [-f path] <fn>         draw the graph the function derives from its code
  funk graph diff -f <fileA> (-f2 <fileB> | --git <rev>) [--json] [fn]
                             semantic graph diff — a change as a picture-level delta
                             (added/removed nodes, re-wired edges, changed labels);
                             exit 1 when the graphs differ
  funk run [-f path] [--server url] [--sandbox docker] [--bind k.a=v] [--trace] <fn> [k=v …]
                             run a function (streams live; --trace prints the RunReport)
  funk make "<task>" [name]  funk writes a new funk function (architect→
                             programmer→check→reflect), adds it to std/generated
  funk serve [--addr :7777]  run funkd (HTTP: /run streams NDJSON, /functions,
                             /introspect)
  funk get [<url>[@ref] [name]]  fetch a package (git repo) into ~/.funk/pkg.
                             @ref pins a tag/branch/commit; a version (v1.2.0) or
                             constraint (v1, latest) resolves to the best semver tag
                             and records a content-hash in funk.lock (verified on load).
                             With NO args, installs every versioned use "url" "vX" as a
                             dependency declared in the current dir's .funk files

env:
  FUNK_STD      path to the std library (default: ./std)
  FUNK_SERVER   run against a funkd server instead of locally
  FUNK_CACHE    package cache dir (default: ~/.funk/pkg)
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
	// fetched packages in the shared cache (~/.funk/pkg) resolve like std.
	if cache := cacheDir(); cache != "" {
		if fi, err := os.Stat(cache); err == nil && fi.IsDir() {
			if err := lib.LoadDir(cache); err != nil {
				return nil, err
			}
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
	if err := verifyLock(lockPath(), cacheDir()); err != nil {
		return nil, err
	}
	if err := lib.Finalize(); err != nil {
		return nil, err
	}
	return lib, nil
}

// takeBool extracts a boolean flag (present/absent), returning the rest.
func takeBool(args []string, name string) (found bool, rest []string) {
	for _, a := range args {
		if a == name {
			found = true
			continue
		}
		rest = append(rest, a)
	}
	return
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

func cmdFmt(args []string) error {
	write, rest := false, []string{}
	for _, a := range args {
		if a == "-w" || a == "--write" {
			write = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("fmt: usage: funk fmt [-w] <file>")
	}
	src, err := os.ReadFile(rest[0])
	if err != nil {
		return err
	}
	prog, err := funk.Parse(string(src))
	if err != nil {
		return err
	}
	out := funk.Format(prog)
	if write {
		return os.WriteFile(rest[0], []byte(out), 0o644)
	}
	fmt.Print(out)
	return nil
}

// docSummary pulls a one-line description from a fn's markdown doc: the text of
// the `### …` line (the convention), else the first non-heading line.
func docSummary(doc string) string {
	for _, l := range strings.Split(doc, "\n") {
		s := strings.TrimSpace(l)
		if strings.HasPrefix(s, "### ") {
			return strings.TrimSpace(s[4:])
		}
	}
	for _, l := range strings.Split(doc, "\n") {
		s := strings.TrimSpace(l)
		if s != "" && !strings.HasPrefix(s, "#") {
			return s
		}
	}
	first, _, _ := strings.Cut(doc, "\n")
	return first
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
		label := docSummary(f.Doc)
		if f.Display != "" && f.Display != f.Name {
			label = "“" + f.Display + "” — " + docSummary(f.Doc)
		}
		fmt.Printf("%-28s %-12s %s\n", f.Address(), kind, label)
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

func cmdTest(args []string) error {
	files, _ := takeFlag(args, "-f")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	results := funk.RunTests(lib)
	pass, fail := 0, 0
	for _, r := range results {
		if r.Ok {
			pass++
			continue
		}
		fail++
		if r.Err != "" {
			fmt.Fprintf(os.Stderr, "FAIL %s: error: %s\n", r.Fn, r.Err)
		} else {
			fmt.Fprintf(os.Stderr, "FAIL %s: got %v, want %v\n", r.Fn, r.Got, r.Want)
		}
	}
	fmt.Printf("%d passed, %d failed (%d assertions)\n", pass, fail, len(results))
	if fail > 0 {
		return fmt.Errorf("%d test(s) failed", fail)
	}
	return nil
}

func cmdCheck(args []string) error {
	files, _ := takeFlag(args, "-f")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	issues := funk.Check(lib)
	errs := 0
	for _, i := range issues {
		if i.Warn {
			fmt.Fprintln(os.Stderr, "warning: "+i.String())
			continue
		}
		fmt.Fprintln(os.Stderr, i.String())
		errs++
	}
	if errs == 0 {
		warns := len(issues)
		if warns == 0 {
			fmt.Printf("ok — %d functions, %d types, no issues\n", len(lib.Fns), len(lib.Types)/2)
		} else {
			fmt.Printf("ok — %d functions, %d types, no errors (%d warning(s))\n", len(lib.Fns), len(lib.Types)/2, warns)
		}
		return nil
	}
	return fmt.Errorf("%d issue(s)", errs)
}

func cmdRun(args []string) error {
	files, rest := takeFlag(args, "-f")
	servers, rest := takeFlag(rest, "--server")
	sandboxes, rest := takeFlag(rest, "--sandbox")
	binds, rest := takeFlag(rest, "--bind")
	trace, rest := takeBool(rest, "--trace")
	progress, rest := takeBool(rest, "--progress")
	progressFiles, rest := takeFlag(rest, "--progress-file")
	server := os.Getenv("FUNK_SERVER")
	if len(servers) > 0 {
		server = servers[len(servers)-1]
	}
	opts := funk.ExecOpts{}
	// FUNK_TIMEOUT (seconds) raises the per-body engine timeout — needed for slow,
	// large claude bodies (e.g. a forge decompose of a big project) that exceed the
	// 120s default. Applies to every engine in this run.
	if v := os.Getenv("FUNK_TIMEOUT"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			opts.Timeout = time.Duration(secs) * time.Second
		}
	}
	if len(sandboxes) > 0 {
		opts.Sandbox = sandboxes[len(sandboxes)-1]
	}
	if len(binds) > 0 {
		opts.Bindings = map[string]string{}
		for _, b := range binds {
			i := strings.IndexByte(b, '=')
			if i <= 0 {
				return fmt.Errorf("run: bad --bind %q (want kind.alias=value)", b)
			}
			key, raw := b[:i], b[i+1:]
			val, err := expandBinding(raw)
			if err != nil {
				return fmt.Errorf("run: --bind %s: %w", key, err)
			}
			opts.Bindings[key] = val
		}
	}
	if len(rest) < 1 {
		return fmt.Errorf("run: usage: funk run [-f path] [--server url] <fn> [k=v …]")
	}
	ref := rest[0]
	isFile := strings.HasSuffix(ref, ".funk")
	rest = rest[1:]
	// For a file, an optional entry-point name may follow: `funk run x.funk deploy k=v`.
	entryName := ""
	if isFile && len(rest) > 0 && !strings.Contains(rest[0], "=") {
		entryName = rest[0]
		rest = rest[1:]
	}
	inputs := map[string]interface{}{}
	for _, kv := range rest {
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

	// Thin client: if a server is configured, run there and stream back.
	if server != "" {
		return runViaServer(server, ref, inputs)
	}

	// A bare .funk file may be passed as the ref target's source.
	if isFile {
		files = append(files, ref)
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}

	// A file is run through its `main` entry point(s); a file with no main can be
	// imported (use "…") but not run directly.
	target := ref
	if isFile {
		t, err := fileEntry(lib, ref, entryName)
		if err != nil {
			return err
		}
		target = t
	}

	if trace {
		res, rep := funk.RunWithReport(lib, target, inputs, opts)
		if !res.OK {
			return fmt.Errorf("%s", res.Error)
		}
		printValue(res.Value)
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Fprintln(os.Stderr, string(out))
		return nil
	}
	// --progress surfaces funk's live self-observation to stderr: each node prints
	// as it ENTERS (→, before it runs — the glow) and when it returns (✓). Turns a
	// long run (e.g. a slow AI pipeline) from a silent "working…" into a live trace.
	if progress || len(progressFiles) > 0 {
		var pf *os.File
		if len(progressFiles) > 0 {
			// truncate at start so each run is fresh; the IDE tails this file.
			pfPath := progressFiles[len(progressFiles)-1]
			os.MkdirAll(filepath.Dir(pfPath), 0o755) // e.g. <project>/.agent-runner
			pf, _ = os.Create(pfPath)
			if pf != nil {
				defer pf.Close()
			}
		}
		onEvent := func(ev funk.TraceEvent) {
			// skip the noise: unnamed nodes and qualified std calls (maths.add, …).
			if ev.Fn == "" || strings.Contains(ev.Fn, ".") {
				return
			}
			var line string
			switch ev.Kind {
			case "enter":
				line = "→ " + ev.Fn
			case "call":
				line = "✓ " + ev.Fn
			default:
				return
			}
			if progress {
				fmt.Fprintln(os.Stderr, line)
			}
			if pf != nil {
				fmt.Fprintln(pf, line)
				pf.Sync() // flush so the IDE sees it immediately
			}
		}
		res, _ := funk.RunLive(lib, target, inputs, opts, onEvent, printValue)
		if pf != nil {
			fmt.Fprintln(pf, "● done")
			pf.Sync()
		}
		if !res.OK {
			return fmt.Errorf("%s", res.Error)
		}
		return nil
	}
	res := funk.RunStreaming(lib, target, inputs, opts, printValue)
	if !res.OK {
		return fmt.Errorf("%s", res.Error)
	}
	return nil
}

// fileEntry picks the runnable entry point for `funk run <path> [name]`: a fn in
// that file marked `main`. With a name, that fn must exist and be a main; with no
// name, the sole main is chosen. Zero or several mains (with no name) are a clear
// error — a file without a main can be imported but not run.
func fileEntry(lib *funk.Library, path, name string) (string, error) {
	var mains []string
	byName := map[string]*funk.Fn{}
	for _, f := range lib.Fns {
		if f.File != path {
			continue
		}
		byName[f.Name] = f
		if f.Main {
			mains = append(mains, f.Name)
		}
	}
	if name != "" {
		f, ok := byName[name]
		if !ok {
			return "", fmt.Errorf("run: %s has no function %q", path, name)
		}
		if !f.Main {
			return "", fmt.Errorf("run: %q is not a runnable entry point — add `main` to run it", name)
		}
		return name, nil
	}
	switch len(mains) {
	case 1:
		return mains[0], nil
	case 0:
		return "", fmt.Errorf("run: %s has no `main` — it can be imported (use \"…\") but not run directly", path)
	default:
		return "", fmt.Errorf("run: %s has multiple entry points: %s — pick one: funk run %s <name>", path, strings.Join(mains, ", "), path)
	}
}

// expandBinding resolves a --bind value from a source, keeping secrets off the
// command line (where `ps`/shell history would capture them): `@path` reads a
// file, `@-` reads stdin, `env:VAR` reads an env var; anything else is literal.
func expandBinding(raw string) (string, error) {
	switch {
	case raw == "@-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	case strings.HasPrefix(raw, "@"):
		b, err := os.ReadFile(raw[1:])
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	case strings.HasPrefix(raw, "env:"):
		name := raw[len("env:"):]
		v, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("env var %q is not set", name)
		}
		return v, nil
	default:
		return raw, nil
	}
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

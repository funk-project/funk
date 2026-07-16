package main

// `funk graph diff` — review a change to a .funk file as a PICTURE-LEVEL delta:
// which nodes appeared, which edges were re-wired, which guards/labels changed.
// Built for reviewing AI-written edits, where the raw text diff hides whether
// the derived graph (the actual plan) changed at all.
//
//	funk graph diff -f old.funk -f2 new.funk [fn]     two files
//	funk graph diff -f file.funk --git <rev> [fn]     the rev's version vs. the file
//
// With no [fn], every fn present in either side is diffed (identical fns are
// skipped). Exit 0 when identical, 1 when different. `--json` for machines.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/funk-project/funk/internal/funk"
)

// fnGraphDiff is one function's report in the --json output.
type fnGraphDiff struct {
	Fn     string          `json:"fn"`
	Status string          `json:"status"` // added | removed | changed | identical
	Diff   *funk.GraphDiff `json:"diff,omitempty"`
}

func cmdGraphDiff(args []string) error {
	filesA, rest := takeFlag(args, "-f")
	filesB, rest := takeFlag(rest, "-f2")
	gits, rest := takeFlag(rest, "--git")
	asJSON, rest := takeBool(rest, "--json")
	if len(filesA) != 1 || (len(filesB) == 0) == (len(gits) == 0) || len(rest) > 1 {
		return fmt.Errorf("graph diff: usage: funk graph diff -f <fileA> (-f2 <fileB> | --git <rev>) [--json] [fn]")
	}
	fnFilter := ""
	if len(rest) == 1 {
		fnFilter = rest[0]
	}

	fileA := filesA[0]
	newSrcBytes, err := os.ReadFile(fileA)
	if err != nil {
		return err
	}
	var oldSrc, newSrc string
	if len(gits) > 0 {
		// --git <rev>: the rev's version of fileA is the OLD side, the working
		// file the NEW side (a review reads "what did this change do?").
		oldSrc, err = gitFileAt(fileA, gits[len(gits)-1])
		if err != nil {
			return err
		}
		newSrc = string(newSrcBytes)
	} else {
		// -f2: fileA is the OLD side, fileB the NEW side.
		oldSrc = string(newSrcBytes)
		nb, err := os.ReadFile(filesB[0])
		if err != nil {
			return err
		}
		newSrc = string(nb)
	}

	oldLib, err := libFromSource(oldSrc)
	if err != nil {
		return fmt.Errorf("old side: %w", err)
	}
	newLib, err := libFromSource(newSrc)
	if err != nil {
		return fmt.Errorf("new side: %w", err)
	}

	// The fn set: old side's fns in order, then new-only fns.
	var names []string
	seen := map[string]bool{}
	for _, f := range oldLib.Fns {
		if !seen[f.Name] {
			seen[f.Name] = true
			names = append(names, f.Name)
		}
	}
	for _, f := range newLib.Fns {
		if !seen[f.Name] {
			seen[f.Name] = true
			names = append(names, f.Name)
		}
	}
	if fnFilter != "" {
		if !seen[fnFilter] {
			return fmt.Errorf("graph diff: function %q not found in either side", fnFilter)
		}
		names = []string{fnFilter}
	}

	var reports []fnGraphDiff
	differs := false
	for _, name := range names {
		fa, hasA := oldLib.Lookup(name)
		fb, hasB := newLib.Lookup(name)
		switch {
		case hasA && !hasB:
			reports = append(reports, fnGraphDiff{Fn: name, Status: "removed"})
			differs = true
		case !hasA && hasB:
			reports = append(reports, fnGraphDiff{Fn: name, Status: "added"})
			differs = true
		default:
			ga, err := funk.DeriveGraph(oldLib, fa)
			if err != nil {
				return err
			}
			gb, err := funk.DeriveGraph(newLib, fb)
			if err != nil {
				return err
			}
			d := funk.DiffGraphs(ga, gb)
			if d.Identical() {
				reports = append(reports, fnGraphDiff{Fn: name, Status: "identical"})
				continue
			}
			differs = true
			reports = append(reports, fnGraphDiff{Fn: name, Status: "changed", Diff: d})
		}
	}

	if asJSON {
		out, _ := json.MarshalIndent(reports, "", "  ")
		fmt.Println(string(out))
	} else {
		printGraphDiff(reports)
	}
	if differs {
		return fmt.Errorf("graphs differ")
	}
	return nil
}

// printGraphDiff renders the human-readable per-fn report.
func printGraphDiff(reports []fnGraphDiff) {
	identical := 0
	for _, r := range reports {
		if r.Status == "identical" {
			identical++
			continue
		}
		switch r.Status {
		case "added":
			fmt.Printf("fn %s — ADDED\n", r.Fn)
			continue
		case "removed":
			fmt.Printf("fn %s — REMOVED\n", r.Fn)
			continue
		}
		d := r.Diff
		fmt.Printf("fn %s — changed (%d nodes / %d edges unchanged)\n", r.Fn, d.UnchangedNodes, d.UnchangedEdges)
		inCtx := func(ctx string) string {
			if ctx == "" {
				return ""
			}
			return " (" + ctx + ")"
		}
		for _, n := range d.AddedNodes {
			fmt.Printf("  + %s%s\n", nodeName(n), inCtx(n.Context))
		}
		for _, n := range d.RemovedNodes {
			fmt.Printf("  - %s%s\n", nodeName(n), inCtx(n.Context))
		}
		for _, n := range d.ChangedNodes {
			fmt.Printf("  ~ %s%s: label %q → %q\n", n.Kind+" "+n.Fn, inCtx(n.Context), n.OldLabel, n.Label)
		}
		for _, e := range d.RewiredEdges {
			if e.What == "source" {
				fmt.Printf("  ~ %s now fed by %s, was %s\n", e.Target, e.Source, e.Was)
			} else {
				fmt.Printf("  ~ %s now feeds %s, was %s\n", e.Source, e.Target, e.Was)
			}
		}
		for _, e := range d.AddedEdges {
			fmt.Printf("  + edge %s → %s%s\n", e.Source, e.Target, edgeNote(e))
		}
		for _, e := range d.RemovedEdges {
			fmt.Printf("  - edge %s → %s%s\n", e.Source, e.Target, edgeNote(e))
		}
	}
	if identical > 0 {
		fmt.Printf("%d fn(s) identical\n", identical)
	}
}

// nodeName renders a delta node as "kind fn" (label when there is no fn).
func nodeName(n funk.NodeDelta) string {
	name := n.Fn
	if name == "" {
		name = n.Label
	}
	if name == "" {
		return n.Kind
	}
	return n.Kind + " " + name
}

// edgeNote annotates an edge line with its non-default plumbing.
func edgeNote(e funk.EdgeDelta) string {
	var bits []string
	if e.Kind == "control" {
		bits = append(bits, "control")
	}
	if e.Type != "" {
		bits = append(bits, e.Type)
	}
	if e.SourceHandle != "" {
		bits = append(bits, "from "+e.SourceHandle)
	}
	if e.TargetHandle != "" {
		bits = append(bits, "into "+e.TargetHandle)
	}
	if len(bits) == 0 {
		return ""
	}
	return " [" + strings.Join(bits, ", ") + "]"
}

// libFromSource parses one side's .funk source into its OWN library — the diff
// compares only what the file itself defines. Finalize is deliberately skipped:
// an `alias` fn draws as its delegation node either way (DeriveGraph reads the
// Alias field), and finalizing would fail on alias targets living outside the
// file.
func libFromSource(src string) (*funk.Library, error) {
	lib := funk.NewLibrary()
	if err := lib.LoadString(src); err != nil {
		return nil, err
	}
	return lib, nil
}

// gitFileAt reads a file's content at a git rev, running git relative to the
// file's own repository (`git -C <root> show rev:relpath`).
func gitFileAt(path, rev string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(abs)
	rootOut, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("graph diff: %s is not in a git repository", path)
	}
	root := strings.TrimSpace(string(rootOut))
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	out, err := exec.Command("git", "-C", root, "show", rev+":"+rel).Output()
	if err != nil {
		return "", fmt.Errorf("graph diff: git show %s:%s failed (%w)", rev, rel, err)
	}
	return string(out), nil
}

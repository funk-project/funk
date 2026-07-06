package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/funk-project/funk/internal/funk"
)

// cmdMake is the self-programming loop: funk writes funk.
//
//	architect → programmer  (generate the code, in funk)
//	→ write → check         (add it to the library, statically check)
//	→ reflect → rewrite      (self-analyze and fix, up to N attempts)
//
// The skills are funk functions on the `claude` engine (std/skills); the fs +
// loop are driven here. This realises the thesis: funk programs its own source.
func cmdMake(args []string) error {
	files, rest := takeFlag(args, "-f")
	if len(rest) < 1 {
		return fmt.Errorf("make: usage: funk make \"<task>\" [name]")
	}
	task := rest[0]
	forcedName := ""
	if len(rest) > 1 {
		forcedName = rest[1]
	}
	llm := funk.ExecOpts{Timeout: 180 * time.Second}

	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}

	// Reuse before you write. Rank the library for this task (semantic when an
	// index + embeddings endpoint are configured, else text/signature).
	cands, semantic := reuseCandidates(lib, task, 8)

	// Whole-reuse: if the top match already covers the task, don't generate a
	// near-duplicate — return the existing function. Gated on a strong semantic
	// score and confirmed by the `covers` judge, so it never reuses the wrong one.
	if semantic && len(cands) > 0 && cands[0].Score >= reuseExactThreshold {
		if top := cands[0]; judgeCovers(lib, task, top, llm) {
			fmt.Fprintf(os.Stderr, "· reuse: %s already covers this (%.2f) — not generating\n", top.Address, top.Score)
			fmt.Printf("; %s already implements this task — reuse it directly\n; %s\n; %s\n", top.Address, top.Signature, top.Doc)
			return nil
		}
	}

	// Otherwise prime the generator so it composes these instead of reinventing.
	gen := task
	if len(cands) > 0 {
		var b strings.Builder
		b.WriteString(task)
		b.WriteString("\n\nReusable funk functions already in the library — COMPOSE these by full address (e.g. `(funk/std/maths/add x 1)`) instead of reinventing them:\n")
		for _, r := range cands {
			fmt.Fprintf(&b, "- %s : %s — %s\n", r.Address, r.Signature, r.Doc)
		}
		gen = b.String()
		fmt.Fprintf(os.Stderr, "· offering %d reusable functions to the generator\n", len(cands))
	}

	fmt.Fprintln(os.Stderr, "· architect → programmer: generating…")
	res := funk.Run(lib, "generate", map[string]interface{}{"task": gen}, llm)
	if !res.OK {
		return fmt.Errorf("generate: %s", res.Error)
	}
	code := cleanFunk(fmt.Sprint(res.Value))

	// Gate: a generated file is only kept if it passes check + tests. Otherwise it
	// must not linger in std/generated (it would break everyone's `funk check`).
	var writtenPath string
	success := false
	defer func() {
		if !success && writtenPath != "" {
			os.Remove(writtenPath)
		}
	}()

	const maxAttempts = 3
	var lastReport string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// The gate keys off the fn's OWN name (from the code); forcedName only names
		// the file. Keying the check/test/index off forcedName would let them pass
		// vacuously whenever the generator names the fn differently.
		actual := fnName(code)
		if actual == "" {
			actual = "generated"
		}
		file := forcedName
		if file == "" {
			file = actual
		}

		// 1) does it parse?
		if _, perr := funk.Parse(code); perr != nil {
			lastReport = "parse error: " + perr.Error()
			fmt.Fprintf(os.Stderr, "· attempt %d: %s → reflect\n", attempt, lastReport)
			if code = reflectFix(lib, code, lastReport, llm); code == "" {
				return fmt.Errorf("reflect failed")
			}
			continue
		}

		// 2) write it into the library and static-check
		path := filepath.Join("std", "generated", file+".funk")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(code+"\n"), 0o644); err != nil {
			return err
		}
		writtenPath = path
		checkLib, err := loadLibrary(files)
		if err != nil {
			lastReport = err.Error()
			fmt.Fprintf(os.Stderr, "· attempt %d: load error → reflect\n", attempt)
			code = reflectFix(lib, code, lastReport, llm)
			continue
		}
		issues := issuesFor(funk.Check(checkLib), actual)
		if fails := testFailures(checkLib, actual); len(issues) == 0 && len(fails) > 0 {
			issues = fails
		}
		if len(issues) == 0 {
			success = true
			fmt.Fprintf(os.Stderr, "· attempt %d: check + tests OK ✓\n", attempt)
			indexUpsert(checkLib, actual) // incremental reindex; no-op without an index
			fmt.Printf("%s\n\n; written to %s — reviewer/tester below\n", code, path)
			opinion(checkLib, code, llm)
			return nil
		}
		lastReport = strings.Join(issues, "; ")
		fmt.Fprintf(os.Stderr, "· attempt %d: %s → reflect\n", attempt, lastReport)
		code = reflectFix(lib, code, lastReport, llm)
		if code == "" {
			return fmt.Errorf("reflect failed")
		}
	}
	return fmt.Errorf("gave up after %d attempts; last issue: %s", maxAttempts, lastReport)
}

// reuseExactThreshold is the cosine above which the top match is a candidate for
// whole-reuse — high enough that only near-identical intents reach the judge.
const reuseExactThreshold = 0.80

// judgeCovers asks the `covers` skill whether an existing function already fully
// satisfies the task, so make can reuse it instead of generating a duplicate.
func judgeCovers(lib *funk.Library, task string, c scoredRow, llm funk.ExecOpts) bool {
	cand := fmt.Sprintf("%s : %s — %s", c.Address, c.Signature, c.Doc)
	r := funk.Run(lib, "covers", map[string]interface{}{"task": task, "candidate": cand}, llm)
	if !r.OK {
		return false
	}
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(fmt.Sprint(r.Value))), "YES")
}

func reflectFix(lib *funk.Library, code, report string, llm funk.ExecOpts) string {
	res := funk.Run(lib, "reflect", map[string]interface{}{"code": code, "report": report}, llm)
	if !res.OK {
		return ""
	}
	return cleanFunk(fmt.Sprint(res.Value))
}

func opinion(lib *funk.Library, code string, llm funk.ExecOpts) {
	if r := funk.Run(lib, "reviewer", map[string]interface{}{"code": code}, llm); r.OK {
		fmt.Fprintf(os.Stderr, "\n[reviewer]\n%s\n", r.Value)
	}
	if r := funk.Run(lib, "tester", map[string]interface{}{"code": code}, llm); r.OK {
		fmt.Fprintf(os.Stderr, "\n[tester]\n%s\n", r.Value)
	}
}

var fenceRe = regexp.MustCompile("(?s)```[a-zA-Z]*\\n?")
var fnNameRe = regexp.MustCompile(`(?m)^\s*fn\s+(\S+)\s*\{`)

// cleanFunk strips markdown fences and surrounding prose, keeping the fn block.
func cleanFunk(s string) string {
	s = fenceRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "```", "")
	// trim to the first `fn ` … matching closing brace region
	if i := strings.Index(s, "fn "); i >= 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	return strings.TrimSpace(s)
}

func fnName(code string) string {
	if m := fnNameRe.FindStringSubmatch(code); m != nil {
		return strings.TrimSuffix(m[1], "?")
	}
	return ""
}

func issuesFor(all []funk.Issue, name string) []string {
	var out []string
	for _, i := range all {
		if i.Warn {
			continue // advisories don't block generation
		}
		if i.Fn == name || strings.Contains(i.Msg, name) {
			out = append(out, i.String())
		}
	}
	return out
}

// testFailures runs the library's inline tests and returns the failures for the
// named function — the second half of the make gate (check + test).
func testFailures(lib *funk.Library, name string) []string {
	var out []string
	for _, r := range funk.RunTests(lib) {
		if r.Fn != name || r.Ok {
			continue
		}
		if r.Err != "" {
			out = append(out, fmt.Sprintf("test error: %s", r.Err))
		} else {
			out = append(out, fmt.Sprintf("test failed: got %v, want %v", r.Got, r.Want))
		}
	}
	return out
}

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

	fmt.Fprintln(os.Stderr, "· architect → programmer: generating…")
	res := funk.Run(lib, "generate", map[string]interface{}{"task": task}, llm)
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
		name := forcedName
		if name == "" {
			name = fnName(code)
		}
		if name == "" {
			name = "generated"
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
		path := filepath.Join("std", "generated", name+".funk")
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
		issues := issuesFor(funk.Check(checkLib), name)
		if fails := testFailures(checkLib, name); len(issues) == 0 && len(fails) > 0 {
			issues = fails
		}
		if len(issues) == 0 {
			success = true
			fmt.Fprintf(os.Stderr, "· attempt %d: check + tests OK ✓\n", attempt)
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

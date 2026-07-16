package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/funk-project/funk/internal/funk"
)

// registryDir resolves the funktion registry — THE COMMONS: a git-backed,
// append-only collection of VERIFIED funktions (publish refuses anything that
// is not check-green AND test-green). Resolution: the --registry flag, then
// FUNK_REGISTRY, then ~/.funk/registry.
func registryDir(override string) string {
	if override != "" {
		return override
	}
	if d := os.Getenv("FUNK_REGISTRY"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".funk", "registry")
}

// publishManifest is the stamp written next to a published package's sources.
type publishManifest struct {
	Package     string   `json:"package"`
	PublishedAt string   `json:"publishedAt"`
	Checksum    string   `json:"checksum"` // sha256 (16 hex chars) over the .funk files
	Fns         []string `json:"fns"`
}

// cmdPublish verifies a project (funk check + funk test must BOTH be green) and
// copies its .funk files into the registry under the package's path, stamped
// with a manifest and committed to the registry's git history.
func cmdPublish(args []string) error {
	files, rest := takeFlag(args, "-f")
	regs, _ := takeFlag(rest, "--registry")
	if len(files) == 0 {
		return fmt.Errorf("publish: usage: funk publish -f <dir> [--registry <path>]")
	}
	dir := files[len(files)-1]
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("publish: -f must name a project directory, got a file: %s", dir)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	override := ""
	if len(regs) > 0 {
		override = regs[len(regs)-1]
	}
	reg := registryDir(override)

	lib, err := loadLibrary([]string{dir})
	if err != nil {
		return err
	}
	under := func(file string) bool {
		abs, err := filepath.Abs(file)
		return err == nil && (abs == absDir || strings.HasPrefix(abs, absDir+string(filepath.Separator)))
	}

	// 1. VERIFY — only verified funktions accumulate in the commons.
	var checkErrs []string
	for _, i := range funk.Check(lib) {
		if !i.Warn && under(i.File) {
			checkErrs = append(checkErrs, i.String())
		}
	}
	if len(checkErrs) > 0 {
		return fmt.Errorf("publish refused — funk check found %d error(s):\n%s",
			len(checkErrs), strings.Join(checkErrs, "\n"))
	}
	names := map[string]bool{}
	pkgs := map[string]bool{}
	var fnList []string
	for _, f := range lib.Fns {
		if !under(f.File) {
			continue
		}
		names[f.Name] = true
		fnList = append(fnList, f.Name)
		if f.Package != "" {
			pkgs[f.Package] = true
		}
	}
	if len(fnList) == 0 {
		return fmt.Errorf("publish: no functions found in %s", dir)
	}
	var testFails []string
	for _, r := range funk.RunTests(lib) {
		if r.Ok || !names[r.Fn] {
			continue
		}
		if r.Err != "" {
			testFails = append(testFails, fmt.Sprintf("FAIL %s: error: %s", r.Fn, r.Err))
		} else {
			testFails = append(testFails, fmt.Sprintf("FAIL %s: got %v, want %v", r.Fn, r.Got, r.Want))
		}
	}
	if len(testFails) > 0 {
		return fmt.Errorf("publish refused — funk test found %d failure(s):\n%s",
			len(testFails), strings.Join(testFails, "\n"))
	}
	switch len(pkgs) {
	case 0:
		return fmt.Errorf("publish: the project needs a package block (its name is the registry path)")
	case 1: // the package path
	default:
		var list []string
		for p := range pkgs {
			list = append(list, p)
		}
		sort.Strings(list)
		return fmt.Errorf("publish: the project declares multiple packages (%s) — publish one package at a time", strings.Join(list, ", "))
	}
	pkg := ""
	for p := range pkgs {
		pkg = p
	}
	sort.Strings(fnList)

	// 2. COPY the package's .funk files (relative layout preserved) + manifest.
	srcs, checksum, err := funkFiles(absDir)
	if err != nil {
		return err
	}
	dst := filepath.Join(reg, filepath.FromSlash(pkg))
	for _, rel := range srcs {
		b, err := os.ReadFile(filepath.Join(absDir, rel))
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
	}
	man := publishManifest{Package: pkg, PublishedAt: time.Now().UTC().Format(time.RFC3339), Checksum: checksum, Fns: fnList}
	mb, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(dst, "funk-manifest.json"), append(mb, '\n'), 0o644); err != nil {
		return err
	}

	// 3. RECORD — the registry is a git history of verified publications.
	if _, err := os.Stat(filepath.Join(reg, ".git")); err != nil {
		if out, err := exec.Command("git", "init", "-q", reg).CombinedOutput(); err != nil {
			return fmt.Errorf("publish: git init %s: %v: %s", reg, err, out)
		}
	}
	if out, err := exec.Command("git", "-C", reg, "add", "-A", ".").CombinedOutput(); err != nil {
		return fmt.Errorf("publish: git add: %v: %s", err, out)
	}
	msg := fmt.Sprintf("publish %s: %s", pkg, strings.Join(fnList, " "))
	commit := exec.Command("git", "-C", reg,
		"-c", "user.name=funk publish", "-c", "user.email=funk@localhost",
		"commit", "-q", "-m", msg)
	if out, err := commit.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			fmt.Printf("published %s (unchanged) → %s\n", pkg, dst)
			return nil
		}
		return fmt.Errorf("publish: git commit: %v: %s", err, out)
	}
	fmt.Printf("published %s (%d fns, checksum %s) → %s\n", pkg, len(fnList), checksum, dst)
	return nil
}

// funkFiles lists a project's .funk files (relative, sorted; .git and
// .agent-runner runtime artefacts excluded) and their combined content hash —
// sha256 over rel path + bytes, go.sum-style (matches hashPkg's shape).
func funkFiles(dir string) ([]string, string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".agent-runner" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".funk") {
			rel, _ := filepath.Rel(dir, p)
			rels = append(rels, rel)
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if len(rels) == 0 {
		return nil, "", fmt.Errorf("publish: no .funk files under %s", dir)
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		fmt.Fprintf(h, "%s\n", filepath.ToSlash(rel))
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return nil, "", err
		}
		h.Write(b)
	}
	return rels, hex.EncodeToString(h.Sum(nil))[:16], nil
}

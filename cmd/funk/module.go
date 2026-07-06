package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// semver is a `vMAJOR.MINOR.PATCH` version tag.
type semver struct{ major, minor, patch int }

func parseSemver(tag string) (semver, bool) {
	p := strings.SplitN(strings.TrimPrefix(tag, "v"), ".", 3)
	if len(p) != 3 {
		return semver{}, false
	}
	var v semver
	var err error
	if v.major, err = strconv.Atoi(p[0]); err != nil {
		return semver{}, false
	}
	if v.minor, err = strconv.Atoi(p[1]); err != nil {
		return semver{}, false
	}
	if v.patch, err = strconv.Atoi(p[2]); err != nil {
		return semver{}, false
	}
	return v, true
}

func (v semver) String() string { return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch) }

func (a semver) less(b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}

// isConstraint reports whether ref asks for version resolution (rather than an
// exact tag / branch / commit): "latest", or a partial `v1` / `v1.2`.
func isConstraint(ref string) bool {
	if ref == "latest" {
		return true
	}
	p := strings.Split(strings.TrimPrefix(ref, "v"), ".")
	if !strings.HasPrefix(ref, "v") || len(p) >= 3 {
		return false
	}
	for _, seg := range p {
		if _, err := strconv.Atoi(seg); err != nil {
			return false
		}
	}
	return true
}

// matchConstraint reports whether v satisfies constraint ("" / "latest" ⇒ any;
// "v1" ⇒ same major; "v1.2" ⇒ same major+minor).
func matchConstraint(constraint string, v semver) bool {
	if constraint == "" || constraint == "latest" {
		return true
	}
	p := strings.Split(strings.TrimPrefix(constraint, "v"), ".")
	if len(p) >= 1 {
		if n, err := strconv.Atoi(p[0]); err != nil || n != v.major {
			return false
		}
	}
	if len(p) >= 2 {
		if n, err := strconv.Atoi(p[1]); err != nil || n != v.minor {
			return false
		}
	}
	return true
}

// resolveVersion picks the highest git tag from url satisfying constraint.
func resolveVersion(url, constraint string) (string, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", url).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve %q: %s", constraint, strings.TrimSpace(string(out)))
	}
	best, found := semver{}, false
	for _, line := range strings.Split(string(out), "\n") {
		i := strings.Index(line, "refs/tags/")
		if i < 0 {
			continue
		}
		tag := strings.TrimSuffix(line[i+len("refs/tags/"):], "^{}")
		v, ok := parseSemver(tag)
		if !ok || !matchConstraint(constraint, v) {
			continue
		}
		if !found || best.less(v) {
			best, found = v, true
		}
	}
	if !found {
		return "", fmt.Errorf("no version tag matching %q at %s", constraint, url)
	}
	return best.String(), nil
}

// hashPkg is a stable content hash of a package directory: sha256 over each
// non-.git file's relative path + bytes, in sorted order. Location-independent,
// so a re-pointed tag or a tampered cache changes the hash (go.sum-style).
func hashPkg(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		fmt.Fprintf(h, "%s\n", filepath.ToSlash(rel))
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// lockPath is the content-hash lockfile (like go.sum): `funk.lock` in the working
// dir, overridable with FUNK_LOCK (for tests).
func lockPath() string {
	if p := os.Getenv("FUNK_LOCK"); p != "" {
		return p
	}
	return "funk.lock"
}

// readLock returns the `name@ref → hash` map from the lockfile ("" if absent).
func readLock(path string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			m[f[0]] = f[1]
		}
	}
	return m
}

// writeLockEntry records key (name@ref) → hash, replacing any prior entry.
func writeLockEntry(path, key, hash string) error {
	m := readLock(path)
	m[key] = hash
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s  %s\n", k, m[k])
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// verifyLock fails if any lockfile entry's package (in cache, keyed by the name
// before `@`) is present but hashes differently — a re-pointed tag or tampering.
func verifyLock(path, cache string) error {
	m := readLock(path)
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		name := key
		if i := strings.Index(key, "@"); i >= 0 {
			name = key[:i]
		}
		dir := filepath.Join(cache, name)
		if _, err := os.Stat(dir); err != nil {
			continue // not fetched here — nothing to verify
		}
		got, err := hashPkg(dir)
		if err != nil {
			return err
		}
		if got != m[key] {
			return fmt.Errorf("integrity: %s hash mismatch (want %s, got %s) — a re-pointed tag or tampered cache", key, m[key], got)
		}
	}
	return nil
}

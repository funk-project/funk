package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSemverParseMatchOrder(t *testing.T) {
	if v, ok := parseSemver("v1.2.3"); !ok || v.major != 1 || v.minor != 2 || v.patch != 3 {
		t.Fatalf("parseSemver(v1.2.3) = %+v %v", v, ok)
	}
	for _, bad := range []string{"v1.2", "v1", "vx.y.z", "1.2", "latest"} {
		if _, ok := parseSemver(bad); ok {
			t.Errorf("parseSemver(%q) should fail", bad)
		}
	}
	a, _ := parseSemver("v1.2.0")
	b, _ := parseSemver("v1.10.0")
	if !a.less(b) {
		t.Error("v1.2.0 < v1.10.0 (numeric, not lexical)")
	}
	cases := []struct {
		c    string
		v    string
		want bool
	}{
		{"v1", "v1.5.0", true}, {"v1", "v2.0.0", false},
		{"v1.2", "v1.2.9", true}, {"v1.2", "v1.3.0", false},
		{"", "v9.9.9", true}, {"latest", "v0.0.1", true},
	}
	for _, c := range cases {
		v, _ := parseSemver(c.v)
		if matchConstraint(c.c, v) != c.want {
			t.Errorf("matchConstraint(%q,%q) = %v, want %v", c.c, c.v, !c.want, c.want)
		}
	}
	for c, want := range map[string]bool{"v1": true, "v1.2": true, "latest": true, "v1.2.3": false, "main": false, "abc": false} {
		if isConstraint(c) != want {
			t.Errorf("isConstraint(%q) = %v, want %v", c, !want, want)
		}
	}
}

func TestHashAndLock(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.funk"), []byte("one"), 0o644)
	h1, err := hashPkg(dir)
	if err != nil || h1 == "" {
		t.Fatalf("hashPkg: %q %v", h1, err)
	}
	os.WriteFile(filepath.Join(dir, "a.funk"), []byte("two"), 0o644)
	if h2, _ := hashPkg(dir); h2 == h1 {
		t.Fatal("hash should change when content changes")
	}

	lock := filepath.Join(t.TempDir(), "funk.lock")
	if err := writeLockEntry(lock, "p@v1.0.0", "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if readLock(lock)["p@v1.0.0"] != "deadbeef" {
		t.Fatal("lock round-trip failed")
	}
}

// makeRepo builds a throwaway git repo with a funk package and version tags.
func makeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	os.WriteFile(filepath.Join(dir, "p.funk"), []byte("package \"ex/p\" {\n  version 0.1.0\n}\nfn f { in (x Num) out (r Num) engine builtin src \"num.inc\" }"), 0o644)
	git("add", "-A")
	git("commit", "-qm", "1")
	for _, tag := range []string{"v0.1.0", "v0.2.0", "v1.0.0", "v1.1.0"} {
		git("tag", tag)
	}
	return dir
}

func TestResolveVersion(t *testing.T) {
	r := makeRepo(t)
	for c, want := range map[string]string{"v0": "v0.2.0", "v1": "v1.1.0", "latest": "v1.1.0", "": "v1.1.0"} {
		if got, err := resolveVersion(r, c); err != nil || got != want {
			t.Errorf("resolveVersion(%q) = %q, %v; want %q", c, got, err, want)
		}
	}
	if _, err := resolveVersion(r, "v9"); err == nil {
		t.Error("v9 should not resolve")
	}
}

func TestGetLocksAndVerifies(t *testing.T) {
	r := makeRepo(t)
	cache := t.TempDir()
	lock := filepath.Join(t.TempDir(), "funk.lock")
	t.Setenv("FUNK_CACHE", cache)
	t.Setenv("FUNK_LOCK", lock)

	if err := cmdGet([]string{r + "@v1", "p"}); err != nil {
		t.Fatalf("get @v1: %v", err)
	}
	if readLock(lock)["p@v1.1.0"] == "" {
		t.Fatalf("lockfile missing p@v1.1.0: %v", readLock(lock))
	}
	if err := verifyLock(lock, cache); err != nil {
		t.Fatalf("verify (clean) should pass: %v", err)
	}
	// tampering the cache breaks integrity
	os.WriteFile(filepath.Join(cache, "p", "p.funk"), []byte("; tampered"), 0o644)
	if err := verifyLock(lock, cache); err == nil {
		t.Fatal("verify should fail after tampering the cache")
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// expandBinding is the secret-input resolver: it must keep credentials off the
// command line (@file / @- / env:VAR) and fail clearly on a missing source.
func TestExpandBinding(t *testing.T) {
	if v, err := expandBinding("plain"); err != nil || v != "plain" {
		t.Fatalf("literal = %q, %v (want plain)", v, err)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte("secret-val\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// @file reads the file and trims the trailing newline
	if v, err := expandBinding("@" + p); err != nil || v != "secret-val" {
		t.Fatalf("@file = %q, %v (want secret-val)", v, err)
	}

	t.Setenv("FUNK_TEST_TOK", "env-val")
	if v, err := expandBinding("env:FUNK_TEST_TOK"); err != nil || v != "env-val" {
		t.Fatalf("env: = %q, %v (want env-val)", v, err)
	}

	if _, err := expandBinding("env:FUNK_TEST_NOPE_UNSET"); err == nil {
		t.Fatal("expected an error for an unset env var")
	}
	if _, err := expandBinding("@" + filepath.Join(dir, "nope")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

// splitRef must peel a trailing @ref without tripping on the @ in an scp URL.
func TestSplitRef(t *testing.T) {
	cases := []struct {
		in, repo, ref string
	}{
		{"git@github.com:u/r.git", "git@github.com:u/r.git", ""},
		{"git@github.com:u/r.git@v1.2.0", "git@github.com:u/r.git", "v1.2.0"},
		{"https://github.com/u/r.git", "https://github.com/u/r.git", ""},
		{"https://github.com/u/r@v0.1.0", "https://github.com/u/r", "v0.1.0"},
		{"./local/lib@main", "./local/lib", "main"},
		{"./local/lib", "./local/lib", ""},
	}
	for _, c := range cases {
		repo, ref := splitRef(c.in)
		if repo != c.repo || ref != c.ref {
			t.Errorf("splitRef(%q) = (%q,%q), want (%q,%q)", c.in, repo, ref, c.repo, c.ref)
		}
	}
}

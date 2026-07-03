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

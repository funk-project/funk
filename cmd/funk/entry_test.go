package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFunk writes content to a temp .funk file and isolates it from the on-disk
// stdlib (FUNK_STD → empty), so cmdRun sees only this file.
func writeFunk(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FUNK_STD", filepath.Join(dir, "no-std"))
	p := filepath.Join(dir, "prog.funk")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const opsLib = `package "ops" {
  version 0.0.1
}
fn deploy {
  main
  in () out (r Str)
  body (flush (r "deployed"))
}
fn rollback {
  main
  in () out (r Str)
  body (flush (r "rolled back"))
}
fn helper {
  in (x Num) out (r Num)
  engine builtin
  src "num.inc"
}`

func TestRunFileMultipleMains(t *testing.T) {
	p := writeFunk(t, opsLib)
	_, err := captureStdout(t, func() error { return cmdRun([]string{p}) })
	if err == nil || !strings.Contains(err.Error(), "multiple entry points") {
		t.Fatalf("expected a multiple-entry-points error, got %v", err)
	}
	// both names must be listed
	if !strings.Contains(err.Error(), "deploy") || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("error should list the mains: %v", err)
	}
}

func TestRunFileMainByName(t *testing.T) {
	p := writeFunk(t, opsLib)
	out, err := captureStdout(t, func() error { return cmdRun([]string{p, "rollback"}) })
	if err != nil || !strings.Contains(out, "rolled back") {
		t.Fatalf("cmdRun rollback: err=%v out=%q", err, out)
	}
}

func TestRunFileHelperNotRunnable(t *testing.T) {
	p := writeFunk(t, opsLib)
	_, err := captureStdout(t, func() error { return cmdRun([]string{p, "helper", "x=5"}) })
	if err == nil || !strings.Contains(err.Error(), "not a runnable entry point") {
		t.Fatalf("expected a not-runnable error, got %v", err)
	}
}

func TestRunFileNoMain(t *testing.T) {
	p := writeFunk(t, `package "lib" {
  version 0.0.1
}
fn add2 { in (a Num) out (r Num) engine builtin src "num.inc" }`)
	_, err := captureStdout(t, func() error { return cmdRun([]string{p}) })
	if err == nil || !strings.Contains(err.Error(), "no `main`") {
		t.Fatalf("expected a no-main error, got %v", err)
	}
}

func TestRunFileSingleMain(t *testing.T) {
	p := writeFunk(t, `package "p" {
  version 0.0.1
}
fn start {
  main
  in () out (r Str)
  body (flush (r "hi"))
}`)
	out, err := captureStdout(t, func() error { return cmdRun([]string{p}) })
	if err != nil || !strings.Contains(out, "hi") {
		t.Fatalf("cmdRun single main: err=%v out=%q", err, out)
	}
}

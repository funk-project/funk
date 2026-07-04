package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A small self-contained library the CLI commands operate on.
const cliLib = `package "test/cli" {
  version 0.0.1
}

type Point { x Num  y Num }

fn add  { in (a Num) (b Num) out (r Num) engine builtin src "num.add" }
fn bump { in (x Num) out (r Num) body (flush (r (add x 100)))
  test (is (bump 5) 105) }
`

// writeLib writes cliLib to a temp file and points FUNK_STD at an empty dir so
// loadLibrary does not pull in the on-disk stdlib (test isolation).
func writeLib(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FUNK_STD", filepath.Join(dir, "no-std"))
	p := filepath.Join(dir, "lib.funk")
	if err := os.WriteFile(p, []byte(cliLib), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what it
// wrote, alongside fn's error.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = orig
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, e := r.Read(buf)
		sb.Write(buf[:n])
		if e != nil {
			break
		}
	}
	r.Close()
	return sb.String(), runErr
}

func TestCmdParseAndFmt(t *testing.T) {
	p := writeLib(t)
	if out, err := captureStdout(t, func() error { return cmdParse([]string{p}) }); err != nil || !strings.Contains(out, `"Head"`) {
		t.Fatalf("cmdParse: err=%v out=%q", err, out)
	}
	if out, err := captureStdout(t, func() error { return cmdFmt([]string{p}) }); err != nil || !strings.Contains(out, "fn bump") {
		t.Fatalf("cmdFmt: err=%v out=%q", err, out)
	}
	// -w rewrites the file in place; it must still parse.
	if _, err := captureStdout(t, func() error { return cmdFmt([]string{"-w", p}) }); err != nil {
		t.Fatalf("cmdFmt -w: %v", err)
	}
}

func TestCmdListTypesIntrospect(t *testing.T) {
	p := writeLib(t)
	if out, err := captureStdout(t, func() error { return cmdList([]string{"-f", p}) }); err != nil || !strings.Contains(out, "bump") {
		t.Fatalf("cmdList: err=%v out=%q", err, out)
	}
	if out, err := captureStdout(t, func() error { return cmdTypes([]string{"-f", p}) }); err != nil || !strings.Contains(out, "Point") {
		t.Fatalf("cmdTypes: err=%v out=%q", err, out)
	}
	if out, err := captureStdout(t, func() error { return cmdIntrospect([]string{"-f", p, "bump"}) }); err != nil || !strings.Contains(out, "composite") {
		t.Fatalf("cmdIntrospect: err=%v out=%q", err, out)
	}
	if _, err := captureStdout(t, func() error { return cmdIntrospect([]string{"-f", p, "nope"}) }); err == nil {
		t.Fatal("cmdIntrospect(nope) should error")
	}
}

func TestCmdCheckAndTest(t *testing.T) {
	p := writeLib(t)
	if out, err := captureStdout(t, func() error { return cmdCheck([]string{"-f", p}) }); err != nil || !strings.Contains(out, "no issues") {
		t.Fatalf("cmdCheck: err=%v out=%q", err, out)
	}
	if out, err := captureStdout(t, func() error { return cmdTest([]string{"-f", p}) }); err != nil || !strings.Contains(out, "1 passed") {
		t.Fatalf("cmdTest: err=%v out=%q", err, out)
	}
}

func TestCmdRun(t *testing.T) {
	p := writeLib(t)
	out, err := captureStdout(t, func() error { return cmdRun([]string{"-f", p, "bump", "x=5"}) })
	if err != nil || !strings.Contains(out, "105") {
		t.Fatalf("cmdRun bump x=5: err=%v out=%q", err, out)
	}
	// a malformed input (no '=') is a clear error
	if _, err := captureStdout(t, func() error { return cmdRun([]string{"-f", p, "bump", "5"}) }); err == nil {
		t.Fatal("cmdRun with bad input should error")
	}
}

func TestCmdGraphAndDoc(t *testing.T) {
	p := writeLib(t)
	if out, err := captureStdout(t, func() error { return cmdGraph([]string{"-f", p, "bump"}) }); err != nil || !strings.Contains(out, "bump") {
		t.Fatalf("cmdGraph: err=%v out=%q", err, out)
	}
	if out, err := captureStdout(t, func() error { return cmdGraph([]string{"-f", p, "--json", "bump"}) }); err != nil || !strings.Contains(out, `"id"`) {
		t.Fatalf("cmdGraph --json: err=%v out=%q", err, out)
	}
	if out, err := captureStdout(t, func() error { return cmdDoc([]string{"-f", p}) }); err != nil || !strings.Contains(out, "bump") {
		t.Fatalf("cmdDoc: err=%v out=%q", err, out)
	}
}

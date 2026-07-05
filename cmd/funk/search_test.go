package main

import (
	"strings"
	"testing"
)

const searchLib = `package "s" {
  version 0.0.1
}
fn mul {
  name "Multiplication"
  doc "Multiply two numbers"
  in (a Num) (b Num) out (r Num)
  engine builtin
  src "num.mul"
}
fn gt {
  name "Greater Than"
  doc "Test whether a is greater than b"
  in (a Num) (b Num) out (r Bool)
  engine builtin
  src "num.gt"
}
fn mean {
  name "Mean"
  doc "Arithmetic mean of a list"
  in (xs (List Num)) out (r Num)
  engine builtin
  src "list.first"
}`

func TestSearchText(t *testing.T) {
	p := writeFunk(t, searchLib)
	out, err := captureStdout(t, func() error { return cmdSearch([]string{"-f", p, "multiply"}) })
	if err != nil || !strings.Contains(out, "mul") {
		t.Fatalf("text search 'multiply': err=%v out=%q", err, out)
	}
	// a term only in the name/display should also match
	out, _ = captureStdout(t, func() error { return cmdSearch([]string{"-f", p, "arithmetic"}) })
	if !strings.Contains(out, "mean") {
		t.Fatalf("text search 'arithmetic' should find mean: %q", out)
	}
}

func TestSearchSignature(t *testing.T) {
	p := writeFunk(t, searchLib)
	// Num Num -> Bool matches gt, not mul or mean
	out, err := captureStdout(t, func() error { return cmdSearch([]string{"-f", p, "Num Num -> Bool"}) })
	if err != nil || !strings.Contains(out, "gt") {
		t.Fatalf("signature Num Num -> Bool should find gt: %q", out)
	}
	if strings.Contains(out, "mean") || strings.Contains(out, "/mul") {
		t.Fatalf("signature Num Num -> Bool should not match mul/mean: %q", out)
	}
	// List -> Num matches mean (List<Num> base is List)
	out, _ = captureStdout(t, func() error { return cmdSearch([]string{"-f", p, "List -> Num"}) })
	if !strings.Contains(out, "mean") {
		t.Fatalf("signature List -> Num should find mean: %q", out)
	}
}

func TestSearchJSONAndEmpty(t *testing.T) {
	p := writeFunk(t, searchLib)
	out, err := captureStdout(t, func() error { return cmdSearch([]string{"-f", p, "--json", "multiply"}) })
	if err != nil || !strings.Contains(out, `"address"`) || !strings.Contains(out, "mul") {
		t.Fatalf("--json search: err=%v out=%q", err, out)
	}
	out, _ = captureStdout(t, func() error { return cmdSearch([]string{"-f", p, "zzzznotfound"}) })
	if !strings.Contains(out, "no matches") {
		t.Fatalf("expected 'no matches', got %q", out)
	}
}

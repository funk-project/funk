package main

import (
	"strings"
	"testing"
)

func TestContext(t *testing.T) {
	p := writeFunk(t, `package "s" {
  version 0.0.1
}
fn mean { name "Mean" doc "arithmetic average of a list" in (xs (List Num)) out (r Num) engine builtin src "list.first" }
fn gt   { name "Greater Than" doc "greater than test" in (a Num) (b Num) out (r Bool) engine builtin src "num.gt" }`)

	out, err := captureStdout(t, func() error { return cmdContext([]string{"-f", p, "average of a list"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "funk is a small language") {
		t.Fatal("context should include the primer")
	}
	if !strings.Contains(out, "# Your task") || !strings.Contains(out, "average of a list") {
		t.Fatal("context should state the task")
	}
	if !strings.Contains(out, "mean") {
		t.Fatalf("context should surface the reusable `mean`: %q", out)
	}
	if _, err := captureStdout(t, func() error { return cmdContext([]string{"-f", p}) }); err == nil {
		t.Fatal("context with no task should error")
	}
}

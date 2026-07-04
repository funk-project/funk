package main

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/funk-project/funk/internal/funk"
)

func TestDeriveName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/u/lib.git": "lib",
		"https://github.com/u/lib/":    "lib",
		"git@github.com:u/repo.git":    "repo",
		"./local/thing":                "thing",
		"solo":                         "solo",
	}
	for in, want := range cases {
		if got := deriveName(in); got != want {
			t.Errorf("deriveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanFunkAndFnName(t *testing.T) {
	raw := "Here you go:\n```funk\nfn foo { in (x Num) out (r Num) body (flush (r x)) }\n```\nHope that helps!"
	got := cleanFunk(raw)
	if !strings.HasPrefix(got, "fn foo {") || !strings.HasSuffix(got, "}") {
		t.Fatalf("cleanFunk did not isolate the fn block: %q", got)
	}
	if fnName(got) != "foo" {
		t.Fatalf("fnName = %q, want foo", fnName(got))
	}
	if fnName("no function here") != "" {
		t.Fatal("fnName should be empty when there is no fn")
	}
}

func TestIssuesForAndTestFailures(t *testing.T) {
	issues := []funk.Issue{
		{Fn: "target", Msg: "bad thing"},
		{Fn: "other", Msg: "unrelated"},
		{Fn: "target", Msg: "advisory", Warn: true},
	}
	got := issuesFor(issues, "target")
	if len(got) != 1 || !strings.Contains(got[0], "bad thing") {
		t.Fatalf("issuesFor = %v, want the one non-warn issue for target", got)
	}

	lib := funk.NewLibrary()
	if err := lib.LoadString(`package "t/x" {
  version 0.0.1
}
fn addp { in (a Num) (b Num) out (r Num) engine builtin src "num.add"
  test (is (addp 1 2) 99) }`); err != nil {
		t.Fatal(err)
	}
	fails := testFailures(lib, "addp")
	if len(fails) != 1 || !strings.Contains(fails[0], "test failed") {
		t.Fatalf("testFailures = %v, want one failure", fails)
	}
	if len(testFailures(lib, "nope")) != 0 {
		t.Fatal("testFailures for an unknown fn should be empty")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, map[string]int{"a": 1})
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `"a"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestUsagePrints(t *testing.T) {
	orig := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	usage()
	w.Close()
	os.Stderr = orig
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, e := r.Read(buf)
		sb.Write(buf[:n])
		if e != nil {
			break
		}
	}
	if !strings.Contains(sb.String(), "funk") {
		t.Fatalf("usage output = %q", sb.String())
	}
}

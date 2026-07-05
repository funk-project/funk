package funk

import (
	"strings"
	"testing"
)

func loadOne(t *testing.T, src string, name string) *Fn {
	t.Helper()
	lib := NewLibrary()
	if err := lib.LoadString(src); err != nil {
		t.Fatalf("load: %v", err)
	}
	f, ok := lib.Lookup(name)
	if !ok {
		t.Fatalf("fn %q not found", name)
	}
	return f
}

// A triple-quoted string is captured across lines, with a leading newline
// dropped, the block dedented, and trailing whitespace trimmed.
func TestTripleQuotedDocDedents(t *testing.T) {
	f := loadOne(t, `package "t/d" {
  version 0.0.1
}
fn g {
  doc """
    First line.

    Second paragraph, indented the same.
  """
  in (x Num) out (r Num) body (flush (r x))
}`, "g")
	want := "First line.\n\nSecond paragraph, indented the same."
	if f.Doc != want {
		t.Fatalf("doc = %q, want %q", f.Doc, want)
	}
}

func TestTripleQuotedInline(t *testing.T) {
	f := loadOne(t, `package "t/d" {
  version 0.0.1
}
fn g { doc """all on one line""" in (x Num) out (r Num) body (flush (r x)) }`, "g")
	if f.Doc != "all on one line" {
		t.Fatalf("doc = %q", f.Doc)
	}
}

// Triple-quoted strings may embed quotes and are not escape-processed (raw).
func TestTripleQuotedRawContent(t *testing.T) {
	f := loadOne(t, `package "t/d" {
  version 0.0.1
}
fn g { doc """a "quoted" word and a \n backslash-n literally""" in (x Num) out (r Num) body (flush (r x)) }`, "g")
	if !strings.Contains(f.Doc, `"quoted"`) || !strings.Contains(f.Doc, `\n`) {
		t.Fatalf("raw content lost: %q", f.Doc)
	}
}

// (doc "…") on a port sets its description, on both inputs and outputs.
func TestPortDescriptions(t *testing.T) {
	f := loadOne(t, `package "t/d" {
  version 0.0.1
}
fn g {
  in  (a Num (doc "the addend")) (b Num (min 0) (doc "must be non-negative"))
  out (r Num (doc "the sum"))
  body (flush (r a))
}`, "g")
	if f.In[0].Doc != "the addend" {
		t.Fatalf("in[0].Doc = %q", f.In[0].Doc)
	}
	if f.In[1].Doc != "must be non-negative" || f.In[1].Min == nil {
		t.Fatalf("in[1] doc/min not both parsed: doc=%q min=%v", f.In[1].Doc, f.In[1].Min)
	}
	if f.Out[0].Doc != "the sum" {
		t.Fatalf("out[0].Doc = %q", f.Out[0].Doc)
	}
}

// Ports may be written across multiple lines (a '(' on a following line
// continues the field), so long descriptions stay readable.
func TestMultiLineInputPorts(t *testing.T) {
	f := loadOne(t, `package "t/d" {
  version 0.0.1
}
fn g {
  in  (a Num (doc "first"))
      (b Num (doc "second"))
  out (r Num)
  body (flush (r a))
}`, "g")
	if len(f.In) != 2 || f.In[0].Doc != "first" || f.In[1].Doc != "second" {
		t.Fatalf("multi-line ports = %+v", f.In)
	}
}

// The `examples` field holds markdown usage examples (triple-quoted), separate
// from executable `test` assertions, and is surfaced by introspection.
func TestExamplesField(t *testing.T) {
	src := `package "t/d" {
  version 0.0.1
}
fn g {
  doc "adds one"
  in (x Num (doc "the input")) out (r Num (doc "x + 1"))
  examples """
    - (g 1) -> 2
    - (g 9) -> 10
  """
  engine builtin
  src "num.inc"
}`
	f := loadOne(t, src, "g")
	want := "- (g 1) -> 2\n- (g 9) -> 10"
	if f.Examples != want {
		t.Fatalf("examples = %q, want %q", f.Examples, want)
	}
	lib := NewLibrary()
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	in, _ := Introspect(lib, "g")
	if in.Examples != want {
		t.Fatalf("introspect examples = %q", in.Examples)
	}
	// round-trips through the formatter as a triple-quoted block
	prog, _ := Parse(src)
	out := Format(prog)
	lib2 := NewLibrary()
	if err := lib2.LoadString(out); err != nil {
		t.Fatalf("reparse: %v\n%s", err, out)
	}
	if g, _ := lib2.Lookup("g"); g.Examples != want {
		t.Fatalf("round-tripped examples = %q", g.Examples)
	}
}

func TestUnterminatedTripleQuote(t *testing.T) {
	_, err := Parse(`fn g { doc """never closed`)
	if err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("expected unterminated-string error, got %v", err)
	}
}

// Format renders a multi-line string as a """ block that re-parses to the same
// value (round-trip through the dedent).
func TestFormatMultiLineRoundTrip(t *testing.T) {
	src := `package "t/d" {
  version 0.0.1
}
fn g {
  doc """
    line one
    line two
  """
  in (x Num) out (r Num) body (flush (r x))
}`
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	out := Format(prog)
	if !strings.Contains(out, `"""`) {
		t.Fatalf("Format did not emit a triple-quoted block:\n%s", out)
	}
	// re-parse and confirm the doc value survives
	lib := NewLibrary()
	if err := lib.LoadString(out); err != nil {
		t.Fatalf("reparse formatted: %v\n%s", err, out)
	}
	g, _ := lib.Lookup("g")
	if g.Doc != "line one\nline two" {
		t.Fatalf("round-tripped doc = %q, want %q", g.Doc, "line one\nline two")
	}
}

func TestCleandoc(t *testing.T) {
	// following lines dedented by their shared indent; relative indent preserved
	if got := cleandoc("    a\n      b\n    c"); got != "a\n  b\nc" {
		t.Fatalf("cleandoc = %q", got)
	}
	// blank lines ignored for the common prefix and normalized to empty
	if got := cleandoc("  a\n\n  b"); got != "a\n\nb" {
		t.Fatalf("cleandoc with blank = %q", got)
	}
	// YAML/PEP-257: an inline first line, continuation aligned under the text —
	// the first line is stripped on its own, the rest dedented by their indent
	if got := cleandoc("# X\n     ### sub\n  "); got != "# X\n### sub" {
		t.Fatalf("cleandoc inline-first = %q", got)
	}
}

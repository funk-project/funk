package funk

import "testing"

// An `alias TARGET` fn delegates to TARGET, inheriting its signature (and name if
// it declared none), and runs as a normal composite.
func TestAliasDelegatesAndInherits(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "test/m" {
  version 0.0.1
}
fn mul { in (a Num) (b Num) out (r Num) engine builtin src "num.mul" }`); err != nil {
		t.Fatal(err)
	}
	if err := lib.LoadString(`package "test/app" {
  version 0.0.1
  use "test/m" as m
}
fn myMul {
  alias m.mul
  name "My Multiply"
}`); err != nil {
		t.Fatal(err)
	}
	if err := lib.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	f, _ := lib.Lookup("myMul")
	if len(f.In) != 2 || len(f.Out) != 1 || f.Out[0].Type != "Num" {
		t.Fatalf("myMul did not inherit mul's signature: in=%v out=%v", f.In, f.Out)
	}
	if f.Display != "My Multiply" {
		t.Fatalf("myMul display = %q, want its own name", f.Display)
	}
	if !f.Composite() {
		t.Fatal("an alias should resolve to a composite body")
	}

	res := Run(lib, "myMul", map[string]interface{}{"a": 6.0, "b": 7.0}, ExecOpts{})
	if !res.OK || mustNum(t, res.Value) != 42 {
		t.Fatalf("myMul(6,7) = %v (%s), want 42", res.Value, res.Error)
	}
}

func TestAliasUnknownTarget(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "p" {
  version 0.0.1
}
fn a { alias nope }`); err != nil {
		t.Fatal(err)
	}
	if err := lib.Finalize(); err == nil {
		t.Fatal("expected an error for an unknown alias target")
	}
}

func TestAliasExclusiveWithSrc(t *testing.T) {
	lib := NewLibrary()
	err := lib.LoadString(`package "p" {
  version 0.0.1
}
fn x { alias y in () out (r Num) engine builtin src "num.inc" }`)
	if err == nil {
		t.Fatal("alias + src should be a parse error")
	}
}

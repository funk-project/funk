package funk

import (
	"strings"
	"testing"
)

// The stdlib migrated to `use … as` (docs/03 §6): a bare name resolves only within
// its own package, cross-package calls go through an aliased import, and a plain
// `use` (no alias) is a check error. These tests pin that behaviour directly.

const mathsPkg = `package "test/maths" {
  version 0.0.1
}
fn add { in (a Num) (b Num) out (r Num) engine builtin src "num.add" }
fn inc { in (a Num)         out (r Num) engine builtin src "num.inc" }
`

// loadTwo loads a maths package plus one app package that shares the same library.
func loadTwo(t *testing.T, appSrc string) *Library {
	t.Helper()
	lib := NewLibrary()
	if err := lib.LoadString(mathsPkg); err != nil {
		t.Fatalf("load maths: %v", err)
	}
	if err := lib.LoadString(appSrc); err != nil {
		t.Fatalf("load app: %v", err)
	}
	return lib
}

func hasIssue(issues []Issue, substr string) bool {
	for _, i := range issues {
		if strings.Contains(i.Msg, substr) {
			return true
		}
	}
	return false
}

func TestUseAliasResolvesAndRuns(t *testing.T) {
	lib := loadTwo(t, `package "test/app" {
  version 0.0.1
  use "test/maths" as m
}
fn bump { in (x Num) out (r Num) body (flush (r (m.add x 100))) }`)

	if issues := Check(lib); len(issues) != 0 {
		t.Fatalf("expected clean check, got %v", issues)
	}
	res := Run(lib, "bump", map[string]interface{}{"x": 5.0}, ExecOpts{})
	if !res.OK || mustNum(t, res.Value) != 105 {
		t.Fatalf("bump 5 via m.add: ok=%v value=%v", res.OK, res.Value)
	}
}

func TestBareCrossPackageIsRejected(t *testing.T) {
	// app imports maths (aliased) but calls `add` bare — no implicit global namespace.
	lib := loadTwo(t, `package "test/app" {
  version 0.0.1
  use "test/maths" as m
}
fn bump { in (x Num) out (r Num) body (flush (r (add x 100))) }`)

	if !hasIssue(Check(lib), `unknown function "add"`) {
		t.Fatalf("expected bare cross-package `add` to be unknown, got %v", Check(lib))
	}
	if res := Run(lib, "bump", map[string]interface{}{"x": 5.0}, ExecOpts{}); res.OK {
		t.Fatalf("expected bump to fail resolving bare `add`, got value %v", res.Value)
	}
}

func TestPlainUseWithoutAliasIsError(t *testing.T) {
	lib := loadTwo(t, `package "test/app" {
  version 0.0.1
  use "test/maths"
}
fn noop { in (x Num) out (r Num) body (flush (r x)) }`)

	if !hasIssue(Check(lib), "must declare an alias") {
		t.Fatalf("expected plain `use` to be a check error, got %v", Check(lib))
	}
}

func TestFullAddressAlwaysResolves(t *testing.T) {
	// A fully-qualified address bypasses the alias rules — callable without a `use`.
	lib := loadTwo(t, `package "test/app" {
  version 0.0.1
}
fn bump { in (x Num) out (r Num) body (flush (r (test/maths/add x 1))) }`)

	if issues := Check(lib); len(issues) != 0 {
		t.Fatalf("expected clean check for address call, got %v", issues)
	}
	res := Run(lib, "bump", map[string]interface{}{"x": 41.0}, ExecOpts{})
	if !res.OK || mustNum(t, res.Value) != 42 {
		t.Fatalf("bump 41 via address: ok=%v value=%v", res.OK, res.Value)
	}
}

func TestIntraPackageBareStillResolves(t *testing.T) {
	// Within one package, bare names still resolve (no `use` needed for siblings).
	lib := NewLibrary()
	if err := lib.LoadString(`package "test/solo" {
  version 0.0.1
}
fn inc  { in (a Num) out (r Num) engine builtin src "num.inc" }
fn inc2 { in (x Num) out (r Num) body (flush (r (inc (inc x)))) }`); err != nil {
		t.Fatal(err)
	}
	if issues := Check(lib); len(issues) != 0 {
		t.Fatalf("expected clean check, got %v", issues)
	}
	res := Run(lib, "inc2", map[string]interface{}{"x": 5.0}, ExecOpts{})
	if !res.OK || mustNum(t, res.Value) != 7 {
		t.Fatalf("inc2 5: ok=%v value=%v", res.OK, res.Value)
	}
}

// A qualified `alias.fn` passed as a first-class VALUE resolves through the
// caller's scope and is stored as an address, so it invokes from any scope.
func TestQualifiedHigherOrderValue(t *testing.T) {
	lib := loadTwo(t, `package "test/app" {
  version 0.0.1
  use "test/maths" as m
}
fn applyF   { in (f Fn) (x Num) out (r Num) body (flush (r (f x))) }
fn viaAlias { in (x Num) out (r Num) body (flush (r (applyF m.inc x))) }`)

	if issues := Check(lib); len(issues) != 0 {
		t.Fatalf("expected clean check, got %v", issues)
	}
	// the passed value must carry the fully-qualified address
	f, _ := lib.Lookup("viaAlias")
	if v, err := (&evalEnv{lib: lib, curFn: f, vars: map[string]interface{}{}}).eval(Atom{Kind: "id", Value: "m.inc"}); err != nil {
		t.Fatalf("resolve m.inc as value: %v", err)
	} else if fv, ok := v.(FnValue); !ok || fv.Ref != "test/maths/inc" {
		t.Fatalf("m.inc value = %#v, want FnValue{Ref:\"test/maths/inc\"}", v)
	}
	res := Run(lib, "viaAlias", map[string]interface{}{"x": 5.0}, ExecOpts{})
	if !res.OK || mustNum(t, res.Value) != 6 {
		t.Fatalf("viaAlias 5 = %v (%s), want 6", res.Value, res.Error)
	}
}

// A versioned remote import parses the quoted version: use "url" "v1.2.0" as ml.
func TestVersionedUseParses(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "app" {
  version 0.1.0
  use "github.com/u/lib" "v1.2.0" as ml
}
fn f { in () out (r Num) body (flush (r 1)) }`); err != nil {
		t.Fatal(err)
	}
	f, _ := lib.Lookup("f")
	var got *UseSpec
	for i := range f.Uses {
		if f.Uses[i].Alias == "ml" {
			got = &f.Uses[i]
		}
	}
	if got == nil || got.Pkg != "github.com/u/lib" || got.Version != "v1.2.0" {
		t.Fatalf("versioned use not parsed: %+v", f.Uses)
	}
}

func TestUnknownAliasMemberIsRejected(t *testing.T) {
	lib := loadTwo(t, `package "test/app" {
  version 0.0.1
  use "test/maths" as m
}
fn bump { in (x Num) out (r Num) body (flush (r (m.nope x))) }`)

	if !hasIssue(Check(lib), `unknown function "m.nope"`) {
		t.Fatalf("expected m.nope to be unknown, got %v", Check(lib))
	}
}

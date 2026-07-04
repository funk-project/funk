package funk

import (
	"reflect"
	"strings"
	"testing"
)

// prims are the atomic builtins the reactive tests wire together.
const prims = `
fn add    { in (a Num) (b Num) out (r Num) engine builtin src "num.add" }
fn div    { in (a Num) (b Num) out (r Num) engine builtin src "num.div" }
fn mod    { in (a Num) (b Num) out (r Num) engine builtin src "num.mod" }
fn double { in (a Num)         out (r Num) engine builtin src "num.double" }
`

func loadRx(t *testing.T, src string) *Library {
	t.Helper()
	lib := NewLibrary()
	if err := lib.LoadString(prims + src); err != nil {
		t.Fatal(err)
	}
	return lib
}

func numList(t *testing.T, v interface{}) []float64 {
	t.Helper()
	items, ok := v.([]interface{})
	if !ok {
		t.Fatalf("not a list: %#v", v)
	}
	out := make([]float64, len(items))
	for i, it := range items {
		n, err := toNum(it)
		if err != nil {
			t.Fatalf("item %d not a number: %#v", i, it)
		}
		out[i] = n
	}
	return out
}

// A scalar function applied to a stream fires per item (the reactive lift).
func TestReactiveLiftPerItem(t *testing.T) {
	lib := loadRx(t, `fn doubles { in (n Num) out (r Stream<Num>) body (double (range 1 n)) }`)
	res := Run(lib, "doubles", map[string]interface{}{"n": 4.0}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if got := numList(t, res.Value); !reflect.DeepEqual(got, []float64{2, 4, 6, 8}) {
		t.Fatalf("doubles(4) = %v, want [2 4 6 8]", got)
	}
}

// zip pairs two driving streams 1:1.
func TestReactiveZipFiring(t *testing.T) {
	lib := loadRx(t, `fn zipAdd { in () out (r Stream<Num>) body (add (range 1 3) (range 10 12)) }`)
	res := Run(lib, "zipAdd", map[string]interface{}{}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if got := numList(t, res.Value); !reflect.DeepEqual(got, []float64{11, 13, 15}) {
		t.Fatalf("zipAdd = %v, want [11 13 15]", got)
	}
}

// Named multi-output + flush, then destructured and re-wired by the caller.
func TestMultiOutputWiring(t *testing.T) {
	lib := loadRx(t, `
fn divmod { in (zip (a Num) (b Num)) out (q Num) (r Num) body (flush (q (div a b)) (r (mod a b))) }
fn useboth { in (zip (a Num) (b Num)) out (r Num) body (let (q r (divmod a b)) (flush (r (add q r)))) }`)
	// divmod returns a tuple map
	res := Run(lib, "divmod", map[string]interface{}{"a": 17.0, "b": 5.0}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	m, ok := res.Value.(map[string]interface{})
	if !ok {
		t.Fatalf("divmod value not a tuple: %#v", res.Value)
	}
	if mustNum(t, m["r"]) != 2 {
		t.Fatalf("divmod.r = %v, want 2", m["r"])
	}
	// useboth destructures and re-wires q,r into add: 10/3≈3.333, mod=1 → ≈4.333
	res = Run(lib, "useboth", map[string]interface{}{"a": 10.0, "b": 3.0}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if got := mustNum(t, res.Value); got < 4.3 || got > 4.4 {
		t.Fatalf("useboth(10,3) = %v, want ≈4.33", got)
	}
}

// (min)/(max) enforced on inputs (arrival) and outputs (post-condition on flush).
func TestPortSpecValidation(t *testing.T) {
	lib := loadRx(t, `
fn clamped { in (x Num (min 0) (max 10)) out (r Num) body (flush (r (double x))) }
fn pos     { in (x Num) out (r Num (min 0)) body (flush (r x)) }`)
	if res := Run(lib, "clamped", map[string]interface{}{"x": 99.0}, ExecOpts{}); res.OK {
		t.Fatal("clamped(99) should fail max")
	}
	if res := Run(lib, "clamped", map[string]interface{}{"x": 4.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 8 {
		t.Fatalf("clamped(4) = %v (%s), want 8", res.Value, res.Error)
	}
	if res := Run(lib, "pos", map[string]interface{}{"x": -5.0}, ExecOpts{}); res.OK {
		t.Fatal("pos(-5) should fail output min")
	}
}

// A latest port with a default is optional; the default fills in when omitted.
func TestLatestDefaultOptional(t *testing.T) {
	lib := loadRx(t, `fn scale { in (latest (x Num) (y Num (default 1))) out (r Num) body (flush (r (add x y))) }`)
	if res := Run(lib, "scale", map[string]interface{}{"x": 5.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 6 {
		t.Fatalf("scale(5) = %v (%s), want 6 (y defaults to 1)", res.Value, res.Error)
	}
}

// The connection type-check flags a clear scalar out→in mismatch.
func TestConnectionTypeCheck(t *testing.T) {
	lib := loadRx(t, `
fn g   { in (n Num) out (r Num) engine builtin src "num.double" }
fn bad { in (s Str) out (r Num) body (flush (r (g s))) }`)
	var found bool
	for _, is := range Check(lib) {
		if strings.Contains(is.Msg, "connection type mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a connection type mismatch for (g s) with s:Str into n:Num")
	}
}

// The optional `name` field is a display label; it defaults to the fn identifier.
func TestDisplayName(t *testing.T) {
	lib := loadRx(t, `
fn add2  { name "Add Two" in (a Num) (b Num) out (r Num) engine builtin src "num.add" }
fn plain { in (a Num) out (r Num) engine builtin src "num.double" }`)
	if f, _ := lib.Lookup("add2"); f.DisplayName() != "Add Two" {
		t.Fatalf("add2 display = %q, want \"Add Two\"", f.DisplayName())
	}
	if f, _ := lib.Lookup("plain"); f.DisplayName() != "plain" {
		t.Fatalf("plain display = %q, want fallback \"plain\"", f.DisplayName())
	}
}

// return is gone: using it is a clear error, not a silent no-op.
func TestReturnRemoved(t *testing.T) {
	lib := loadRx(t, `fn t { in (x Num) out (r Num) body (return x) }`)
	var flagged bool
	for _, is := range Check(lib) {
		if strings.Contains(is.Msg, "has been removed") {
			flagged = true
		}
	}
	if !flagged {
		t.Fatal("expected `return` to be flagged as removed")
	}
}

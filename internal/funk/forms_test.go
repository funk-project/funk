package funk

import (
	"reflect"
	"strings"
	"testing"
)

// A `latest` group driven by a real stream fires combineLatest-style (fireLatest),
// seeding the defaulted port. Single driving stream ⇒ deterministic order.
func TestLatestFiringWithDefault(t *testing.T) {
	lib := loadRx(t, `
fn combineD { in (latest (x Num) (y Num (default 100))) out (r Num) body (flush (r (add x y))) }
fn driveD   { in (n Num) out (r Stream<Num>) body (combineD (range 1 3)) }`)
	res := Run(lib, "driveD", map[string]interface{}{"n": 3.0}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if got := numList(t, res.Value); !reflect.DeepEqual(got, []float64{101, 102, 103}) {
		t.Fatalf("driveD = %v, want [101 102 103]", got)
	}
}

// Two driving streams under `latest` — order is timing-dependent, so assert the
// invariant (every emit is some x+y in range) rather than an exact sequence.
func TestLatestFiringTwoStreams(t *testing.T) {
	lib := loadRx(t, `
fn combine2 { in (latest (x Num) (y Num)) out (r Num) body (flush (r (add x y))) }
fn drive2   { in () out (r Stream<Num>) body (combine2 (range 1 3) (range 10 12)) }`)
	res := Run(lib, "drive2", map[string]interface{}{}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	got := numList(t, res.Value)
	if len(got) < 3 {
		t.Fatalf("drive2 emitted %d items, want >= 3", len(got))
	}
	for _, v := range got {
		if v < 11 || v > 15 {
			t.Fatalf("drive2 value %v out of range [11,15]", v)
		}
	}
}

// for-each consumes by effect; set stages an output (last write wins across the
// shared frame); do sequences; flush emits the staged value.
func TestForEachSetFlush(t *testing.T) {
	lib := loadRx(t, `fn lastVal { in (n Num) out (r Num)
  body (do (for-each (range 1 n) (i) (set r i)) (flush)) }`)
	res := Run(lib, "lastVal", map[string]interface{}{"n": 4.0}, ExecOpts{})
	if !res.OK || mustNum(t, res.Value) != 4 {
		t.Fatalf("lastVal(4) = %v (%s), want 4 (last item)", res.Value, res.Error)
	}
}

func TestForEachContinueAndBreak(t *testing.T) {
	lib := loadRx(t, `
fn even? { in (a Num) out (r Bool) engine builtin src "num.even" }
fn lt    { in (a Num) (b Num) out (r Bool) engine builtin src "num.lt" }
fn lastOdd { in (n Num) out (r Num)
  body (do (set r 0) (for-each (range 1 n) (i) (if (even? i) (continue) (set r i))) (flush)) }
fn upTo4   { in () out (r Num)
  body (do (set r 0) (for-each (range 1 100) (i) (if (lt i 4) (set r i) (break))) (flush)) }`)
	if res := Run(lib, "lastOdd", map[string]interface{}{"n": 5.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 5 {
		t.Fatalf("lastOdd(5) = %v (%s), want 5 (last odd)", res.Value, res.Error)
	}
	if res := Run(lib, "upTo4", map[string]interface{}{}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 3 {
		t.Fatalf("upTo4 = %v (%s), want 3 (break at i=4)", res.Value, res.Error)
	}
}

func TestWindowFormsCountSlideAndList(t *testing.T) {
	lib := loadRx(t, `
fn winTumble { in (n Num) out (r Stream) body (window (range 1 n) 2) }
fn winSlide  { in (n Num) out (r Stream) body (window (range 1 n) 2 every 1) }
fn lastTwo   { in (n Num) out (r List)   body (flush (r (window (collect (range 1 n)) 2))) }`)

	if res := Run(lib, "winTumble", map[string]interface{}{"n": 4.0}, ExecOpts{}); !res.OK {
		t.Fatal(res.Error)
	} else if l, ok := res.Value.([]interface{}); !ok || len(l) != 2 {
		t.Fatalf("winTumble(4) = %#v, want 2 windows", res.Value)
	}
	if res := Run(lib, "winSlide", map[string]interface{}{"n": 4.0}, ExecOpts{}); !res.OK {
		t.Fatal(res.Error)
	} else if l, ok := res.Value.([]interface{}); !ok || len(l) != 3 {
		t.Fatalf("winSlide(4) = %#v, want 3 sliding windows", res.Value)
	}
	if res := Run(lib, "lastTwo", map[string]interface{}{"n": 5.0}, ExecOpts{}); !res.OK {
		t.Fatal(res.Error)
	} else if got := numList(t, res.Value); !reflect.DeepEqual(got, []float64{4, 5}) {
		t.Fatalf("lastTwo(5) = %v, want [4 5]", got)
	}
}

func TestRepeatAndTick(t *testing.T) {
	lib := loadRx(t, `
fn reps { in (n Num) out (r Stream) body (take (repeat 7) n) }
fn tk   { in (n Num) out (r Stream) body (take (tick 5ms) n) }`)
	if res := Run(lib, "reps", map[string]interface{}{"n": 3.0}, ExecOpts{}); !res.OK {
		t.Fatal(res.Error)
	} else if got := numList(t, res.Value); !reflect.DeepEqual(got, []float64{7, 7, 7}) {
		t.Fatalf("reps(3) = %v, want [7 7 7]", got)
	}
	if res := Run(lib, "tk", map[string]interface{}{"n": 2.0}, ExecOpts{}); !res.OK {
		t.Fatal(res.Error)
	} else if got := numList(t, res.Value); !reflect.DeepEqual(got, []float64{0, 1}) {
		t.Fatalf("tk(2) = %v, want [0 1]", got)
	}
}

// `return` at runtime errors, and the message names the fn's first output port
// (firstOutName), guiding the migration to flush.
func TestReturnRuntimeErrorNamesOutput(t *testing.T) {
	lib := loadRx(t, `fn ret { in (x Num) out (myout Num) body (return x) }`)
	res := Run(lib, "ret", map[string]interface{}{"x": 1.0}, ExecOpts{})
	if res.OK {
		t.Fatal("return should error at runtime")
	}
	if !strings.Contains(res.Error, "myout") {
		t.Fatalf("error %q should name the output port `myout`", res.Error)
	}
}

// RunTests evaluates inline asserts (funk verifying funk); valueEqual decides
// pass/fail. One passing + one failing assertion exercises both outcomes.
func TestRunTestsPassAndFail(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "test/t" {
  version 0.0.1
}
fn addp { in (a Num) (b Num) out (r Num) engine builtin src "num.add"
  test (is (addp 1 2) 3)
  test (is (addp 1 2) 99) }`); err != nil {
		t.Fatal(err)
	}
	results := RunTests(lib)
	if len(results) != 2 {
		t.Fatalf("expected 2 test results, got %d", len(results))
	}
	var pass, fail int
	for _, r := range results {
		if r.Ok {
			pass++
		} else {
			fail++
		}
	}
	if pass != 1 || fail != 1 {
		t.Fatalf("expected 1 pass + 1 fail, got %d/%d", pass, fail)
	}
}

func TestIssueString(t *testing.T) {
	lib := loadRx(t, `fn bad { in (x Num) out (r Num) body (return x) }`)
	issues := Check(lib)
	if len(issues) == 0 {
		t.Fatal("expected at least one issue")
	}
	if s := issues[0].String(); !strings.Contains(s, "bad") && !strings.Contains(s, "removed") {
		t.Fatalf("Issue.String() = %q, want it to mention the fn or the message", s)
	}
}

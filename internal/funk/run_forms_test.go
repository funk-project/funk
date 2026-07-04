package funk

import (
	"reflect"
	"sync"
	"testing"
)

// while carries state across iterations and returns the final state; a break
// inside the step ends the loop early.
func TestWhileLoopAndBreak(t *testing.T) {
	lib := loadRx(t, `
fn lt    { in (a Num) (b Num) out (r Bool) engine builtin src "num.lt" }
fn inc   { in (a Num) out (r Num) engine builtin src "num.inc" }
fn count { in (n Num) out (r Num) body (while (s 0) (lt s n) (inc s)) }
fn cap3  { in ()      out (r Num) body (while (s 0) (lt s 100) (if (lt s 3) (inc s) (break))) }`)
	if res := Run(lib, "count", map[string]interface{}{"n": 5.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 5 {
		t.Fatalf("count(5) = %v (%s), want 5", res.Value, res.Error)
	}
	if res := Run(lib, "cap3", map[string]interface{}{}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 3 {
		t.Fatalf("cap3 = %v (%s), want 3 (break at s=3)", res.Value, res.Error)
	}
}

// retry: success on the first try returns the value; if every attempt errors it
// returns the last error (here division by zero), with an optional backoff wait.
func TestRetrySuccessAndExhaust(t *testing.T) {
	lib := loadRx(t, `fn tryDiv { in (a Num) (b Num) out (r Num) body (retry (flush (r (div a b))) 2 backoff 1ms) }`)
	if res := Run(lib, "tryDiv", map[string]interface{}{"a": 10.0, "b": 2.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 5 {
		t.Fatalf("tryDiv(10,2) = %v (%s), want 5", res.Value, res.Error)
	}
	if res := Run(lib, "tryDiv", map[string]interface{}{"a": 1.0, "b": 0.0}, ExecOpts{}); res.OK {
		t.Fatalf("tryDiv(1,0) should exhaust retries and fail, got %v", res.Value)
	}
}

// on-error: when the body succeeds, the handler is not taken.
func TestOnErrorSuccessPath(t *testing.T) {
	lib := loadRx(t, `fn safe { in (a Num) (b Num) out (r Num) body (on-error (flush (r (div a b))) (e) (flush (r -1))) }`)
	if res := Run(lib, "safe", map[string]interface{}{"a": 8.0, "b": 2.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 4 {
		t.Fatalf("safe(8,2) = %v (%s), want 4 (no error → body value)", res.Value, res.Error)
	}
}

// RunStreaming drains a stream result to the emit callback, and passes a scalar
// result through as a single emit.
func TestRunStreaming(t *testing.T) {
	lib := loadRx(t, `
fn nums  { in (n Num) out (r Stream<Num>) body (double (range 1 n)) }
fn scal  { in (x Num) out (r Num) body (flush (r (double x))) }`)

	var got []interface{}
	if res := RunStreaming(lib, "nums", map[string]interface{}{"n": 3.0}, ExecOpts{}, func(v interface{}) { got = append(got, v) }); !res.OK {
		t.Fatal(res.Error)
	}
	if len(got) != 3 {
		t.Fatalf("RunStreaming(nums,3) emitted %d items, want 3", len(got))
	}

	var single []interface{}
	if res := RunStreaming(lib, "scal", map[string]interface{}{"x": 5.0}, ExecOpts{}, func(v interface{}) { single = append(single, v) }); !res.OK {
		t.Fatal(res.Error)
	}
	if len(single) != 1 || mustNum(t, single[0]) != 10 {
		t.Fatalf("RunStreaming(scal,5) = %v, want [10]", single)
	}
}

// RunLive streams trace events (including per-node "enter" glows) and values,
// then returns the batch report.
func TestRunLive(t *testing.T) {
	lib := loadRx(t, `fn nums { in (n Num) out (r Stream<Num>) body (double (range 1 n)) }`)
	var mu sync.Mutex
	var events []TraceEvent
	var values []interface{}
	res, rep := RunLive(lib, "nums", map[string]interface{}{"n": 3.0}, ExecOpts{},
		func(ev TraceEvent) { mu.Lock(); events = append(events, ev); mu.Unlock() },
		func(v interface{}) { mu.Lock(); values = append(values, v); mu.Unlock() })
	if !res.OK {
		t.Fatal(rep.Error)
	}
	if len(values) != 3 {
		t.Fatalf("RunLive emitted %d values, want 3", len(values))
	}
	if len(events) == 0 || len(rep.Events) == 0 {
		t.Fatalf("RunLive produced no trace events (live=%d, report=%d)", len(events), len(rep.Events))
	}
	// the report keeps its exact shape; a scalar-lift over [1,2,3] doubles to [2,4,6]
	nums := make([]float64, len(values))
	for i, v := range values {
		nums[i] = mustNum(t, v)
	}
	if !reflect.DeepEqual(nums, []float64{2, 4, 6}) {
		t.Fatalf("RunLive values = %v, want [2 4 6]", nums)
	}
}

package funk

import (
	"context"
	"encoding/json"
)

// TestResult is the outcome of one inline `test` assertion.
type TestResult struct {
	Fn   string
	Ok   bool
	Got  interface{}
	Want interface{}
	Err  string
}

// RunTests evaluates every function's inline `test` assertions — funk verifying
// funk (the stdlib is self-testing).
func RunTests(lib *Library) []TestResult {
	var out []TestResult
	for _, f := range lib.Fns {
		for _, tc := range f.Tests {
			ctx, cancel := context.WithCancel(context.Background())
			e := &evalEnv{lib: lib, vars: map[string]interface{}{}, ctx: ctx, cancel: cancel, curFn: f}
			r := TestResult{Fn: f.Name}
			got, err := e.eval(tc.Call)
			if s, ok := got.(Stream); ok {
				got = drain(s)
			}
			want, werr := e.eval(tc.Expect)
			cancel()
			switch {
			case err != nil:
				r.Err = err.Error()
			case werr != nil:
				r.Err = werr.Error()
			default:
				r.Got, r.Want = got, want
				r.Ok = valueEqual(got, want)
			}
			out = append(out, r)
		}
	}
	return out
}

func valueEqual(a, b interface{}) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

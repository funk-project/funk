package funk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testEnv() *evalEnv {
	ctx, cancel := context.WithCancel(context.Background())
	return &evalEnv{lib: NewLibrary(), ctx: ctx, cancel: cancel, vars: map[string]interface{}{}}
}

func TestCountWindow(t *testing.T) {
	e := testEnv()
	in := make(Stream)
	go func() {
		defer close(in)
		for i := 1; i <= 7; i++ {
			in <- float64(i)
		}
	}()
	var got [][]interface{}
	for w := range e.countWindow(in, 3) {
		got = append(got, w.([]interface{}))
	}
	if len(got) != 3 || len(got[0]) != 3 || len(got[2]) != 1 {
		t.Fatalf("countWindow(7,3) = %v, want [3][3][1]", got)
	}
}

func TestSlidingCountWindow(t *testing.T) {
	e := testEnv()
	in := make(Stream)
	go func() {
		defer close(in)
		for i := 1; i <= 5; i++ {
			in <- float64(i)
		}
	}()
	var got [][]interface{}
	for w := range e.slideCountWindow(in, 3, 1) {
		got = append(got, w.([]interface{}))
	}
	// 1..5, size 3, slide 1 → [1,2,3] [2,3,4] [3,4,5]
	if len(got) != 3 || len(got[0]) != 3 || got[0][0] != 1.0 || got[2][2] != 5.0 {
		t.Fatalf("slideCountWindow(1..5,3,1) = %v, want [[1 2 3][2 3 4][3 4 5]]", got)
	}
}

func TestTimeWindow(t *testing.T) {
	e := testEnv()
	items := []interface{}{
		map[string]interface{}{"time": 0.0}, map[string]interface{}{"time": 1.0},
		map[string]interface{}{"time": 5.0}, map[string]interface{}{"time": 6.0},
		map[string]interface{}{"time": 12.0},
	}
	in := make(Stream)
	go func() {
		defer close(in)
		for _, it := range items {
			in <- it
		}
	}()
	var got [][]interface{}
	for w := range e.timeWindow(in, 5.0, "time") {
		got = append(got, w.([]interface{}))
	}
	// 5s event-time buckets: {0,1} {5,6} {12} → three windows sized 2,2,1
	if len(got) != 3 || len(got[0]) != 2 || len(got[2]) != 1 {
		t.Fatalf("timeWindow = %v, want sizes 2,2,1", got)
	}
}

func TestParseFn(t *testing.T) {
	prog, err := Parse(`fn add { doc "add" in (a Num) (b Num) out (r Num) engine builtin src "num.add" }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog) != 1 || prog[0].Head != "fn" || prog[0].Name != "add" {
		t.Fatalf("bad block: %+v", prog)
	}
	f, err := FnFromBlock(prog[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.In) != 2 || f.In[0].Name != "a" || f.In[0].Type != "Num" {
		t.Fatalf("bad ports: %+v", f.In)
	}
	if f.Engine != "builtin" || f.Src != "num.add" {
		t.Fatalf("bad atomic: %+v", f)
	}
}

func TestParsePackageManifest(t *testing.T) {
	prog, err := Parse(`package "funk/std/maths" { version 0.0.1 }`)
	if err != nil {
		t.Fatal(err)
	}
	if prog[0].Head != "package" || prog[0].Name != "funk/std/maths" {
		t.Fatalf("bad manifest: %+v", prog[0])
	}
}

func TestSrcXorBody(t *testing.T) {
	prog, _ := Parse(`fn bad { in (x Num) out (r Num) engine builtin src "id" body (flush (r x)) }`)
	if _, err := FnFromBlock(prog[0], ""); err == nil {
		t.Fatal("expected error for src+body")
	}
}

func TestParseErrorPosition(t *testing.T) {
	// unterminated string — the error points at the opening quote (line 2, col 9)
	_, err := Parse("fn s {\n    src \"oops }\n")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("want *ParseError, got %T (%v)", err, err)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 9 {
		t.Fatalf("unterminated string reported at %s, want 2:9", pe.Pos)
	}
}

func TestCheckIssuePosition(t *testing.T) {
	lib := NewLibrary()
	// the bad call sits on line 4, its head `ghostCall` at column 9
	src := "fn wrapper {\n  in (a Num)\n  out (r Num)\n  body (ghostCall a)\n}\n"
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	issues := Check(lib)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Pos.Line != 4 || issues[0].Pos.Col != 9 {
		t.Fatalf("issue reported at %s, want 4:9", issues[0].Pos)
	}
}

func TestEngineMissingBinary(t *testing.T) {
	res := runCmd("funk-no-such-binary-xyz", nil, nil, 5*1e9)
	if res.OK {
		t.Fatal("expected failure for a missing binary")
	}
	if !strings.Contains(res.Error, "not found in PATH") {
		t.Fatalf("unhelpful error for missing binary: %q", res.Error)
	}
}

func TestCheckArityAndUnknown(t *testing.T) {
	lib := NewLibrary()
	src := `
fn add2 { in (a Num) (b Num) out (r Num) engine builtin src "num.add" }
fn bad {
  in (x Num)
  out (r Num)
  body (do (add2 x) (ghost x))
}`
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	issues := Check(lib)
	if len(issues) != 2 {
		t.Fatalf("want 2 issues, got %d: %v", len(issues), issues)
	}
	var arity, unknown bool
	for _, i := range issues {
		if strings.Contains(i.Msg, "expects 2 input") {
			arity = true
		}
		if strings.Contains(i.Msg, `unknown function "ghost"`) {
			unknown = true
		}
	}
	if !arity || !unknown {
		t.Fatalf("missing expected issues (arity=%v unknown=%v): %v", arity, unknown, issues)
	}
}

func TestBrokerInjectsAndGuards(t *testing.T) {
	// a mock integration that records the Authorization header it receives
	var gotAuth string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mock.Close()
	u, _ := url.Parse(mock.URL)

	cfg := brokerConfig{
		creds: map[string]string{"gh": "s3cr3t-token"},
		hosts: map[string]bool{u.Hostname(): true},
	}
	base, stop, err := startBroker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// (1) naming the capability (NOT the token) reaches the integration with the
	// broker-injected Authorization — the body never holds the secret.
	call := `{"secret":"gh","url":"` + mock.URL + `/x","method":"GET"}`
	resp, err := http.Post(base+"/call", "application/json", strings.NewReader(call))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("broker call status = %d, want 200", resp.StatusCode)
	}
	if gotAuth != "Bearer s3cr3t-token" {
		t.Fatalf("integration saw Authorization %q, want the injected Bearer token", gotAuth)
	}

	// (2) an undeclared host is denied — egress control (docs/06 §7).
	bad := `{"secret":"gh","url":"http://evil.example.com/x"}`
	r2, err := http.Post(base+"/call", "application/json", strings.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusForbidden {
		t.Fatalf("undeclared host status = %d, want 403", r2.StatusCode)
	}
}

func TestSecretWithNetWarning(t *testing.T) {
	lib := NewLibrary()
	src := `
fn risky {
  in  (t Str)
  out (r Str)
  engine python
  needs { secret token }
  effects { net evil.example.com }
  src "return t"
}
fn safe {
  in  (t Str)
  out (r Str)
  engine python
  needs { secret token }
  src "return t"
}`
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	var warns []Issue
	for _, i := range Check(lib) {
		if i.Warn {
			warns = append(warns, i)
		}
	}
	if len(warns) != 1 || warns[0].Fn != "risky" {
		t.Fatalf("want exactly one secret+net warning on 'risky', got %+v", warns)
	}
}

func TestHasNetEffect(t *testing.T) {
	withNet := &Fn{Effects: []Effect{{Kind: "net", Args: []string{"api.github.com"}}}}
	noNet := &Fn{Effects: []Effect{{Kind: "fs", Args: []string{"read", "/tmp"}}}}
	if !hasNetEffect(withNet) {
		t.Fatal("expected net effect to be detected")
	}
	if hasNetEffect(noNet) || hasNetEffect(&Fn{}) {
		t.Fatal("expected no net effect for fs-only / empty")
	}
}

func TestTraceEventCarriesNodeID(t *testing.T) {
	lib := NewLibrary()
	src := "fn g { in (a Num) (b Num) out (r Bool) engine builtin src \"num.gt\" }\n" +
		"fn f { in (x Num) out (r Bool) body (g x 5) }"
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	ff, _ := lib.Lookup("f")
	// the call-site node id is the (g …) form's AST position — the same id that
	// `funk graph --json` emits for that node.
	want := ff.Body.(Form).Pos.String()
	_, rep := RunWithReport(lib, "f", map[string]interface{}{"x": 9.0}, ExecOpts{})
	var got string
	for _, ev := range rep.Events {
		if ev.Kind == "call" && ev.Fn == "g" {
			got = ev.Node
		}
	}
	if got == "" || got != want {
		t.Fatalf("call event node id = %q, want %q (the (g …) call-site Pos)", got, want)
	}
}

func TestOnErrorRecovers(t *testing.T) {
	lib := NewLibrary()
	src := `
fn d { in (a Num) (b Num) out (r Num) engine builtin src "num.div" }
fn safe {
  in (a Num) (b Num)
  out (r Num)
  body (on-error (flush (r (d a b))) (e) (flush (r 0)))
}`
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	if res := Run(lib, "safe", map[string]interface{}{"a": 10.0, "b": 0.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 0 {
		t.Fatalf("safe(10,0) = %v (%s), want 0 (recovered)", res.Value, res.Error)
	}
	if res := Run(lib, "safe", map[string]interface{}{"a": 10.0, "b": 2.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 5 {
		t.Fatalf("safe(10,2) = %v (%s), want 5 (no error, no recovery)", res.Value, res.Error)
	}
}

func TestRetryAttempts(t *testing.T) {
	lib := NewLibrary()
	src := `
fn boom { in (a Num) (b Num) out (r Num) engine builtin src "num.div" }
fn tryBoom {
  in (x Num)
  out (r Num)
  body (retry (boom x 0) 3)
}`
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	res, rep := RunWithReport(lib, "tryBoom", map[string]interface{}{"x": 10.0}, ExecOpts{})
	if res.OK {
		t.Fatal("expected tryBoom to fail after exhausting retries")
	}
	retries := 0
	for _, ev := range rep.Events {
		if ev.Kind == "retry" {
			retries++
		}
	}
	if retries != 3 {
		t.Fatalf("want 3 retry attempts recorded, got %d: %+v", retries, rep.Events)
	}
}

func TestTraceRedactsSecret(t *testing.T) {
	lib := NewLibrary()
	// `flow` touches the secret in an intermediate step (whose call Value the trace
	// would capture) but returns a non-secret. The trace must mask the secret; the
	// returned value is the caller's result and is left intact.
	src := `
fn echo { in (a Str) out (r Str) engine builtin src "id" }
fn flow {
  in (x Num)
  out (r Str)
  needs { secret token }
  body (do (echo needs.secret.token) (flush (r "done")))
}`
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	opts := ExecOpts{Bindings: map[string]string{"secret.token": "s3cr3t-value"}}
	res, rep := RunWithReport(lib, "flow", map[string]interface{}{"x": 1.0}, opts)
	if !res.OK {
		t.Fatalf("run failed: %s", res.Error)
	}
	if res.Value != "done" {
		t.Fatalf("output = %v, want \"done\"", res.Value)
	}
	blob, _ := json.Marshal(rep)
	if strings.Contains(string(blob), "s3cr3t-value") {
		t.Fatalf("secret leaked into trace: %s", blob)
	}
	if !strings.Contains(string(blob), "***") {
		t.Fatalf("expected redaction marker in trace: %s", blob)
	}
}

func TestBuiltins(t *testing.T) {
	cases := []struct {
		src  string
		in   map[string]interface{}
		want float64
	}{
		{"num.add", map[string]interface{}{"a": 40.0, "b": 2.0}, 42},
		{"num.sub", map[string]interface{}{"a": 10.0, "b": 3.0}, 7},
		{"num.mul", map[string]interface{}{"a": 6.0, "b": 7.0}, 42},
		{"num.sqrt", map[string]interface{}{"a": 25.0}, 5},
		{"num.pow", map[string]interface{}{"a": 2.0, "b": 10.0}, 1024},
	}
	for _, c := range cases {
		res := Exec(&Fn{Engine: "builtin", Src: c.src}, c.in, ExecOpts{})
		if !res.OK {
			t.Fatalf("%s: %s", c.src, res.Error)
		}
		got, err := toNum(res.Value)
		if err != nil || got != c.want {
			t.Fatalf("%s = %v, want %v", c.src, res.Value, c.want)
		}
	}
}

func TestBuiltinDivZero(t *testing.T) {
	res := Exec(&Fn{Engine: "builtin", Src: "num.div"}, map[string]interface{}{"a": 1.0, "b": 0.0}, ExecOpts{})
	if res.OK {
		t.Fatal("expected division by zero error")
	}
}

// loadStd loads the real std library from the repo.
func loadStd(t *testing.T) *Library {
	t.Helper()
	lib := NewLibrary()
	if err := lib.LoadDir("../../std"); err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestCompositeBumpGatesCondition(t *testing.T) {
	lib := loadStd(t)
	// bump(9) = 109 (x>5 → add), bump(3) = 3 (else → pass through).
	// If the add ran ungated, bump(3) would be 103. It must be 3.
	if res := Run(lib, "bump", map[string]interface{}{"x": 9.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 109 {
		t.Fatalf("bump(9) = %v (%s), want 109", res.Value, res.Error)
	}
	if res := Run(lib, "bump", map[string]interface{}{"x": 3.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 3 {
		t.Fatalf("bump(3) = %v (%s), want 3 — the condition failed to gate the add", res.Value, res.Error)
	}
}

func TestCompositeAnalyze(t *testing.T) {
	lib := loadStd(t)
	if res := Run(lib, "analyze", map[string]interface{}{"xs": []interface{}{1.0, 2.0, 3.0, 4.0}}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 2.5 {
		t.Fatalf("analyze[1,2,3,4] = %v (%s), want 2.5", res.Value, res.Error)
	}
	if res := Run(lib, "analyze", map[string]interface{}{"xs": []interface{}{}}, ExecOpts{}); !res.OK || res.Value != "no data" {
		t.Fatalf("analyze[] = %v (%s), want \"no data\"", res.Value, res.Error)
	}
}

func TestStreamMapInfiniteTake(t *testing.T) {
	lib := loadStd(t)
	// evens = (take (map (nats) double) n): maps an INFINITE source, take must
	// bound it and cancel upstream — if cancellation failed this would hang.
	res := Run(lib, "evens", map[string]interface{}{"n": 5.0}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	list, ok := res.Value.([]interface{})
	if !ok || len(list) != 5 {
		t.Fatalf("evens(5) = %v", res.Value)
	}
	for i, want := range []float64{0, 2, 4, 6, 8} {
		if mustNum(t, list[i]) != want {
			t.Fatalf("evens[%d] = %v, want %v", i, list[i], want)
		}
	}
}

func TestScanAndMerge(t *testing.T) {
	lib := loadStd(t)
	res := Run(lib, "runningSum", map[string]interface{}{"n": 5.0}, ExecOpts{})
	list, ok := res.Value.([]interface{})
	if !ok || len(list) != 5 || mustNum(t, list[4]) != 15 {
		t.Fatalf("runningSum(5) = %v, want [1 3 6 10 15]", res.Value)
	}
	if res := Run(lib, "mergedCount", map[string]interface{}{"n": 4.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 8 {
		t.Fatalf("mergedCount(4) = %v, want 8", res.Value)
	}
}

func TestStreamFilter(t *testing.T) {
	lib := loadStd(t)
	res := Run(lib, "firstEvens", map[string]interface{}{"n": 4.0}, ExecOpts{})
	if !res.OK {
		t.Fatal(res.Error)
	}
	list := res.Value.([]interface{})
	if len(list) != 4 || mustNum(t, list[3]) != 6 {
		t.Fatalf("firstEvens(4) = %v, want [0 2 4 6]", res.Value)
	}
}

func TestRunStreamingLive(t *testing.T) {
	lib := loadStd(t)
	var got []float64
	res := RunStreaming(lib, "squares", map[string]interface{}{"n": 4.0}, ExecOpts{}, func(v interface{}) {
		got = append(got, mustNum(t, v))
	})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if len(got) != 4 || got[3] != 16 {
		t.Fatalf("squares(4) streamed %v, want [1 4 9 16]", got)
	}
}

func TestRunReport(t *testing.T) {
	lib := loadStd(t)
	// bump(3): condition false → else branch → the add must NOT appear in the trace.
	_, rep := RunWithReport(lib, "bump", map[string]interface{}{"x": 3.0}, ExecOpts{})
	if !rep.OK {
		t.Fatalf("report not ok: %s", rep.Error)
	}
	calledAdd, branch := false, ""
	for _, ev := range rep.Events {
		if ev.Kind == "call" && ev.Fn == "add" {
			calledAdd = true
		}
		if ev.Kind == "branch" {
			branch = ev.Detail
		}
	}
	if calledAdd {
		t.Fatal("bump(3) trace shows add called — the condition failed to gate it")
	}
	if branch != "else" {
		t.Fatalf("branch = %q, want else", branch)
	}
}

func TestRunLiveStreamsEvents(t *testing.T) {
	lib := loadStd(t)
	var live []TraceEvent
	var values []interface{}
	res, rep := RunLive(lib, "bump", map[string]interface{}{"x": 3.0}, ExecOpts{},
		func(ev TraceEvent) { live = append(live, ev) },
		func(v interface{}) { values = append(values, v) })
	if !res.OK {
		t.Fatalf("bump live run failed: %s", rep.Error)
	}
	// The live sink must see an "enter" (glow) signal — it fires before a node runs.
	sawEnter := false
	for _, ev := range live {
		if ev.Kind == "enter" {
			sawEnter = true
		}
	}
	if !sawEnter {
		t.Fatalf("want a live 'enter' event; got %+v", live)
	}
	// The batch RunReport must NOT record 'enter' events — its shape is unchanged.
	for _, ev := range rep.Events {
		if ev.Kind == "enter" {
			t.Fatalf("RunReport should not record 'enter' events: %+v", rep.Events)
		}
	}
	// bump(3) → 3, delivered live via onValue.
	if len(values) != 1 || mustNum(t, values[0]) != 3 {
		t.Fatalf("want live value [3], got %v", values)
	}
}

func TestIntrospect(t *testing.T) {
	lib := loadStd(t)
	in, ok := Introspect(lib, "analyze")
	if !ok || in.Kind != "composite" {
		t.Fatalf("analyze introspection: %+v", in)
	}
	got := map[string]bool{}
	for _, c := range in.Calls {
		got[c] = true
	}
	if !got["isEmpty?"] || !got["mean"] {
		t.Fatalf("analyze should call isEmpty? and mean, got %v", in.Calls)
	}
	if len(in.Unresolved) != 0 {
		t.Fatalf("analyze has unresolved calls: %v", in.Unresolved)
	}
	add, _ := Introspect(lib, "add")
	if add.Kind != "atomic" || add.Engine != "builtin" {
		t.Fatalf("add introspection: %+v", add)
	}
}

func TestNeedsParseAndAggregate(t *testing.T) {
	lib := loadStd(t)
	ci, ok := Introspect(lib, "createIssue")
	if !ok || len(ci.Needs) != 3 {
		t.Fatalf("createIssue needs = %v", ci.Needs)
	}
	if len(ci.Effects) != 1 || ci.Effects[0].Kind != "net" {
		t.Fatalf("createIssue effects = %v", ci.Effects)
	}
	// triage calls createIssue; its needs/effects must bubble up (docs/05).
	tr, _ := Introspect(lib, "triage")
	kinds := map[string]bool{}
	for _, n := range tr.Needs {
		kinds[n.Kind] = true
	}
	if !kinds["github"] || !kinds["secret"] || !kinds["config"] {
		t.Fatalf("triage needs did not aggregate up: %v", tr.Needs)
	}
	if len(tr.Effects) != 1 {
		t.Fatalf("triage effects did not aggregate: %v", tr.Effects)
	}
}

func TestFormatIdempotent(t *testing.T) {
	src := "fn analyze {\n" +
		"  doc \"mean\"\n" +
		"  in (xs Stream<Num>)\n" +
		"  out (r Num)\n" +
		"  needs {\n    config site Str\n    secret token\n  }\n" +
		"  body (if (isEmpty? xs) (exit \"no data\") (flush (r (mean (window xs 100)))))\n" +
		"}\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	f1 := Format(prog)
	prog2, err := Parse(f1)
	if err != nil {
		t.Fatalf("reparse of formatted source failed: %v\n%s", err, f1)
	}
	if f2 := Format(prog2); f1 != f2 {
		t.Fatalf("format not idempotent:\n--- f1 ---\n%s\n--- f2 ---\n%s", f1, f2)
	}
}

func TestWhileLoop(t *testing.T) {
	lib := loadStd(t)
	if res := Run(lib, "powTwoLE", map[string]interface{}{"n": 100.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 64 {
		t.Fatalf("powTwoLE(100) = %v (%s), want 64", res.Value, res.Error)
	}
	if res := Run(lib, "countUp", map[string]interface{}{"n": 7.0}, ExecOpts{}); !res.OK || mustNum(t, res.Value) != 7 {
		t.Fatalf("countUp(7) = %v (%s), want 7", res.Value, res.Error)
	}
}

func TestResourceInjection(t *testing.T) {
	lib := loadStd(t)
	// siteGreeting reads needs.config.site in a funk body; bind resolves it.
	opts := ExecOpts{Bindings: map[string]string{"config.site": "funk"}}
	res := Run(lib, "siteGreeting", nil, opts)
	if !res.OK || res.Value != "hello from funk" {
		t.Fatalf("siteGreeting = %v (%s), want \"hello from funk\"", res.Value, res.Error)
	}
	// unbound resource resolves to empty, not an error.
	res = Run(lib, "siteGreeting", nil, ExecOpts{})
	if !res.OK {
		t.Fatalf("unbound siteGreeting errored: %s", res.Error)
	}
}

func TestParseNeedsBlock(t *testing.T) {
	prog, err := Parse("fn f {\n in (x Str)\n needs {\n  github gh\n  config repo Str\n }\n engine python\n src \"return x\"\n}")
	if err != nil {
		t.Fatal(err)
	}
	f, err := FnFromBlock(prog[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Needs) != 2 || f.Needs[0].Kind != "github" || f.Needs[0].Alias != "gh" {
		t.Fatalf("needs = %v", f.Needs)
	}
	if f.Needs[1].Schema != "Str" {
		t.Fatalf("config schema = %q", f.Needs[1].Schema)
	}
}

func mustNum(t *testing.T, v interface{}) float64 {
	t.Helper()
	f, err := toNum(v)
	if err != nil {
		t.Fatalf("not a number: %v", v)
	}
	return f
}

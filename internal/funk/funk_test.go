package funk

import "testing"

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
	prog, _ := Parse(`fn bad { in (x Num) out (r Num) engine builtin src "id" body (return x) }`)
	if _, err := FnFromBlock(prog[0], ""); err == nil {
		t.Fatal("expected error for src+body")
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
		{"list.mean", map[string]interface{}{"a": []interface{}{2.0, 4.0, 6.0}}, 4},
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

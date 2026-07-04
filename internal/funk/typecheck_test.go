package funk

import "testing"

// tcPrims are the atomic builtins the type-check fixtures wire together.
const tcPrims = `
fn add    { in (a Num) (b Num) out (r Num)  engine builtin src "num.add" }
fn div    { in (a Num) (b Num) out (r Num)  engine builtin src "num.div" }
fn mod    { in (a Num) (b Num) out (r Num)  engine builtin src "num.mod" }
fn inc    { in (a Num)         out (r Num)  engine builtin src "num.inc" }
fn double { in (a Num)         out (r Num)  engine builtin src "num.double" }
fn lt     { in (a Num) (b Num) out (r Bool) engine builtin src "num.lt" }
`

// A body exercising every composite form type-checks clean: each/yield, scan,
// fold, window, let (single + destructure), for-each/set, while, on-error, retry,
// tick, and a plain call. This drives checkForm + inferForm across their branches.
func TestCheckCleanAcrossForms(t *testing.T) {
	lib := NewLibrary()
	src := tcPrims + `
fn dm  { in (a Num) (b Num) out (q Num) (r Num) body (flush (q (div a b)) (r (mod a b))) }
fn eachY  { in (xs Stream<Num>) out (r Stream<Num>) body (each xs (i) (yield (double i))) }
fn scanF  { in (n Num) out (r Stream<Num>) body (scan (range 1 n) add 0) }
fn foldF  { in (xs List) out (r Num) body (flush (r (fold xs add 0))) }
fn winF   { in (xs Stream<Num>) out (r Stream) body (window xs 3) }
fn letS   { in (n Num) out (r Num) body (let (x (double n)) (flush (r x))) }
fn letD   { in (a Num) (b Num) out (r Num) body (let (q r (dm a b)) (flush (r (add q r)))) }
fn forE   { in (xs List) out (r Num) body (do (for-each xs (i) (set r i)) (flush)) }
fn whileF { in (n Num) out (r Num) body (while (s 0) (lt s n) (inc s)) }
fn onErr  { in (a Num) (b Num) out (r Num) body (on-error (flush (r (div a b))) (e) (flush (r 0))) }
fn retryF { in (a Num) (b Num) out (r Num) body (retry (flush (r (div a b))) 2 backoff 1ms) }
fn tickF  { in (n Num) out (r Stream) body (take (tick 10ms) n) }
`
	if err := lib.LoadString(src); err != nil {
		t.Fatal(err)
	}
	if issues := Check(lib); len(issues) != 0 {
		t.Fatalf("expected a clean check across forms, got %d issue(s): %v", len(issues), issues)
	}
}

func TestCheckArityMismatch(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(tcPrims + `fn g { in (x Num) out (r Num) body (flush (r (add x))) }`); err != nil {
		t.Fatal(err)
	}
	if !hasIssue(Check(lib), "expects 2 input(s), got 1") {
		t.Fatalf("expected arity issue, got %v", Check(lib))
	}
}

func TestCheckUnknownFunctionAndIdentifier(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(tcPrims + `
fn u1 { in (x Num) out (r Num) body (flush (r (nope x))) }
fn u2 { in (x Num) out (r Num) body (flush (r zzz)) }`); err != nil {
		t.Fatal(err)
	}
	issues := Check(lib)
	if !hasIssue(issues, `unknown function "nope"`) {
		t.Fatalf("expected unknown function, got %v", issues)
	}
	if !hasIssue(issues, `unknown identifier "zzz"`) {
		t.Fatalf("expected unknown identifier, got %v", issues)
	}
}

func TestCheckScanUnknownFn(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(tcPrims + `fn s { in (n Num) out (r Stream<Num>) body (scan (range 1 n) nope 0) }`); err != nil {
		t.Fatal(err)
	}
	if !hasIssue(Check(lib), `scan references unknown function "nope"`) {
		t.Fatalf("expected scan unknown-fn issue, got %v", Check(lib))
	}
}

func TestCheckTypeMismatch(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(tcPrims + `fn m { in (s Str) out (r Num) body (flush (r (double s))) }`); err != nil {
		t.Fatal(err)
	}
	if !hasIssue(Check(lib), "type mismatch") {
		t.Fatalf("expected a connection type mismatch, got %v", Check(lib))
	}
}

// Issue.String locates the issue at file:line:col when known, degrading to
// line:col, then to just the fn name.
func TestIssueStringAllForms(t *testing.T) {
	full := Issue{Fn: "f", File: "x.funk", Pos: Pos{Line: 3, Col: 2}, Msg: "boom"}
	if s := full.String(); s != "x.funk:3:2: f: boom" {
		t.Fatalf("full String() = %q", s)
	}
	posOnly := Issue{Fn: "f", Pos: Pos{Line: 3, Col: 2}, Msg: "boom"}
	if s := posOnly.String(); s != "3:2: f: boom" {
		t.Fatalf("pos-only String() = %q", s)
	}
	bare := Issue{Fn: "f", Msg: "boom"}
	if s := bare.String(); s != "f: boom" {
		t.Fatalf("bare String() = %q", s)
	}
}

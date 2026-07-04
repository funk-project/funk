package funk

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIsIdent(t *testing.T) {
	cases := map[string]bool{
		"abc": true, "a_1": true, "A9": true, "_x": true,
		"": false, "1ab": false, "a-b": false, "a.b": false, "a b": false,
	}
	for in, want := range cases {
		if got := isIdent(in); got != want {
			t.Errorf("isIdent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMapStrIndentOrDefault(t *testing.T) {
	got := mapStr([]string{"a", "b"}, func(s string) string { return s + "!" })
	if len(got) != 2 || got[0] != "a!" || got[1] != "b!" {
		t.Fatalf("mapStr = %v", got)
	}
	if indentLines("x\ny", "  ") != "  x\n  y" {
		t.Fatalf("indentLines = %q", indentLines("x\ny", "  "))
	}
	if orDefault("", "d") != "d" || orDefault("v", "d") != "v" {
		t.Fatal("orDefault wrong")
	}
}

func TestToNum(t *testing.T) {
	ok := []struct {
		in   interface{}
		want float64
	}{
		{3.0, 3}, {float32(2.5), 2.5}, {int(4), 4}, {int64(5), 5}, {int32(6), 6},
		{"7.5", 7.5}, {"  8 ", 8}, {true, 1}, {false, 0}, {json.Number("9"), 9},
	}
	for _, c := range ok {
		got, err := toNum(c.in)
		if err != nil || got != c.want {
			t.Errorf("toNum(%#v) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []interface{}{"nope", []int{1}, struct{}{}} {
		if _, err := toNum(bad); err == nil {
			t.Errorf("toNum(%#v) should error", bad)
		}
	}
}

func TestTruthyAndNumFmt(t *testing.T) {
	truthyCases := map[bool][]interface{}{
		true:  {true, 1.0, int(3), "x", []interface{}{1}, struct{}{}},
		false: {false, 0.0, "", "false", nil, []interface{}{}},
	}
	for want, vals := range truthyCases {
		for _, v := range vals {
			if truthy(v) != want {
				t.Errorf("truthy(%#v) = %v, want %v", v, truthy(v), want)
			}
		}
	}
	if numFmt(3.0) != int64(3) {
		t.Errorf("numFmt(3.0) = %#v, want int64(3)", numFmt(3.0))
	}
	if numFmt(3.5) != 3.5 {
		t.Errorf("numFmt(3.5) = %#v, want 3.5", numFmt(3.5))
	}
}

func TestASTHelpers(t *testing.T) {
	if _, ok := AsAtom(Atom{Value: "x"}); !ok {
		t.Fatal("AsAtom should match an Atom")
	}
	if _, ok := AsAtom(Form{Head: "f"}); ok {
		t.Fatal("AsAtom should not match a Form")
	}
	if _, ok := AsForm(Form{Head: "f"}); !ok {
		t.Fatal("AsForm should match a Form")
	}
	if _, ok := AsForm(Atom{Value: "x"}); ok {
		t.Fatal("AsForm should not match an Atom")
	}
	// node() marker methods (kept for the interface)
	Atom{}.node()
	Form{}.node()

	b := Block{Fields: []Field{{Key: "engine", Values: []Node{Atom{Kind: "id", Value: "python"}}}}}
	if b.FieldStr("engine") != "python" {
		t.Fatalf("FieldStr = %q", b.FieldStr("engine"))
	}
	if b.FieldStr("missing") != "" {
		t.Fatal("FieldStr(missing) should be empty")
	}
	if _, ok := b.Field("engine"); !ok {
		t.Fatal("Field(engine) should exist")
	}
}

func TestNodeTypeString(t *testing.T) {
	if got := nodeTypeString(Atom{Kind: "id", Value: "Num"}); got != "Num" {
		t.Fatalf("scalar type = %q", got)
	}
	streamNum := Form{Head: "Stream", Args: []Node{Atom{Kind: "id", Value: "Num"}}}
	if got := nodeTypeString(streamNum); got != "Stream<Num>" {
		t.Fatalf("stream type = %q", got)
	}
}

func TestStrAndAsList(t *testing.T) {
	if str("hi") != "hi" || str(nil) != "" || str(int64(5)) != "5" {
		t.Fatal("str wrong")
	}
	if l := asList([]interface{}{1, 2}); len(l) != 2 {
		t.Fatalf("asList(list) = %v", l)
	}
	if asList(nil) != nil {
		t.Fatal("asList(nil) should be nil")
	}
	if l := asList(7); len(l) != 1 || l[0] != 7 {
		t.Fatalf("asList(scalar) = %v", l)
	}
}

func TestParseErrorAndKindName(t *testing.T) {
	withPos := &ParseError{Msg: "boom", Pos: Pos{Line: 2, Col: 3}}
	if withPos.Error() != "2:3: boom" {
		t.Fatalf("ParseError = %q", withPos.Error())
	}
	noPos := &ParseError{Msg: "boom"}
	if noPos.Error() != "boom" {
		t.Fatalf("ParseError (no pos) = %q", noPos.Error())
	}
	if kindName(tLParen) != "'('" || kindName(tStr) != "a string" || kindName(tNL) != "newline" {
		t.Fatal("kindName wrong")
	}
}

func TestEngineHelpers(t *testing.T) {
	if inputsJSON(nil) != "{}" {
		t.Fatalf("inputsJSON(nil) = %q", inputsJSON(nil))
	}
	if !strings.Contains(inputsJSON(map[string]interface{}{"a": 1.0}), `"a"`) {
		t.Fatal("inputsJSON should include the key")
	}
	if needsJSON(&Fn{}, ExecOpts{}) != "{}" {
		t.Fatalf("needsJSON(empty) = %q", needsJSON(&Fn{}, ExecOpts{}))
	}
	if parseOut("") != "" || parseOut("hello") != "hello" {
		t.Fatal("parseOut string wrong")
	}
	if v, ok := parseOut("42").(float64); !ok || v != 42 {
		t.Fatalf("parseOut(42) = %#v", parseOut("42"))
	}
	if _, ok := parseOut("[1,2]").([]interface{}); !ok {
		t.Fatalf("parseOut(list) = %#v", parseOut("[1,2]"))
	}
	if (ExecOpts{}).timeout(5*time.Second) != 5*time.Second {
		t.Fatal("default timeout wrong")
	}
	if (ExecOpts{Timeout: 2 * time.Second}).timeout(5*time.Second) != 2*time.Second {
		t.Fatal("explicit timeout wrong")
	}
	if !strings.Contains(strings.Join(baseEnv(`{"a":1}`), "\n"), "FUNK_INPUTS=") {
		t.Fatal("baseEnv should carry FUNK_INPUTS")
	}
}

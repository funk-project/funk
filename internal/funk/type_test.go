package funk

import "testing"

// Parameterized types are written homoiconically as prefix forms and render in the
// familiar angle-bracket form.
func TestParameterizedTypeRendering(t *testing.T) {
	num := Atom{Kind: "id", Value: "Num"}
	cases := []struct {
		node Node
		want string
	}{
		{Atom{Kind: "id", Value: "Num"}, "Num"},
		{Form{Head: "List", Args: []Node{num}}, "List<Num>"},
		{Form{Head: "List", Args: []Node{Atom{Kind: "id", Value: "Num|Str"}}}, "List<Num|Str>"},
		{Form{Head: "Stream", Args: []Node{Form{Head: "List", Args: []Node{num}}}}, "Stream<List<Num>>"},
		{Form{Head: "Num|Str"}, "Num|Str"}, // a no-arg form renders as just its head
	}
	for _, c := range cases {
		if got := nodeTypeString(c.node); got != c.want {
			t.Errorf("nodeTypeString = %q, want %q", got, c.want)
		}
	}
}

// A `(List Num)` port type parses and is stored as "List<Num>".
func TestParameterizedPortParses(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "t/ty" {
  version 0.0.1
}
fn f { in (xs (List Num)) out (r Num) engine builtin src "list.first" }`); err != nil {
		t.Fatal(err)
	}
	f, _ := lib.Lookup("f")
	if f.In[0].Type != "List<Num>" {
		t.Fatalf("port type = %q, want List<Num>", f.In[0].Type)
	}
}

// connMismatch is conservative: unknown/Any is compatible, the reactive lift is
// honoured, and only concrete, differing types are flagged.
func TestConnMismatch(t *testing.T) {
	ok := [][2]string{
		{"Stream<Num>", "Num"}, // reactive lift, element matches the scalar port
		{"List<Num>", "List"},  // untyped consumer accepts anything
		{"List", "List<Num>"},  // untyped producer flows into a typed port
		{"Any", "Num"},         // Any is compatible
		{"Num", "Any"},         //
		{"", "Num"},            // unknown producer
		{"Num", "Num"},         // same scalar
		{"List<Num>", "List<Num>"},
	}
	for _, c := range ok {
		if _, _, bad := connMismatch(c[0], c[1]); bad {
			t.Errorf("connMismatch(%q,%q) flagged, want compatible", c[0], c[1])
		}
	}
	bad := [][2]string{
		{"Num", "Str"},             // scalar mismatch
		{"Stream<Num>", "Str"},     // lift whose element differs from the scalar port
		{"List<Num>", "List<Str>"}, // same collection, differing elements
	}
	for _, c := range bad {
		if _, _, isBad := connMismatch(c[0], c[1]); !isBad {
			t.Errorf("connMismatch(%q,%q) compatible, want a mismatch", c[0], c[1])
		}
	}
}

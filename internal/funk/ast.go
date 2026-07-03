// Package funk implements the funk notation: parser, type checker, and engine.
//
// The AST mirrors the reference design (docs/04-the-protocol.md): a program is a
// sequence of brace-blocks (`fn` / `type` / `package`); expressions are prefix
// forms `(head arg …)`; the graph is *derived* from the code, never drawn.
package funk

// Node is an expression node: either an Atom or a Form.
type Node interface{ node() }

// Atom is an identifier, string, or number literal.
type Atom struct {
	Kind  string // "id" | "str" | "num"
	Value string
}

func (Atom) node() {}

// Form is a prefix form: (Head Args…). An empty form (Head == "") is `()`.
type Form struct {
	Head string
	Args []Node
}

func (Form) node() {}

// Field is a `key value…` line inside a block. A field may instead hold a nested
// brace block (`needs { … }` / `effects { … }`), captured line-by-line in Sub.
type Field struct {
	Key    string
	Values []Node
	Sub    []Field `json:",omitempty"`
}

// Block is a top-level definition: `head Name { field… }`.
type Block struct {
	Head   string // "fn" | "type" | "package"
	Name   string
	Fields []Field
}

// Program is a parsed .funk source: a list of blocks.
type Program []Block

// Field returns the first field with the given key.
func (b Block) Field(key string) (Field, bool) {
	for _, f := range b.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// FieldStr returns a single string/atom field value (e.g. `engine python`).
func (b Block) FieldStr(key string) string {
	f, ok := b.Field(key)
	if !ok || len(f.Values) == 0 {
		return ""
	}
	if a, ok := f.Values[0].(Atom); ok {
		return a.Value
	}
	return ""
}

// AsForm returns n as a Form if it is one.
func AsForm(n Node) (Form, bool) {
	f, ok := n.(Form)
	return f, ok
}

// AsAtom returns n as an Atom if it is one.
func AsAtom(n Node) (Atom, bool) {
	a, ok := n.(Atom)
	return a, ok
}

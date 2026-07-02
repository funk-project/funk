package funk

import (
	"fmt"
	"regexp"
	"strings"
)

// ParseError is a syntax error with position context.
type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return "parse error: " + e.Msg }

type tokKind int

const (
	tLParen tokKind = iota
	tRParen
	tLBrace
	tRBrace
	tNL
	tStr
	tAtom
)

type tok struct {
	k tokKind
	v string
}

var numRe = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

const delims = " \t\r\n(){};\""

func tokenize(src string) []tok {
	var toks []tok
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			toks = append(toks, tok{tNL, ""})
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == ';':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '(':
			toks = append(toks, tok{tLParen, ""})
			i++
		case c == ')':
			toks = append(toks, tok{tRParen, ""})
			i++
		case c == '{':
			toks = append(toks, tok{tLBrace, ""})
			i++
		case c == '}':
			toks = append(toks, tok{tRBrace, ""})
			i++
		case c == '"':
			i++
			var sb strings.Builder
			for i < n && src[i] != '"' {
				if src[i] == '\\' && i+1 < n {
					switch src[i+1] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					default:
						sb.WriteByte(src[i+1])
					}
					i += 2
				} else {
					sb.WriteByte(src[i])
					i++
				}
			}
			i++ // closing quote
			toks = append(toks, tok{tStr, sb.String()})
		default:
			start := i
			for i < n && !strings.ContainsRune(delims, rune(src[i])) {
				i++
			}
			toks = append(toks, tok{tAtom, src[start:i]})
		}
	}
	return toks
}

type parser struct {
	toks []tok
	p    int
}

func (ps *parser) peek() (tok, bool) {
	if ps.p < len(ps.toks) {
		return ps.toks[ps.p], true
	}
	return tok{}, false
}

func (ps *parser) next() (tok, error) {
	if ps.p >= len(ps.toks) {
		return tok{}, &ParseError{"unexpected end of input"}
	}
	t := ps.toks[ps.p]
	ps.p++
	return t, nil
}

func (ps *parser) skipNL() {
	for {
		t, ok := ps.peek()
		if !ok || t.k != tNL {
			return
		}
		ps.p++
	}
}

func (ps *parser) expect(k tokKind, name string) error {
	t, err := ps.next()
	if err != nil {
		return err
	}
	if t.k != k {
		return &ParseError{fmt.Sprintf("expected %q", name)}
	}
	return nil
}

func atomNode(v string, str bool) Atom {
	if str {
		return Atom{Kind: "str", Value: v}
	}
	if numRe.MatchString(v) {
		return Atom{Kind: "num", Value: v}
	}
	return Atom{Kind: "id", Value: v}
}

func (ps *parser) parseValue() (Node, error) {
	t, ok := ps.peek()
	if !ok {
		return nil, &ParseError{"expected a value"}
	}
	switch t.k {
	case tLParen:
		return ps.parseForm()
	case tStr:
		ps.p++
		return atomNode(t.v, true), nil
	case tAtom:
		ps.p++
		return atomNode(t.v, false), nil
	default:
		return nil, &ParseError{"unexpected token where a value was expected"}
	}
}

func (ps *parser) parseForm() (Node, error) {
	if err := ps.expect(tLParen, "("); err != nil {
		return nil, err
	}
	ps.skipNL()
	if t, ok := ps.peek(); ok && t.k == tRParen {
		ps.p++
		return Form{Head: "", Args: nil}, nil // empty form, e.g. `in ()`
	}
	head, err := ps.next()
	if err != nil {
		return nil, err
	}
	if head.k != tAtom {
		return nil, &ParseError{"a form must start with an operator name"}
	}
	var args []Node
	for {
		ps.skipNL()
		t, ok := ps.peek()
		if !ok {
			return nil, &ParseError{"unterminated form (missing ')')"}
		}
		if t.k == tRParen {
			ps.p++
			break
		}
		v, err := ps.parseValue()
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	return Form{Head: head.v, Args: args}, nil
}

func (ps *parser) parseField() (Field, error) {
	key, err := ps.next()
	if err != nil {
		return Field{}, err
	}
	if key.k != tAtom {
		return Field{}, &ParseError{"a field must start with a key"}
	}
	var values []Node
	// allow the value to start on the next line (e.g. `body\n  (…)`)
	ps.skipNL()
	if first, ok := ps.peek(); ok && first.k != tRBrace && first.k != tNL {
		v, err := ps.parseValue()
		if err != nil {
			return Field{}, err
		}
		values = append(values, v)
	}
	// extra values may only be forms or strings — a bare atom starts the next field
	for {
		t, ok := ps.peek()
		if ok && (t.k == tLParen || t.k == tStr) {
			v, err := ps.parseValue()
			if err != nil {
				return Field{}, err
			}
			values = append(values, v)
		} else {
			break
		}
	}
	ps.skipNL()
	return Field{Key: key.v, Values: values}, nil
}

func (ps *parser) parseBlock() (Block, error) {
	head, err := ps.next()
	if err != nil {
		return Block{}, err
	}
	if head.k != tAtom {
		return Block{}, &ParseError{"a block must start with a head (type/fn/package)"}
	}
	name, err := ps.next()
	if err != nil {
		return Block{}, err
	}
	// A block name is an atom (`fn add`, `type User`) or a quoted string
	// (`package "funk/std/maths"`).
	if name.k != tAtom && name.k != tStr {
		return Block{}, &ParseError{fmt.Sprintf("block %q is missing a name", head.v)}
	}
	if err := ps.expect(tLBrace, "{"); err != nil {
		return Block{}, err
	}
	ps.skipNL()
	var fields []Field
	for {
		t, ok := ps.peek()
		if !ok || t.k == tRBrace {
			break
		}
		f, err := ps.parseField()
		if err != nil {
			return Block{}, err
		}
		fields = append(fields, f)
		ps.skipNL()
	}
	if err := ps.expect(tRBrace, "}"); err != nil {
		return Block{}, err
	}
	return Block{Head: head.v, Name: name.v, Fields: fields}, nil
}

// Parse parses .funk source into a Program.
func Parse(src string) (Program, error) {
	ps := &parser{toks: tokenize(src)}
	var prog Program
	ps.skipNL()
	for {
		if _, ok := ps.peek(); !ok {
			break
		}
		b, err := ps.parseBlock()
		if err != nil {
			return nil, err
		}
		prog = append(prog, b)
		ps.skipNL()
	}
	return prog, nil
}

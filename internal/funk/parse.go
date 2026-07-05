package funk

import (
	"fmt"
	"regexp"
	"strings"
)

// String renders a position as "line:col".
func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// ParseError is a syntax error carrying the source position where it was found.
type ParseError struct {
	Msg string
	Pos Pos
}

// Error formats as "line:col: message" so a File Watcher / LSP can place it;
// callers that know the file prefix it as "path:line:col: message".
func (e *ParseError) Error() string {
	if e.Pos.Line > 0 {
		return fmt.Sprintf("%d:%d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
	}
	return e.Msg
}

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

func kindName(k tokKind) string {
	switch k {
	case tLParen:
		return "'('"
	case tRParen:
		return "')'"
	case tLBrace:
		return "'{'"
	case tRBrace:
		return "'}'"
	case tNL:
		return "newline"
	case tStr:
		return "a string"
	case tAtom:
		return "an identifier"
	}
	return "token"
}

type tok struct {
	k         tokKind
	v         string
	line, col int
}

func (t tok) pos() Pos { return Pos{Line: t.line, Col: t.col} }

var numRe = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

const delims = " \t\r\n(){};\""

// tokenize scans src into tokens, tracking 1-based line/col for each. It returns
// the end-of-input position (for EOF diagnostics) and a ParseError if a string
// literal is left unterminated.
func tokenize(src string) ([]tok, Pos, *ParseError) {
	var toks []tok
	i, n := 0, len(src)
	line, col := 1, 1
	step := func() { // advance one source byte, tracking position
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		i++
	}
	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			toks = append(toks, tok{k: tNL, line: line, col: col})
			step()
		case c == ' ' || c == '\t' || c == '\r':
			step()
		case c == ';':
			for i < n && src[i] != '\n' {
				step()
			}
		case c == '(':
			toks = append(toks, tok{k: tLParen, line: line, col: col})
			step()
		case c == ')':
			toks = append(toks, tok{k: tRParen, line: line, col: col})
			step()
		case c == '{':
			toks = append(toks, tok{k: tLBrace, line: line, col: col})
			step()
		case c == '}':
			toks = append(toks, tok{k: tRBrace, line: line, col: col})
			step()
		case c == '"' && i+2 < n && src[i+1] == '"' && src[i+2] == '"':
			// triple-quoted multi-line string ("""…"""): content is captured
			// verbatim (no escapes), then trimmed YAML/PEP-257 style by cleandoc —
			// the first line stripped, following lines dedented by their shared
			// indent — so you can align the text under wherever it starts.
			sl, sc := line, col
			step()
			step()
			step() // opening """
			var sb strings.Builder
			for i < n && !(src[i] == '"' && i+2 < n && src[i+1] == '"' && src[i+2] == '"') {
				sb.WriteByte(src[i])
				step()
			}
			if i >= n {
				return toks, Pos{line, col}, &ParseError{Pos: Pos{sl, sc}, Msg: "unterminated string (missing closing '\"\"\"')"}
			}
			step()
			step()
			step() // closing """
			toks = append(toks, tok{k: tStr, v: cleandoc(sb.String()), line: sl, col: sc})
		case c == '"':
			sl, sc := line, col
			step() // opening quote
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
					step()
					step()
				} else {
					sb.WriteByte(src[i])
					step()
				}
			}
			if i >= n {
				return toks, Pos{line, col}, &ParseError{Pos: Pos{sl, sc}, Msg: "unterminated string (missing closing '\"')"}
			}
			step() // closing quote
			toks = append(toks, tok{k: tStr, v: sb.String(), line: sl, col: sc})
		default:
			sl, sc := line, col
			start := i
			for i < n && !strings.ContainsRune(delims, rune(src[i])) {
				step()
			}
			toks = append(toks, tok{k: tAtom, v: src[start:i], line: sl, col: sc})
		}
	}
	return toks, Pos{line, col}, nil
}

// cleandoc trims a triple-quoted block YAML/PEP-257 style: the first line (which
// may sit inline right after the opening """) is stripped of its own leading
// whitespace, and every FOLLOWING line is dedented by the whitespace prefix they
// share — so you can align the continuation lines under wherever the text starts.
// Relative indentation among the continuation lines is preserved; blank lines are
// normalized to empty; leading and trailing blank lines are dropped.
func cleandoc(s string) string {
	lines := strings.Split(s, "\n")
	min := -1
	for _, l := range lines[1:] { // the common indent is measured on lines 2..n
		t := strings.TrimLeft(l, " \t")
		if t == "" {
			continue
		}
		if ind := len(l) - len(t); min < 0 || ind < min {
			min = ind
		}
	}
	lines[0] = strings.TrimLeft(lines[0], " \t")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = ""
		} else if min > 0 {
			lines[i] = lines[i][min:]
		}
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

type parser struct {
	toks []tok
	p    int
	end  Pos // position just past the last token, for EOF errors
}

func (ps *parser) peek() (tok, bool) {
	if ps.p < len(ps.toks) {
		return ps.toks[ps.p], true
	}
	return tok{}, false
}

func (ps *parser) next() (tok, error) {
	if ps.p >= len(ps.toks) {
		return tok{}, &ParseError{Pos: ps.end, Msg: "unexpected end of input"}
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
		return &ParseError{Pos: t.pos(), Msg: fmt.Sprintf("expected %s but found %s", name, kindName(t.k))}
	}
	return nil
}

func atomNode(v string, str bool, pos Pos) Atom {
	if str {
		return Atom{Kind: "str", Value: v, Pos: pos}
	}
	if numRe.MatchString(v) {
		return Atom{Kind: "num", Value: v, Pos: pos}
	}
	return Atom{Kind: "id", Value: v, Pos: pos}
}

func (ps *parser) parseValue() (Node, error) {
	t, ok := ps.peek()
	if !ok {
		return nil, &ParseError{Pos: ps.end, Msg: "expected a value but reached end of input"}
	}
	switch t.k {
	case tLParen:
		return ps.parseForm()
	case tStr:
		ps.p++
		return atomNode(t.v, true, t.pos()), nil
	case tAtom:
		ps.p++
		return atomNode(t.v, false, t.pos()), nil
	default:
		return nil, &ParseError{Pos: t.pos(), Msg: fmt.Sprintf("unexpected %s where a value was expected", kindName(t.k))}
	}
}

func (ps *parser) parseForm() (Node, error) {
	open, _ := ps.peek()
	if err := ps.expect(tLParen, "'('"); err != nil {
		return nil, err
	}
	openPos := open.pos()
	ps.skipNL()
	if t, ok := ps.peek(); ok && t.k == tRParen {
		ps.p++
		return Form{Head: "", Args: nil, Pos: openPos}, nil // empty form, e.g. `in ()`
	}
	head, err := ps.next()
	if err != nil {
		return nil, err
	}
	if head.k != tAtom {
		return nil, &ParseError{Pos: head.pos(), Msg: fmt.Sprintf("a form must start with an operator name, found %s", kindName(head.k))}
	}
	var args []Node
	for {
		ps.skipNL()
		t, ok := ps.peek()
		if !ok {
			return nil, &ParseError{Pos: openPos, Msg: fmt.Sprintf("unterminated form — missing ')' to close the '(' opened at %s", openPos)}
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
	return Form{Head: head.v, Args: args, Pos: head.pos()}, nil
}

func (ps *parser) parseField() (Field, error) {
	key, err := ps.next()
	if err != nil {
		return Field{}, err
	}
	if key.k != tAtom {
		return Field{}, &ParseError{Pos: key.pos(), Msg: fmt.Sprintf("a field must start with a key, found %s", kindName(key.k))}
	}
	// A nested brace block: `needs { … }` / `effects { … }`, parsed line-by-line.
	if t, ok := ps.peek(); ok && t.k == tLBrace {
		sub, err := ps.parseBraceFields()
		if err != nil {
			return Field{}, err
		}
		return Field{Key: key.v, Sub: sub, Pos: key.pos()}, nil
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
	// Extra values continue the field. A form on a FOLLOWING line also continues
	// it: a field always begins with an atom key, so a leading '(' is never a new
	// field — this lets `in`/`out` span multiple lines for readable port docs. A
	// bare atom, by contrast, starts the next field.
	for {
		j := ps.p
		for j < len(ps.toks) && ps.toks[j].k == tNL {
			j++
		}
		if j < len(ps.toks) && ps.toks[j].k == tLParen {
			ps.p = j
			v, err := ps.parseValue()
			if err != nil {
				return Field{}, err
			}
			values = append(values, v)
			continue
		}
		if t, ok := ps.peek(); ok && t.k == tStr { // same-line string continuation
			v, err := ps.parseValue()
			if err != nil {
				return Field{}, err
			}
			values = append(values, v)
			continue
		}
		break
	}
	ps.skipNL()
	return Field{Key: key.v, Values: values, Pos: key.pos()}, nil
}

// parseBraceFields parses `{ key val… \n key val… }` line-by-line (used for
// needs / effects blocks, where a line is `kind alias [schema]`).
func (ps *parser) parseBraceFields() ([]Field, error) {
	open, _ := ps.peek()
	if err := ps.expect(tLBrace, "'{'"); err != nil {
		return nil, err
	}
	ps.skipNL()
	var fields []Field
	for {
		t, ok := ps.peek()
		if !ok {
			return nil, &ParseError{Pos: open.pos(), Msg: fmt.Sprintf("unterminated brace block — missing '}' to close the '{' opened at %s", open.pos())}
		}
		if t.k == tRBrace {
			break
		}
		key, err := ps.next()
		if err != nil {
			return nil, err
		}
		if key.k != tAtom {
			return nil, &ParseError{Pos: key.pos(), Msg: fmt.Sprintf("a brace-block line must start with a key, found %s", kindName(key.k))}
		}
		var vals []Node
		for {
			nt, ok := ps.peek()
			if !ok || nt.k == tNL || nt.k == tRBrace {
				break
			}
			v, err := ps.parseValue()
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}
		fields = append(fields, Field{Key: key.v, Values: vals, Pos: key.pos()})
		ps.skipNL()
	}
	if err := ps.expect(tRBrace, "'}'"); err != nil {
		return nil, err
	}
	return fields, nil
}

func (ps *parser) parseBlock() (Block, error) {
	head, err := ps.next()
	if err != nil {
		return Block{}, err
	}
	if head.k != tAtom {
		return Block{}, &ParseError{Pos: head.pos(), Msg: fmt.Sprintf("a block must start with a head (type/fn/package), found %s", kindName(head.k))}
	}
	name, err := ps.next()
	if err != nil {
		return Block{}, err
	}
	// A block name is an atom (`fn add`, `type User`) or a quoted string
	// (`package "funk/std/maths"`).
	if name.k != tAtom && name.k != tStr {
		return Block{}, &ParseError{Pos: head.pos(), Msg: fmt.Sprintf("block %q is missing a name", head.v)}
	}
	if err := ps.expect(tLBrace, "'{'"); err != nil {
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
	if err := ps.expect(tRBrace, "'}'"); err != nil {
		return Block{}, err
	}
	return Block{Head: head.v, Name: name.v, Fields: fields, Pos: head.pos()}, nil
}

// Parse parses .funk source into a Program.
func Parse(src string) (Program, error) {
	toks, end, perr := tokenize(src)
	if perr != nil {
		return nil, perr
	}
	ps := &parser{toks: toks, end: end}
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

package funk

import (
	"fmt"
	"strings"
)

// Format renders a parsed program back to canonical .funk source (the inverse
// of Parse). Because the graph is derived from code, formatting is lossless over
// the notation.
func Format(prog Program) string {
	var b strings.Builder
	for i, blk := range prog {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(formatBlock(blk))
	}
	return b.String()
}

func formatBlock(bl Block) string {
	var b strings.Builder
	name := bl.Name
	if bl.Head == "package" {
		name = quote(bl.Name)
	}
	fmt.Fprintf(&b, "%s %s {\n", bl.Head, name)
	for _, f := range bl.Fields {
		b.WriteString(formatField(f, "  "))
	}
	b.WriteString("}\n")
	return b.String()
}

func formatField(f Field, indent string) string {
	var b strings.Builder
	if f.Sub != nil {
		fmt.Fprintf(&b, "%s%s {\n", indent, f.Key)
		for _, sf := range f.Sub {
			b.WriteString(indent + "  " + sf.Key)
			for _, v := range sf.Values {
				b.WriteString(" " + formatNode(v))
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s}\n", indent)
		return b.String()
	}
	b.WriteString(indent + f.Key)
	for _, v := range f.Values {
		b.WriteString(" " + formatNode(v))
	}
	b.WriteString("\n")
	return b.String()
}

func formatNode(n Node) string {
	switch t := n.(type) {
	case Atom:
		if t.Kind == "str" {
			return quote(t.Value)
		}
		return t.Value
	case Form:
		if t.Head == "" {
			return "()"
		}
		parts := make([]string, 0, len(t.Args)+1)
		parts = append(parts, t.Head)
		for _, a := range t.Args {
			parts = append(parts, formatNode(a))
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	return ""
}

func quote(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\t", "\\t")
	return "\"" + r.Replace(s) + "\""
}

package funk

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// primitive is a native Go implementation of a `builtin`-engine function.
type primitive func(in map[string]interface{}) (interface{}, error)

var builtins = map[string]primitive{}

func regBuiltin(name string, p primitive) { builtins[name] = p }

func arg(in map[string]interface{}, k string) (float64, error) {
	v, ok := in[k]
	if !ok {
		return 0, fmt.Errorf("missing input %q", k)
	}
	return toNum(v)
}

// binNum wraps a two-number → number primitive.
func binNum(f func(a, b float64) (float64, error)) primitive {
	return func(in map[string]interface{}) (interface{}, error) {
		a, err := arg(in, "a")
		if err != nil {
			return nil, err
		}
		b, err := arg(in, "b")
		if err != nil {
			return nil, err
		}
		r, err := f(a, b)
		if err != nil {
			return nil, err
		}
		return numFmt(r), nil
	}
}

// unNum wraps a one-number → number primitive.
func unNum(f func(a float64) (float64, error)) primitive {
	return func(in map[string]interface{}) (interface{}, error) {
		a, err := arg(in, "a")
		if err != nil {
			return nil, err
		}
		r, err := f(a)
		if err != nil {
			return nil, err
		}
		return numFmt(r), nil
	}
}

// cmpNum wraps a two-number → bool primitive.
func cmpNum(f func(a, b float64) bool) primitive {
	return func(in map[string]interface{}) (interface{}, error) {
		a, err := arg(in, "a")
		if err != nil {
			return nil, err
		}
		b, err := arg(in, "b")
		if err != nil {
			return nil, err
		}
		return f(a, b), nil
	}
}

func init() {
	// arithmetic
	regBuiltin("num.add", binNum(func(a, b float64) (float64, error) { return a + b, nil }))
	regBuiltin("num.sub", binNum(func(a, b float64) (float64, error) { return a - b, nil }))
	regBuiltin("num.mul", binNum(func(a, b float64) (float64, error) { return a * b, nil }))
	regBuiltin("num.div", binNum(func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	}))
	regBuiltin("num.mod", binNum(func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return math.Mod(a, b), nil
	}))
	regBuiltin("num.pow", binNum(func(a, b float64) (float64, error) { return math.Pow(a, b), nil }))
	regBuiltin("num.min", binNum(func(a, b float64) (float64, error) { return math.Min(a, b), nil }))
	regBuiltin("num.max", binNum(func(a, b float64) (float64, error) { return math.Max(a, b), nil }))
	regBuiltin("num.neg", unNum(func(a float64) (float64, error) { return -a, nil }))
	regBuiltin("num.abs", unNum(func(a float64) (float64, error) { return math.Abs(a), nil }))
	regBuiltin("num.sqrt", unNum(func(a float64) (float64, error) {
		if a < 0 {
			return 0, fmt.Errorf("sqrt of negative")
		}
		return math.Sqrt(a), nil
	}))
	regBuiltin("num.double", unNum(func(a float64) (float64, error) { return a * 2, nil }))
	regBuiltin("num.square", unNum(func(a float64) (float64, error) { return a * a, nil }))
	regBuiltin("num.inc", unNum(func(a float64) (float64, error) { return a + 1, nil }))
	regBuiltin("num.even", func(in map[string]interface{}) (interface{}, error) {
		a, err := arg(in, "a")
		if err != nil {
			return nil, err
		}
		return math.Mod(a, 2) == 0, nil
	})

	// comparisons → Bool
	regBuiltin("num.gt", cmpNum(func(a, b float64) bool { return a > b }))
	regBuiltin("num.lt", cmpNum(func(a, b float64) bool { return a < b }))
	regBuiltin("num.gte", cmpNum(func(a, b float64) bool { return a >= b }))
	regBuiltin("num.lte", cmpNum(func(a, b float64) bool { return a <= b }))
	regBuiltin("num.eq", cmpNum(func(a, b float64) bool { return a == b }))
	regBuiltin("num.neq", cmpNum(func(a, b float64) bool { return a != b }))

	// booleans (and/or/not now live in funk std over `if`)

	// strings
	regBuiltin("str.concat", func(in map[string]interface{}) (interface{}, error) {
		return fmt.Sprint(str(in["a"]), str(in["b"])), nil
	})
	regBuiltin("str.upper", func(in map[string]interface{}) (interface{}, error) {
		return strings.ToUpper(str(in["a"])), nil
	})
	regBuiltin("str.lower", func(in map[string]interface{}) (interface{}, error) {
		return strings.ToLower(str(in["a"])), nil
	})
	regBuiltin("str.trim", func(in map[string]interface{}) (interface{}, error) {
		return strings.TrimSpace(str(in["a"])), nil
	})
	regBuiltin("str.len", func(in map[string]interface{}) (interface{}, error) {
		return int64(len(str(in["a"]))), nil
	})

	// lists (len/sum/mean now live in funk std over `fold`)
	regBuiltin("list.isEmpty", func(in map[string]interface{}) (interface{}, error) {
		return len(asList(in["a"])) == 0, nil
	})
	regBuiltin("list.first", func(in map[string]interface{}) (interface{}, error) {
		l := asList(in["a"])
		if len(l) == 0 {
			return nil, nil
		}
		return l[0], nil
	})
	regBuiltin("list.last", func(in map[string]interface{}) (interface{}, error) {
		l := asList(in["a"])
		if len(l) == 0 {
			return nil, nil
		}
		return l[len(l)-1], nil
	})

	// identity / debug
	regBuiltin("id", func(in map[string]interface{}) (interface{}, error) { return in["a"], nil })

	// filesystem (effect: fs) — the substrate reflect uses to rewrite source
	regBuiltin("sys.readFile", func(in map[string]interface{}) (interface{}, error) {
		b, err := os.ReadFile(str(in["path"]))
		if err != nil {
			return nil, err
		}
		return string(b), nil
	})
	regBuiltin("sys.writeFile", func(in map[string]interface{}) (interface{}, error) {
		path := str(in["path"])
		if dir := filepath.Dir(path); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		if err := os.WriteFile(path, []byte(str(in["content"])), 0o644); err != nil {
			return nil, err
		}
		return path, nil
	})
}

func str(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func asList(v interface{}) []interface{} {
	if l, ok := v.([]interface{}); ok {
		return l
	}
	// a live stream reaching a list context is drained (bounded streams only)
	if s, ok := v.(Stream); ok {
		if l, ok := drain(s).([]interface{}); ok {
			return l
		}
	}
	if v == nil {
		return nil
	}
	return []interface{}{v}
}

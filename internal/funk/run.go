package funk

import (
	"fmt"
	"strconv"
)

// Run executes a function (atomic or composite) by reference with named inputs.
func Run(lib *Library, ref string, inputs map[string]interface{}, opts ExecOpts) ExecResult {
	f, ok := lib.Lookup(ref)
	if !ok {
		return ExecResult{Error: fmt.Sprintf("unknown function %q", ref)}
	}
	if !f.Composite() {
		return Exec(f, inputs, opts)
	}
	e := &evalEnv{lib: lib, opts: opts, vars: map[string]interface{}{}}
	for k, v := range inputs {
		e.vars[k] = v
	}
	v, err := e.eval(f.Body)
	if err != nil {
		return ExecResult{Error: err.Error()}
	}
	return ExecResult{OK: true, Value: v}
}

type evalEnv struct {
	lib  *Library
	opts ExecOpts
	vars map[string]interface{}
}

func (e *evalEnv) child() *evalEnv {
	c := &evalEnv{lib: e.lib, opts: e.opts, vars: map[string]interface{}{}}
	for k, v := range e.vars {
		c.vars[k] = v
	}
	return c
}

func (e *evalEnv) eval(n Node) (interface{}, error) {
	switch t := n.(type) {
	case Atom:
		return e.evalAtom(t)
	case Form:
		return e.evalForm(t)
	}
	return nil, fmt.Errorf("cannot evaluate node")
}

func (e *evalEnv) evalAtom(a Atom) (interface{}, error) {
	switch a.Kind {
	case "num":
		f, err := strconv.ParseFloat(a.Value, 64)
		if err != nil {
			return nil, err
		}
		return numFmt(f), nil
	case "str":
		return a.Value, nil
	case "id":
		switch a.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null", "nil":
			return nil, nil
		}
		if v, ok := e.vars[a.Value]; ok {
			return v, nil
		}
		return nil, fmt.Errorf("unknown identifier %q", a.Value)
	}
	return nil, fmt.Errorf("bad atom")
}

func (e *evalEnv) evalForm(f Form) (interface{}, error) {
	switch f.Head {
	case "do":
		return e.evalDo(f.Args)
	case "let":
		return e.evalLet(f.Args)
	case "if":
		return e.evalIf(f.Args)
	case "return":
		if len(f.Args) == 0 {
			return nil, nil
		}
		return e.eval(f.Args[0])
	case "exit":
		if len(f.Args) == 0 {
			return nil, nil
		}
		return e.eval(f.Args[0]) // v1: exit yields its value (a terminal)
	case "for-each":
		return e.evalForEach(f.Args)
	case "window":
		return e.evalWindow(f.Args)
	default:
		return e.evalCall(f)
	}
}

func (e *evalEnv) evalDo(args []Node) (interface{}, error) {
	var last interface{}
	for _, a := range args {
		v, err := e.eval(a)
		if err != nil {
			return nil, err
		}
		last = v
	}
	return last, nil
}

// (let (name expr) body)
func (e *evalEnv) evalLet(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("let: expected (let (name expr) body)")
	}
	bind, ok := args[0].(Form)
	if !ok || len(bind.Args) != 1 {
		return nil, fmt.Errorf("let: first arg must be (name expr)")
	}
	val, err := e.eval(bind.Args[0])
	if err != nil {
		return nil, err
	}
	c := e.child()
	c.vars[bind.Head] = val
	return c.eval(args[1])
}

// (if cond then else)
func (e *evalEnv) evalIf(args []Node) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("if: expected (if cond then [else])")
	}
	cond, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	if truthy(cond) {
		return e.eval(args[1])
	}
	if len(args) >= 3 {
		return e.eval(args[2])
	}
	return nil, nil
}

// (for-each coll (item) body) — consume by effect; returns nil.
func (e *evalEnv) evalForEach(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("for-each: expected (for-each coll (item) body)")
	}
	coll, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	bind, ok := args[1].(Form)
	if !ok {
		return nil, fmt.Errorf("for-each: second arg must be (item)")
	}
	for _, item := range asList(coll) {
		c := e.child()
		c.vars[bind.Head] = item
		if _, err := c.eval(args[2]); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// (window coll size …) — count window on a finite list: last `size` items.
func (e *evalEnv) evalWindow(args []Node) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("window: expected (window stream size …)")
	}
	coll, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	size, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	n, err := toNum(size)
	if err != nil {
		return nil, err
	}
	l := asList(coll)
	if k := int(n); k >= 0 && k < len(l) {
		l = l[len(l)-k:]
	}
	return l, nil
}

// A plain call: (fn arg…). Args map positionally to the callee's `in` ports.
func (e *evalEnv) evalCall(f Form) (interface{}, error) {
	callee, ok := e.lib.Lookup(f.Head)
	if !ok {
		return nil, fmt.Errorf("unknown function %q", f.Head)
	}
	inputs := map[string]interface{}{}
	for i, arg := range f.Args {
		v, err := e.eval(arg)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("_%d", i)
		if i < len(callee.In) {
			name = callee.In[i].Name
		}
		inputs[name] = v
	}
	res := Run(e.lib, f.Head, inputs, e.opts)
	if !res.OK {
		return nil, fmt.Errorf("%s: %s", f.Head, res.Error)
	}
	return res.Value, nil
}

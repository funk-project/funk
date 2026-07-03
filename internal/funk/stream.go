package funk

import "fmt"

// Stream is a reactive stream: values over time, closed on completion.
// docs/03: stream = chan; onNext = send; onComplete = close; cancel = ctx.
type Stream = chan interface{}

// send respects cancellation: false if the scope was cancelled.
func (e *evalEnv) send(out Stream, v interface{}) bool {
	select {
	case out <- v:
		return true
	case <-e.ctx.Done():
		return false
	}
}

// asStream coerces a value to a Stream — a Stream passes through; a List/value
// becomes a finite stream.
func (e *evalEnv) asStream(v interface{}) Stream {
	if s, ok := v.(Stream); ok {
		return s
	}
	out := make(Stream)
	items := asList(v)
	go func() {
		defer close(out)
		for _, it := range items {
			if !e.send(out, it) {
				return
			}
		}
	}()
	return out
}

// drain collects a stream to a List (blocks until complete).
func drain(s Stream) interface{} {
	out := []interface{}{}
	for v := range s {
		out = append(out, v)
	}
	return out
}

func fnRefName(n Node) (string, bool) {
	if a, ok := n.(Atom); ok && a.Kind == "id" {
		return a.Value, true
	}
	return "", false
}

func (e *evalEnv) oneArg(fn string, v interface{}) map[string]interface{} {
	name := "a"
	if callee, ok := e.lib.Lookup(fn); ok && len(callee.In) > 0 {
		name = callee.In[0].Name
	}
	return map[string]interface{}{name: v}
}

// (range a b) — emit a..b inclusive.
func (e *evalEnv) evalRange(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("range: (range a b)")
	}
	av, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	bv, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	a, err := toNum(av)
	if err != nil {
		return nil, err
	}
	b, err := toNum(bv)
	if err != nil {
		return nil, err
	}
	out := make(Stream)
	go func() {
		defer close(out)
		for i := a; i <= b; i++ {
			if !e.send(out, numFmt(i)) {
				return
			}
		}
	}()
	return out, nil
}

// (nats) — infinite 0,1,2,… (a live source; bound it with take/window).
func (e *evalEnv) evalNats(args []Node) (interface{}, error) {
	out := make(Stream)
	go func() {
		defer close(out)
		for i := 0.0; ; i++ {
			if !e.send(out, numFmt(i)) {
				return
			}
		}
	}()
	return out, nil
}

// (repeat v) — infinite stream of v.
func (e *evalEnv) evalRepeat(args []Node) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("repeat: (repeat v)")
	}
	v, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	out := make(Stream)
	go func() {
		defer close(out)
		for {
			if !e.send(out, v) {
				return
			}
		}
	}()
	return out, nil
}

// (map stream fn) — apply fn to each item.
func (e *evalEnv) evalMap(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("map: (map stream fn)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	fn, ok := fnRefName(args[1])
	if !ok {
		return nil, fmt.Errorf("map: second arg must be a function name")
	}
	in := e.asStream(src)
	out := make(Stream)
	go func() {
		defer close(out)
		for v := range in {
			res := Run(e.lib, fn, e.oneArg(fn, v), e.opts)
			if !res.OK {
				e.cancel()
				return
			}
			if !e.send(out, res.Value) {
				return
			}
		}
	}()
	return out, nil
}

// (filter stream fn) — keep items where fn is truthy.
func (e *evalEnv) evalFilter(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("filter: (filter stream fn)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	fn, ok := fnRefName(args[1])
	if !ok {
		return nil, fmt.Errorf("filter: second arg must be a function name")
	}
	in := e.asStream(src)
	out := make(Stream)
	go func() {
		defer close(out)
		for v := range in {
			res := Run(e.lib, fn, e.oneArg(fn, v), e.opts)
			if !res.OK {
				e.cancel()
				return
			}
			if truthy(res.Value) {
				if !e.send(out, v) {
					return
				}
			}
		}
	}()
	return out, nil
}

// (take stream n) — first n items, then cancel upstream (reactive cancellation).
func (e *evalEnv) evalTake(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("take: (take stream n)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	nv, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	nf, err := toNum(nv)
	if err != nil {
		return nil, err
	}
	n := int(nf)
	in := e.asStream(src)
	out := make(Stream)
	go func() {
		defer close(out)
		i := 0
		for v := range in {
			if i >= n {
				break
			}
			if !e.send(out, v) {
				return
			}
			i++
		}
		e.cancel() // enough taken — stop upstream sources
	}()
	return out, nil
}

// (collect stream) — drain a (bounded) stream to a List.
func (e *evalEnv) evalCollect(args []Node) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("collect: (collect stream)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	return drain(e.asStream(src)), nil
}

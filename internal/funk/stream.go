package funk

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// (each stream (item) body) — iterate a stream; body may `yield` to the output.
// This + yield is the minimal primitive over which map/filter are defined IN
// funk (self-hosting: the operators become readable artifacts, not Go core).
func (e *evalEnv) evalEach(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("each: (each stream (item) body)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	bind, ok := args[1].(Form)
	if !ok {
		return nil, fmt.Errorf("each: second arg must be (item)")
	}
	in := e.asStream(src)
	out := make(Stream)
	go func() {
		defer close(out)
		for item := range in {
			c := e.child()
			c.vars[bind.Head] = item
			c.yieldTo = out
			if _, err := c.eval(args[2]); err != nil {
				if errors.Is(err, errContinue) {
					continue
				}
				if errors.Is(err, errBreak) {
					return
				}
				e.cancel()
				return
			}
		}
	}()
	return out, nil
}

// (yield v) — emit v to the enclosing each's output stream.
func (e *evalEnv) evalYield(args []Node) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("yield: (yield v)")
	}
	v, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	if e.yieldTo != nil {
		e.send(e.yieldTo, v)
	}
	return nil, nil
}

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

// (tick dur) — an infinite live source emitting 0,1,2,… every dur (a real-time
// pipeline; bound it with take). e.g. (tick 200ms).
func (e *evalEnv) evalTick(args []Node) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("tick: (tick <duration>)")
	}
	a, ok := args[0].(Atom)
	if !ok {
		return nil, fmt.Errorf("tick: expected a duration like 200ms")
	}
	sec, ok := parseDuration(a.Value)
	if !ok {
		return nil, fmt.Errorf("tick: bad duration %q", a.Value)
	}
	out := make(Stream)
	go func() {
		defer close(out)
		ticker := time.NewTicker(time.Duration(sec * float64(time.Second)))
		defer ticker.Stop()
		i := 0.0
		for {
			select {
			case <-e.ctx.Done():
				return
			case <-ticker.C:
				if !e.send(out, numFmt(i)) {
					return
				}
				i++
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

// map and filter are no longer core forms — they are defined in funk std, over
// each/yield (see std/stream/operators.funk). The Go kernel shrank.

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

// countWindow emits a List every n items (tumbling); a trailing partial window
// is emitted on completion.
func (e *evalEnv) countWindow(in Stream, n int) Stream {
	out := make(Stream)
	go func() {
		defer close(out)
		var buf []interface{}
		for v := range in {
			buf = append(buf, v)
			if n > 0 && len(buf) >= n {
				w := buf
				buf = nil
				if !e.send(out, w) {
					return
				}
			}
		}
		if len(buf) > 0 {
			e.send(out, buf)
		}
	}()
	return out
}

// slideCountWindow emits overlapping windows of exactly `size` items, advancing
// by `slide` each time — `(window s size every slide)`. Only full windows are
// emitted (no trailing partial), so 1..5 with size 3 every 1 → [1,2,3] [2,3,4]
// [3,4,5]. slide == size degenerates to tumbling (without the partial tail).
func (e *evalEnv) slideCountWindow(in Stream, size, slide int) Stream {
	out := make(Stream)
	if slide < 1 {
		slide = 1
	}
	go func() {
		defer close(out)
		var buf []interface{}
		start := 0
		for v := range in {
			buf = append(buf, v)
			for size > 0 && len(buf) >= start+size {
				w := make([]interface{}, size)
				copy(w, buf[start:start+size])
				if !e.send(out, w) {
					return
				}
				start += slide
			}
		}
	}()
	return out
}

// timeWindow emits a List per event-time bucket of width `dur` (tumbling). The
// watermark is simple: a later bucket closes all earlier ones (docs/04 §7).
func (e *evalEnv) timeWindow(in Stream, dur float64, field string) Stream {
	out := make(Stream)
	go func() {
		defer close(out)
		buckets := map[int64][]interface{}{}
		emit := func(b int64) bool {
			if items, ok := buckets[b]; ok {
				delete(buckets, b)
				return e.send(out, items)
			}
			return true
		}
		// close every existing bucket with key < b, in ascending order.
		closeBefore := func(b int64) bool {
			var toClose []int64
			for k := range buckets {
				if k < b {
					toClose = append(toClose, k)
				}
			}
			sort.Slice(toClose, func(i, j int) bool { return toClose[i] < toClose[j] })
			for _, k := range toClose {
				if !emit(k) {
					return false
				}
			}
			return true
		}
		for v := range in {
			t := eventTime(v, field)
			b := int64(t / dur)
			buckets[b] = append(buckets[b], v)
			// watermark: a later event closes earlier buckets (default drop late).
			if !closeBefore(b) {
				return
			}
		}
		// drain remaining buckets in ascending order
		keys := make([]int64, 0, len(buckets))
		for k := range buckets {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, k := range keys {
			if !emit(k) {
				return
			}
		}
	}()
	return out
}

func eventTime(v interface{}, field string) float64 {
	if m, ok := v.(map[string]interface{}); ok {
		if t, ok := m[field]; ok {
			if f, err := toNum(t); err == nil {
				return f
			}
		}
	}
	f, _ := toNum(v)
	return f
}

// parseDuration parses `5s` / `1m` / `100ms` / `2h` / `1d` to seconds.
func parseDuration(s string) (float64, bool) {
	for _, u := range []struct {
		suf string
		mul float64
	}{{"ms", 0.001}, {"s", 1}, {"m", 60}, {"h", 3600}, {"d", 86400}} {
		if strings.HasSuffix(s, u.suf) {
			if f, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suf), 64); err == nil {
				return f * u.mul, true
			}
		}
	}
	return 0, false
}

// (scan stream fn init) — running fold: emit fn(acc, x) for each item, starting
// from init. e.g. (scan (range 1 5) add 0) → 1,3,6,10,15.
func (e *evalEnv) evalScan(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("scan: (scan stream fn init)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	fn, ok := fnRefName(args[1])
	if !ok {
		return nil, fmt.Errorf("scan: second arg must be a function name")
	}
	init, err := e.eval(args[2])
	if err != nil {
		return nil, err
	}
	in := e.asStream(src)
	out := make(Stream)
	go func() {
		defer close(out)
		acc := init
		callee, _ := e.lib.Lookup(fn)
		for v := range in {
			m := map[string]interface{}{"a": acc, "b": v}
			if callee != nil && len(callee.In) >= 2 {
				m = map[string]interface{}{callee.In[0].Name: acc, callee.In[1].Name: v}
			}
			res := Run(e.lib, fn, m, e.opts)
			if !res.OK {
				e.cancel()
				return
			}
			acc = res.Value
			if !e.send(out, acc) {
				return
			}
		}
	}()
	return out, nil
}

// (merge s1 s2) — interleave two streams as items arrive.
func (e *evalEnv) evalMerge(args []Node) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("merge: (merge s1 s2)")
	}
	a, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	b, err := e.eval(args[1])
	if err != nil {
		return nil, err
	}
	sa, sb := e.asStream(a), e.asStream(b)
	out := make(Stream)
	go func() {
		defer close(out)
		var wg sync.WaitGroup
		wg.Add(2)
		pipe := func(s Stream) {
			defer wg.Done()
			for v := range s {
				if !e.send(out, v) {
					return
				}
			}
		}
		go pipe(sa)
		go pipe(sb)
		wg.Wait()
	}()
	return out, nil
}

// (fold stream f init) — reduce a stream to a value: acc = f(acc, item) for each
// item, starting from init. The minimal stateful consumer; sum/count/mean are
// defined in funk over it. Synchronous (blocks on a bounded stream).
func (e *evalEnv) evalFold(args []Node) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("fold: (fold stream f init)")
	}
	src, err := e.eval(args[0])
	if err != nil {
		return nil, err
	}
	fn, ok := fnRefName(args[1])
	if !ok {
		return nil, fmt.Errorf("fold: second arg must be a function name")
	}
	acc, err := e.eval(args[2])
	if err != nil {
		return nil, err
	}
	callee, _ := e.lib.Lookup(fn)
	for v := range e.asStream(src) {
		m := map[string]interface{}{"a": acc, "b": v}
		if callee != nil && len(callee.In) >= 2 {
			m = map[string]interface{}{callee.In[0].Name: acc, callee.In[1].Name: v}
		}
		res := Run(e.lib, fn, m, e.opts)
		if !res.OK {
			return nil, fmt.Errorf("fold: %s", res.Error)
		}
		acc = res.Value
	}
	return acc, nil
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

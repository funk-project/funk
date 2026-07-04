package funk

import (
	"fmt"
	"sync"
)

// callReactive fires the callee once per input item (docs/07 §5 — the function as
// a dataflow node that wakes on input). Stream arguments bound to *scalar* ports
// drive firing, grouped by policy: `zip` pairs them 1:1, `latest` combines the
// most recent of each (combineLatest). Non-driving args — plain scalars and whole
// Streams passed to Stream-typed ports — are sampled as constants on every fire.
// It returns the output stream (unwrapped per the callee's out-ports).
func (e *evalEnv) callReactive(callee *Fn, ref, id string, vals []interface{}) (interface{}, error) {
	var names []string
	var streams []Stream
	consts := map[string]interface{}{}
	anyLatest := false
	for i, v := range vals {
		name := fmt.Sprintf("_%d", i)
		policy := "zip"
		if i < len(callee.In) {
			name = callee.In[i].Name
			policy = callee.In[i].Policy
		}
		if s, ok := v.(Stream); ok && scalarPort(callee, i) {
			names = append(names, name)
			streams = append(streams, s)
			if policy == "latest" {
				anyLatest = true
			}
			continue
		}
		consts[name] = v
	}

	var tuples <-chan map[string]interface{}
	if anyLatest {
		tuples = e.fireLatest(callee, names, streams)
	} else {
		tuples = e.fireZip(names, streams)
	}

	out := make(Stream)
	go func() {
		defer close(out)
		for tup := range tuples {
			inputs := map[string]interface{}{}
			for k, v := range consts {
				inputs[k] = v
			}
			for k, v := range tup {
				inputs[k] = v
			}
			e.fillDefaults(callee, inputs)
			if err := validateInputs(callee, inputs); err != nil {
				e.cancel()
				return
			}
			if callee.Composite() {
				if _, err := e.runComposite(callee, inputs, out); err != nil {
					e.cancel()
					return
				}
				continue
			}
			res := Exec(callee, inputs, e.opts)
			if !res.OK {
				e.cancel()
				return
			}
			if !e.send(out, res.Value) {
				return
			}
		}
	}()
	e.emit(TraceEvent{Fn: ref, Kind: "call", Node: id})
	return out, nil
}

// fireZip pairs the driving streams 1:1: it emits a tuple once every stream has
// produced its next item, and stops when any of them completes (docs/07 §2.1).
func (e *evalEnv) fireZip(names []string, streams []Stream) <-chan map[string]interface{} {
	out := make(chan map[string]interface{})
	go func() {
		defer close(out)
		for {
			tup := make(map[string]interface{}, len(streams))
			for i, s := range streams {
				v, ok := <-s
				if !ok {
					return
				}
				tup[names[i]] = v
			}
			select {
			case out <- tup:
			case <-e.ctx.Done():
				return
			}
		}
	}()
	return out
}

// fireLatest combines the most recent value of each driving stream (combineLatest,
// docs/07 §2.1): it emits once every required port has a value, then on each new
// arrival. A port with a default is seeded (and thus optional).
func (e *evalEnv) fireLatest(callee *Fn, names []string, streams []Stream) <-chan map[string]interface{} {
	out := make(chan map[string]interface{})
	go func() {
		defer close(out)
		latest := make([]interface{}, len(streams))
		has := make([]bool, len(streams))
		for i, name := range names {
			if p, ok := portByName(callee, name); ok && p.Optional && p.Default != nil {
				if dv, err := e.eval(p.Default); err == nil {
					latest[i], has[i] = dv, true
				}
			}
		}
		type item struct {
			idx int
			v   interface{}
		}
		agg := make(chan item)
		var wg sync.WaitGroup
		wg.Add(len(streams))
		for i, s := range streams {
			go func(i int, s Stream) {
				defer wg.Done()
				for v := range s {
					select {
					case agg <- item{i, v}:
					case <-e.ctx.Done():
						return
					}
				}
			}(i, s)
		}
		go func() { wg.Wait(); close(agg) }()

		allHave := func() bool {
			for _, h := range has {
				if !h {
					return false
				}
			}
			return true
		}
		for it := range agg {
			latest[it.idx], has[it.idx] = it.v, true
			if !allHave() {
				continue
			}
			tup := make(map[string]interface{}, len(names))
			for i, name := range names {
				tup[name] = latest[i]
			}
			select {
			case out <- tup:
			case <-e.ctx.Done():
				return
			}
		}
	}()
	return out
}

// fillDefaults injects the default value of any optional input port not supplied.
func (e *evalEnv) fillDefaults(callee *Fn, inputs map[string]interface{}) {
	for _, p := range callee.In {
		if _, ok := inputs[p.Name]; ok || !p.Optional || p.Default == nil {
			continue
		}
		if dv, err := e.eval(p.Default); err == nil {
			inputs[p.Name] = dv
		}
	}
}

func portByName(f *Fn, name string) (Port, bool) {
	for _, p := range f.In {
		if p.Name == name {
			return p, true
		}
	}
	return Port{}, false
}

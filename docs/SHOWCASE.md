# funk — showcase (copy, paste, run)

Every block below runs against the reference CLI:

```sh
go install github.com/funk-project/funk/cmd/funk@latest   # or: go build -o bin/funk ./cmd/funk
```

## 1. funk writes funk

An AI-authored function, checked and added to the library, then run:

```sh
funk make "reverse the order of words in a sentence" reverseWords
funk run reverse_words s="the machine is no longer a black box"
# → box black a longer no is machine the
```

The pipeline is itself funk: `architect → programmer → reviewer → tester → reflect` (std/skills).

## 2. The black box, open

The run is data — the execution path, branch decisions, and values:

```sh
funk run bump x=3 --trace
# value 3, and events: gt→false, branch→else, return —  the `add` NEVER runs.
# The condition gates the branch, and you can SEE it.
```

## 3. Reactive, live, infinite

```sh
funk run evens n=6        # 0 2 4 6 8 10  — maps an INFINITE source, take bounds it
funk run runningSum n=5   # 1 3 6 10 15   — a running fold (scan)
funk run ticks n=5        # 0 1 2 3 4      — one every 200ms, a live pipeline
funk run batchSums n=10   # 6 15 24 10    — tumbling windows of 3, summed
```

## 4. funk makes itself

`map` is not a Go black box — it's funk, over the `each`/`yield` primitive:

```
fn fmap {
  in  (xs Stream) (f Fn)
  out (r Stream)
  body (each xs (item) (yield (f item)))
}
```

```sh
funk run fmapSum n=4      # 20  — the funk-defined map matches the Go one
```

## 5. Self-observable & resource-aware

```sh
funk introspect triage    # what it calls + the needs/effects it declares, aggregated up
funk run siteGreeting --bind config.site=funk   # a config injected at runtime → hello from funk
```

## 6. Over the wire

```sh
funk serve &                                       # funkd
curl -N localhost:7777/run -d '{"ref":"evens","inputs":{"n":5}}'
# {"value":0}\n{"value":2}\n…  — the pipeline streams over HTTP
```

## 7. Teach any AI

```sh
funk prompt | pbcopy    # paste into Claude / GPT / any chat — now it writes funk
```

---

Open, Apache-2.0 / CC0 — a gift, built by a human and an AI as peers.
See [`docs/01-the-idea.md`](01-the-idea.md) for why this is possible now.

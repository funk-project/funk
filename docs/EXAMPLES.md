# funk — examples (a cookbook)

Task-shaped, escalating examples. **Every command below was run against the reference CLI and
its output is real** — except §12, which drives an LLM and so varies by model. Where a function
is defined here it lives in
[`std/examples/cookbook.funk`](../std/examples/cookbook.funk); the rest are in the other
`std/examples/*.funk` files.

Build the CLI once:

```sh
go build -o bin/funk ./cmd/funk    # or: go install github.com/funk-project/funk/cmd/funk@latest
```

> Reading funk: `(head arg…)` is a prefix form — `(add x 100)` means "add 100 to x". A function
> is **atomic** (`engine` + `src`, code) or **composite** (`body`, a composition). The `;` starts
> a comment. See [`funk prompt`](../README.md) for the one-page primer.

---

## 1. Run a function

The stdlib is full of functions; `run` calls one with named inputs.

```sh
$ funk run add a=40 b=2
42
```

## 2. Write an atomic function (Python)

An atomic function runs code on an engine. Input ports arrive as local variables; you `return`
the result. This is `slugify` from the cookbook:

```
fn slugify {
  doc "turn a title into a url slug (lowercase, non-alnum → single hyphen)"
  in  (s Str)
  out (r Str)
  engine python
  src "import re; return re.sub(r'[^a-z0-9]+', '-', s.lower()).strip('-')"
  test (is (slugify "The Machine Is No Longer a Black Box!") "the-machine-is-no-longer-a-black-box")
}
```

```sh
$ funk run slugify s="The Machine Is No Longer a Black Box!"
the-machine-is-no-longer-a-black-box

$ funk run initials s="funk over fear"
FOF

$ funk run wordCount s="a language for the plans AIs make"
7
```

The `test` line is an inline assertion — `funk test` runs it. funk verifies funk.

## 3. Compose functions (no engine, just a body)

A composite function names no engine — its `body` composes other functions, and the execution
graph is *derived* from that code. `celsiusToF` is `c × 1.8 + 32`:

```
fn celsiusToF {
  in  (c Num)
  out (r Num)
  body (return (add (mul c 1.8) 32))
}
```

```sh
$ funk run celsiusToF c=20
68
$ funk run celsiusToF c=100
212
```

## 4. Branch — and *see* that the condition gates it

`bump` adds 100 only when `x > 5`. Because funk evaluates only the taken branch, the `add`
literally never runs for `x=3`. `--trace` shows the run as data:

```sh
$ funk run bump x=3 --trace
3
{
  "ref": "bump",
  "ok": true,
  "value": 3,
  "events": [
    { "kind": "call",   "fn": "gt", "value": false },
    { "kind": "branch", "detail": "else" },
    { "kind": "terminal", "detail": "return" }
  ]
}
```

`gt → false`, branch `else`, return — the `add` is absent. The black box, open.

## 5. Transform a live stream

Streams can be infinite; operators run as live pipelines and `take` bounds (and cancels) the
source upstream.

```sh
$ funk run evens n=6         # map an INFINITE source, take 6
0
2
4
6
8
10

$ funk run squares n=5       # squares of 1..n
1
4
9
16
25
```

## 6. Aggregate over a stream (windows & running folds)

```sh
$ funk run batchSums n=10    # sum of each tumbling window of 3
6
15
24
10

$ funk run runningSum n=5    # a running fold (scan)
1
3
6
10
15
```

## 7. Pass a function as a value (higher-order)

Functions are first-class — passed by name as an introspectable reference, not an opaque closure.

```sh
$ funk run quadruple x=5      # applyTwice(double, 5)
20
$ funk run incThenSquare x=4  # compose2(square, inc, 4)
25
```

## 8. A stateful loop

```sh
$ funk run powTwoLE n=1000    # largest power of two ≤ n, via a while loop
512
```

## 9. Recover from errors (fallback & retry)

By default an error propagates and cancels the run. Two opt-in forms handle it. `on-error` runs a
fallback; the trace records a `recover` event so you can see it fired.

```
fn safeDiv {
  in  (a Num) (b Num)
  out (r Num)
  body (on-error (return (div a b)) (e) (return 0))
}
```

```sh
$ funk run safeDiv a=10 b=2
5
$ funk run safeDiv a=10 b=0        # divide-by-zero → recovered
0
```

`(retry <body> <n> [backoff <dur>])` re-runs a flaky body up to `n` attempts (the error
propagates only after the last), with an optional cancellable backoff — ideal for a network or
LLM call that occasionally fails.

## 10. Inject config & secrets (and watch them stay masked)

A function declares what it `needs`; values are injected at runtime from `--bind` (or env
`FUNK_<KIND>_<ALIAS>`). Keep secrets off the command line with `@file`, `@-` (stdin), or
`env:VAR` — and note the trace **redacts** secret values.

```sh
$ funk run siteGreeting --bind config.site=funk
hello from funk

$ printf 'ghp_supersecrettoken123' > token.txt
$ funk run secretPeek --bind config.repo=funk-project/funk --bind secret.token=@token.txt --trace
funk-project/funk @ ghp***
{
  "ref": "secretPeek",
  "ok": true,
  "value": "funk-project/funk @ ghp***",
  "events": [
    { "kind": "call", "fn": "secretPeek", "value": "funk-project/funk @ ghp***" }
  ]
}
```

The raw token never appears — not in the trace, not in `ps` (it was read from a file). See
[`docs/05`](05-resources-and-integrations.md) for the full resource model.

## 11. Inspect a function without running it

```sh
$ funk graph bump
bump(x Num) → r Num
└─ if ⟨cond⟩ ⟨then⟩ ⟨else⟩
   ├─ gt()
   │  ├─ x
   │  └─ 5
   ├─ return ◂ terminal
   │  └─ add()
   │     ├─ x
   │     └─ 100
   └─ return ◂ terminal
      └─ x
```

```sh
$ funk introspect secretPeek     # structure + declared needs/effects (aggregated up)
{
  "name": "secretPeek",
  "kind": "atomic",
  "engine": "python",
  "out": [ { "name": "r", "type": "Str" } ],
  "needs": [
    { "kind": "config", "alias": "repo", "schema": "Str" },
    { "kind": "secret", "alias": "token" }
  ]
}
```

## 12. Serve funk over HTTP

```sh
$ funk serve &                    # funkd on :7777
$ curl -N localhost:7777/run -d '{"ref":"evens","inputs":{"n":5}}'
{"value":0}
{"value":2}
{"value":4}
{"value":6}
{"value":8}
```

Each stream item is flushed as it is produced — the pipeline streams over the wire.

## 13. Let funk write funk

The self-programming loop (`architect → programmer → check → reflect`, on the `claude` engine)
generates a new function, checks it, runs its tests, and keeps it only if green (a failing
generation is discarded, never left in `std/generated`). Requires `claude` in your PATH; the
generated code — and so the exact function name — varies by model:

```sh
$ funk make "reverse the order of words in a sentence" reverseWords
$ funk run reverse_words s="the machine is no longer a black box"
box black a longer no is machine the
```

## Write your own

1. Add a `.funk` file (start from the `package "…" { version … }` block).
2. `funk check -f yourfile.funk` — static errors come back as `path:line:col: message`.
3. `funk test -f yourfile.funk` — run your inline `test (is (call) expected)` assertions.
4. `funk run -f yourfile.funk yourFn arg=…` — run it.

Stuck on the notation? `funk prompt` prints a one-page primer you can paste into any AI.

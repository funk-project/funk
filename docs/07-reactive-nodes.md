# funk — Reactive Nodes (the function as a dataflow actor)

> Status: **implemented (2026-07-04).** Co-designed by Bruno and the assistant, then built the
> same session. This closes the gap found that day: funk was reactive *at the stream layer*
> (sources/operators are goroutines over channels) but **not at the function layer** — the
> evaluator was an eager, pull-based tree-walk, so a function did **not** fire when its inputs
> arrived. The **function is now a reactive node**: a Stream argument bound to a scalar input port
> drives **per-item** firing (`internal/funk/firing.go`), outputs are named and emitted with
> `flush`, and `return` is gone. It was a **breaking** change (all of `std/` migrated); pre-0.1,
> we took the clean break.
>
> This doc now describes what shipped. A few extensions stay marked *(proposal)*.

## 0. The gap, precisely

Today (`internal/funk/run.go`):

- `evalCall` evaluates a call's arguments **sequentially** in a `for` loop, then runs the body —
  pull, depth-first. No goroutine per node. So `docs/01`'s "each node is a goroutine, parallelism
  is implicit" and `docs/04 §7`'s "functions communicate as streams; a node fires on `onNext`"
  are realized **only** for the built-in stream operators (`each`/`take`/`merge`/…), never for
  ordinary function calls.
- An atomic `src` runs **once per call** on a materialized input map (`engine.go` `execPython`
  passes one JSON blob, calls `_fn(**_in)` once) — it is *not* the NDJSON stream-processor of
  `docs/04 §4`.

The fix reframes every function as a **node that wakes when its inputs are ready, runs, and emits
named outputs** — one uniform model in which *source*, *transform*, and *sink* are just "how many
times it flushes".

## 1. The node model

A node is a reactive dataflow actor:

```
            ┌─────────────────────────────┐
  x ─▶  in  │   body: reads in-ports,      │  out ─▶ r
  y ─▶  in  │   (set …) stages outputs,    │  out ─▶ q
            │   (flush) emits a tuple      │
            └─────────────────────────────┘
```

1. **Input ports** carry typed streams and are grouped by a **firing policy** (§2). The node
   **wakes** when the policy says its inputs are ready.
2. On waking, the **body** runs (§4): it reads the current input values, `set`s named outputs, and
   `flush`es to **emit** one output tuple. It may flush **0..N** times.
3. **Output ports** are named streams (§3). Flushing sends one tuple across all output ports; when
   the inputs complete, the output ports close (`onComplete`).

`flush` count is the whole taxonomy: **0 flushes ⇒ sink**, **1 flush per input tuple ⇒ transform**,
**N flushes ⇒ source/expander**. There is no separate notion of source vs transform.

## 2. Input ports — firing groups + specs

`in` holds one or more **firing groups**; each group is a form whose head is the policy and whose
args are **port specs**. Ungrouped ports are an implicit `zip` group.

```
in (zip    (x Num (min 0))
           (y Num (max 10)))            ; fire when BOTH have a new item (paired 1:1)

in (latest (x Num (min 0))
           (y Num (max 10) (default 1)))  ; fire on any new item, newest of the others
```

### 2.1 Firing policies

- **`zip`** *(default)* — pair by position: the node fires when **every** port in the group has an
  unconsumed item; it consumes one from each. No value is dropped; the faster port buffers. For a
  scalar (length-1 stream), `zip` and `latest` **coincide** — one value each ⇒ one firing — so the
  default is invisible in the common `(add a b)` case.
- **`latest`** — fire when **any** port in the group produces a new item, combining it with the
  **most recent** value of the others (once each required port has ≥1). The reactive-UI /
  `combineLatest` behaviour; specialized, opt-in.
- Groups compose: `in (zip (a) (b)) (latest (c))` fires on each `a,b` pair using the latest `c`
  (the `withLatestFrom` shape).

### 2.2 Port spec

A port is `(name Type spec…)`, each spec a prefix form `(key value…)` — no `=`, no commas (stays
homoiconic; the parser already reads nested forms):

| spec | meaning | when checked |
|---|---|---|
| `(min n)` / `(max n)` | numeric range (refinement) | on input arrival |
| `(default v)` | value used until the first real item arrives | — |
| `(doc "…")` | human description of the port (display only) | — |
| `(len min max)` *(proposal)* | Str/List length bound | on arrival |
| `(one-of a b …)` *(proposal)* | enum | on arrival |
| `(pattern "…")` *(proposal)* | Str regex | on arrival |

A spec violation on an input is an **`onError` that cancels the scope** (`docs/04 §6`,
propagate-and-cancel) — the contract is enforced, and it is introspectable data (ambition #3:
*prove what the agent did*). `Check` gains real teeth here (today it validates only arity /
unknown ids, never types or contracts — the gap found in `typecheck.go`).

### 2.3 `required` ⟺ `default` **(open — confirm)**

We propose **no `required` keyword**. Presence is derived:

- every port in a `zip` group is **required** (no pair without it);
- a `latest` port is **optional iff it has `(default v)`** (fires using the default until its first
  real item); without a default it is **required** (needs ≥1 value before the first fire).

So *optional ⟺ has a default*. This is the one point where the proposal overrides an earlier call
(Bruno had kept `(required true)`); confirm dropping it, or we keep `required` as an explicit
override.

## 3. Output ports — named, `set`/`flush`, no `return`

`out` lists named ports (flat — no groups), same spec grammar as inputs (a spec on an output is a
**post-condition**):

```
out (q Num) (r Num (min 0))
```

Emission replaces `return` with two forms:

```
(set r x)                 ; stage output port r (does not emit)
(flush)                   ; emit a tuple from whatever set staged
(flush (r x) (q y))       ; shorthand: stage these + emit in one form   ← common case
```

- `(return v)` is **removed** (the engine reports a clear error pointing here); a single-output
  body writes `(flush (r v))` (or `(set r v)` + `(flush)`).
- **`each`/`yield` are kept** as the low-level "define your own operator" primitive (Experimental):
  `yield` emits into the enclosing `each`'s stream, `flush` emits the node's outputs. `filter`
  still uses them; `map` no longer needs them (it is the reactive lift `(f xs)`). Folding
  `yield` into `flush` would require reworking `each` around the node model — deferred
  *(proposal)*. (This corrects the earlier RN4 note that said `yield` was removed.)
- `(exit s)` and error propagation **stay** — the error/terminal path still needs a terminal;
  only the *value-returning* `return` goes away.
- **Atomic bodies:** an `engine`+`src` leaf keeps its single implicit output — the `src` return
  value binds to the sole `out` port and flushes once per input tuple. `set`/`flush` is a
  **composite** concern.
- A staged-but-unflushed frame at body end is **not** emitted (no implicit flush) — emission is
  always explicit.

## 4. Composition & wiring — the graph stays *derived*

Multiple named outputs need a way to wire *one* output of `f` into an input of `g` **without
drawing an edge by hand** (the core `docs/01` property). The answer is **destructuring in `let`**:

```
fn describe {
  in  (zip (a Num) (b Num))
  out (s Str)
  body
    (let (q r (divmod a b))       ; destructure divmod's named outputs q, r
      (flush (s (fmt "q={} r={}" q r))))   ; wire divmod.q, divmod.r onward by composition
}
```

- `(let (q r (f a b)) body)` binds the outputs of `f` positionally to `q`, `r` for use in `body`:
  the last element is the expression, the leading names bind its output tuple. A single-output
  call keeps the scalar form `(let (v (f a)) …)`.
- Wiring is still **composition**, not hand-drawn edges: the graph is derived from which bound name
  flows into which call. `divmod.q → describe.s`'s input is expressed by nesting, exactly as before
  — multi-output just adds the selector.

## 5. Execution model (what shipped)

`evalCall` now evaluates its arguments, then **dispatches** (`internal/funk/run.go` `invoke`):

- **Scalar fast path** — no argument drives firing: run the body once. This *is* the length-1
  firing (`docs/04 §3`, "the engine optimizes the length-1 case"): all inputs are present, so the
  node fires exactly once. A composite runs with its own output frame; the value is derived from
  the flushes (0 ⇒ body value / sink, 1 ⇒ that tuple, N ⇒ a finite stream).
- **Reactive path** (`firing.go` `callReactive`) — a Stream argument bound to a **scalar** input
  port drives firing. A goroutine reads the driving streams per policy (`fireZip` pairs them 1:1;
  `fireLatest` = combineLatest, seeding optional ports from their defaults), and per fired tuple
  runs the body (atomic → `Exec`; composite → `runComposite` with `flush` streaming to the output).
  Non-driving args — plain scalars and whole Streams passed to `Stream`/`List`/`Json`/`Any` ports —
  are sampled as constants each fire. The output is a live stream; `take`-style upstream
  cancellation (already real in `stream.go`) propagates via context.

**Which Stream arguments drive** (`scalarPort`): only ports typed `Num`/`Str`/`Bool`/`Time`/`Bytes`
— a single scalar element — trigger the per-item lift. `Stream`/`List`/`Json`/`Any`/untyped ports
consume the whole argument (so `map`/`window`/`sum` receive the stream/list intact). This is what
keeps the lift from mis-firing aggregate functions.

Not yet done: a full goroutine-per-node graph where *every* call (not just stream-driven ones) is a
concurrently-scheduled node — `P2`'s graph-level implicit parallelism. The scalar path is still a
synchronous inline eval. That is the next step *(proposal)*.

## 6. What broke (and was migrated)

- **All of `std/`** — every `(return X)` was rewritten to `(flush (<port> X))` (31 sites, 12
  files; paren-aware, layout preserved). `yield` was kept.
- `map` (`std/stream/operators.funk`) is now the reactive lift `body (f xs)`; `filter` still uses
  `each`/`yield` (conditional emit).
- The evaluator gained the output frame + firing dispatch (`run.go`, `firing.go`); `return` now
  errors with a pointer here (`run.go`, `typecheck.go`); `graph.go` classifies `flush` as a
  terminal and `set` as a form.
- **Not yet:** the artifact schema (`docs/04 §9`) carrying per-port specs and firing groups on
  `input` nodes, and `docs/04` prose still describes `return` — a follow-up pass *(proposal)*.

All Go tests, `go vet`, `funk check` (127 fns), and the inline `funk test` suite (26/26) are green
after the migration.

## 7. Worked examples

```
; atomic — unchanged shape, single implicit output
fn add {
  in  (zip (a Num) (b Num))
  out (r Num)
  engine builtin
  src  "num.add"
}

; multi-output composite (destructured by the caller with `let`)
fn divmod {
  in  (zip (a Num) (b Num))
  out (q Num) (r Num (min 0))
  body (flush (q (div a b)) (r (mod a b)))
}

; source — flush N times over a finite range
fn evens {
  in  (n Num)
  out (r Num)
  body (for-each (range 0 n) (i) (flush (r (double i))))
}

; the reactive lift — a scalar fn over a stream fires per item; `map` is just:
fn map {
  in  (xs Stream) (f Fn)
  out (r Stream)
  body (f xs)
}

; latest + default — y is optional, 1 until it first arrives
fn scale {
  in  (latest (x Num (min 0)) (y Num (default 1)))
  out (r Num)
  body (flush (r (mul x y)))
}
```

## 8. Decisions (status)

| # | Decision | Status |
|---|---|---|
| RN1 | Firing default = **`zip`**; groups `zip`/`latest`; ungrouped ⇒ implicit `zip`; groups compose. | **decided + shipped** |
| RN2 | Port spec syntax = prefix forms `(key value…)`, no `=`/commas. | **decided + shipped** |
| RN3 | Multi-output selection = **`let` destructuring** `(let (q r (f …)) …)`. | **decided + shipped** |
| RN4 | Emission = `set` (stage) + `flush` (emit); `(flush (r v) …)` shorthand; **`return` removed**; `exit`/error stay. **`yield` kept** (each's emit primitive). | **decided + shipped** (yield-removal deferred) |
| RN5 | **Drop `required`** — optional ⟺ has `(default)`. | **decided** (Bruno, "ok") **+ shipped** |
| RN6 | `latest` = `combineLatest` (fire on any, newest of others once required present). | shipped |
| RN7 | Input-spec violation **cancels the scope** (propagate-and-cancel, not clamp). | shipped |
| RN8 | Constraint vocabulary v1 = `min max default`. | shipped |
| RN9 | **Output post-conditions** — `min`/`max` on out-ports enforced on `flush`. | shipped |
| RN10 | **Connection type-check** — `check` flags a scalar out→in mismatch (lenient: silent on stream/list/Json/Any/unknown, so no false positives; catches e.g. Str→Num). | shipped |

**Still proposal / next:** `len`/`one-of`/`pattern` constraints · full goroutine-per-node
scheduling (graph-level `P2`) · fold `yield` into `flush` · `docs/04` prose + artifact-schema
update for specs/groups.

# funk — The Protocol

> Status: **draft — to validate.** The notation was co-designed and partly implemented
> already; this restates it as a specification and folds in the streams-first and module
> decisions. New or unsettled syntax is marked **(proposal)**. The notation is the invention —
> read this critically, section by section.

## 1. What `.funk` is

A small, **homoiconic, typed** notation for defining workflows. You write code; the **graph is
derived** from it — you never draw nodes or edges by hand. **Everything is a stream.**

*Homoiconic:* code is data. A `.funk` program is a sequence of brace-blocks whose expressions
are prefix forms — the same structure the agent writes is the structure it reads, introspects,
and rewrites.

## 2. Surface syntax

**Top-level blocks:**
```
package "…" { … }     ; the module manifest (one per package)
type Name { … }       ; a named type / schema
fn name { … }         ; a function — atomic (src) or composite (body)
```

**Expressions are prefix forms:** `(head arg arg …)`. There is no infix; sugar desugars 1:1 to
forms.

- **Atoms:** identifiers (`foo`, `isEmpty?`), **addresses** (`funk/std/map`,
  `github.com/u/lib/fn`), numbers (`42`, `-3.14`), strings (`"…"`, with `\n` `\t` `\"` escapes;
  may span lines).
- **Comments:** `; to end of line`.
- **Fields inside a block:** `key value…` — e.g. `in (x Num)`, `engine python`, `src "…"`.

## 3. Types

Types are **streams**. `T` is shorthand for `Stream<T>` — **a value is a stream of length 1.**
One uniform model; the engine optimizes the length-1 case.

- **Base:** `Num`, `Str`, `Bool`, `Bytes`, `Time`, `List`, `Json` (dynamic), `Any` (unknown).
  `Time` is an instant — the event-time a `window … by <field>` reads (§7).
- **`Stream<T>` vs `List<T>`** — the two "sequences", kept distinct:
  - `Stream<T>` — items **over time** (may be infinite).
  - `List<T>` — **one** whole collection, held as a single value (finite, in memory).
  - The bridge: `(window s 5s)` turns a `Stream` into `List`s — each closed window is delivered
    as the list of that window.
- **Named types / schemas** — fields are `name Type`; **nesting** is by naming other types, and
  the builtin **generics** `List<T>` / `Stream<T>` apply. User-defined generics are **deferred**
  (v1 has no `type Foo<T>`).
  ```
  type User { name Str  age Num }
  type Repo { name Str  owner User  tags List<Str> }
  ```
- **Ports** (a function's in/out) carry types: `(x Num)`, `(items Stream<Json>)`.
- The compiler **type-checks** that a producer's output type is compatible with each consumer's
  input port. `Any` / `Json` are wildcards.
- **Both forms (decided):** `T` *is* a stream (length-1 for a plain value); write `Stream<T>`
  explicitly to emphasize a flowing / multi-item stream, and `List<T>` for a bounded collection
  held at once.

## 4. Functions — `fn`

**Everything is a function.** `fn` is the *only* definition block — there is no separate `flow`.
A function has a typed signature (`in` / `out`) and communicates as streams. It has one of two
natures, decided by **which field you fill** — `src` **or** `body`, never both:

- **atomic** — `src` + `engine`: a leaf that runs code in some runtime, per item.
- **composite** — `body`: a composition of other functions; the graph is *derived* from `body`.

A caller **cannot tell the two apart** — both present the same typed stream signature. That is
what makes composition uniform: a composite function is called exactly like an atomic one, and
functions import functions by address.

**Atomic:**
```
fn dbl {
  doc  "double a number"
  in   (n Num)
  out  (r Num)
  engine python
  requires (pip numpy)        ; optional
  src  "return n * 2"
}
```

**Composite:**
```
fn analyze {
  doc  "mean of the last 100, or exit if empty"
  in   (xs Stream<Num>)
  out  (r Num)
  body
    (if (isEmpty? xs)
      (exit "no data")
      (return (mean (window xs 100))))
}
```

**Fields:**
- `doc` — description.
- `in` / `out` — typed **ports** (streams); the function's signature.
- **atomic only:** `engine` (`python` | `go` | `claude` | `builtin` | …), `requires`
  (`(pip …) (go …) (os …)`), `src` (the body code).
- **composite only:** `body` — a single composing expression (§5).
- **resources / capabilities:** `needs` / `bind` and `effects` declare what the function
  receives and may touch (§10 and [`05-resources-and-integrations.md`](05-resources-and-integrations.md)).

**Exactly one of `src` / `body`.** A function with **neither** is a **signature-only
declaration** — an interface / slot to be bound (**proposal**; this is the `needs T` of doc 05).

**Calling convention (atomic).** An atomic body is a **stream processor**: it reads **NDJSON on
stdin** (one object per `onNext`) and writes **NDJSON on stdout** (one per emitted value). The
runtime **wraps the user's per-item body in the stream loop**, so simple bodies stay simple
(`return n * 2` runs once per item). Flavors follow from arity:

- **source** — `in ()`, `out (…)`: a generator (emits N, possibly forever).
- **transform** — per-item map.
- **sink / aggregate** — collapses a (windowed) stream to a value: reduce / collect.

A **composite** function needs no calling convention of its own — it *composes* the streams of
the functions it calls.

## 5. Composition — the `body`

A **composite** function's `body` is a **single expression** (usually a control form) that
composes calls to other functions. The **graph is derived** from it — you never draw nodes or
edges. From the `analyze` function above:

```
body
  (if (isEmpty? xs)
    (exit "no data")
    (return (mean (window xs 100))))
```

- `in` / `out` are the function's signature, exactly as for an atomic function.
- `body` is **one** expression; sequence with `do`, bind with `let`, branch / loop with the
  control forms (§6).
- **Every branch ends in a terminal** (`return` / `exit` / `break` / `continue`).
- Because the graph is *derived*, the compiler **never produces an edge that bypasses a
  condition** (§6, §9).

Nested composition is just calls: a composite function calls other functions — atomic or
composite — by address, and they compile in as subgraphs (§9).

## 6. Forms / constructs

- **Call** — `(f arg…)`, addressed `(pkg/f arg…)`, or aliased `(ml/fn arg…)`.
- **`let`** — `(let (name expr) body)`: bind a name (a stream) for reuse (fan-out).
- **`if`** — `(if cond then else)`: `cond` is a `Bool`; the taken branch runs, the other is
  **cancelled** (its scope's context). The condition gates the branch's *operation*, so no data
  edge ever bypasses the condition.
- **`for-each`** — `(for-each coll (item) body)`: run `body` per `onNext` of `coll` to
  **consume by effect** (returns nothing). Use `map` (in `funk/std`) when you want to
  *transform* and get a stream back; `for-each` is the sink-like *consume-for-effect*.
- **`while`** — `(while (s init) cond step)`: stateful loop; `break` / `continue` control the
  loop scope.
- **`do`** — `(do a b …)`: sequence; the last is the value.
- **Terminals** — `(return v)` forwards the function's output stream; `(exit s)` ends / errors
  the scope; `(break)` / `(continue)` control the enclosing loop. All map to completing or
  cancelling a `context`.

**Parallelism is implicit.** Independent `calls` — those that do not depend on each other's
output — run **concurrently** (each node is a goroutine). There is no `par` form: the graph
already says what is parallel.

**Errors: propagate-and-cancel.** An error is an `onError` that **cancels its scope**; it
propagates upward and surfaces in the `RunReport`.

**Recovery forms** *(implemented; Experimental)* — opt-in, so the default stays simple:

- **`(on-error <body> (e) <handler>)`** — run `body`; if it errors, bind the error message to
  `e` and run `handler` (fallback / substitute value). Loop signals (`break`/`continue`) are not
  caught.
- **`(retry <body> <n> [backoff <dur>])`** — re-run `body` up to `n` attempts, optional
  (cancellable) backoff between them; the error propagates only after the last attempt.

Both are ordinary forms that scope a `context`; without them, errors propagate-and-cancel. The
run's `RunReport` records a `recover` / `retry` event when they fire.

## 6a. Stability (what is stable today)

The spec runs ahead of the engine; this table is the contract for the **reference CLI**. See
[`docs/ROADMAP.md`](ROADMAP.md) for the milestones.

- **Stable** — tested; will not break without a major bump.
- **Experimental** — implemented, shape may still change; pin your funk version.
- **Proposal** — in this spec, **not yet implemented**; writing it will not run.

| Construct | Status |
|---|---|
| `fn` (atomic `engine`+`src` / composite `body`), typed `in`/`out` | **Stable** |
| Engines `builtin` · `python` · `go` · `claude` · `codex` | **Stable** |
| `do` · `let` · `if` · `return` · `exit` | **Stable** |
| `while` · `break` · `continue` · `for-each` | **Stable** |
| Sources `range` · `nats` · `tick` · `repeat` | **Stable** |
| Operators `map` · `filter` · `take` · `scan` · `merge` · `collect` | **Stable** |
| `window` — count + event-time (`by <field>`) | **Stable** |
| `window … every <slide>` — sliding **count** windows | **Experimental** |
| `each` / `yield` (define your own operators) · `fold` | **Experimental** |
| Recovery — `(on-error …)` · `(retry …)` | **Experimental** |
| `needs` / `effects` blocks; injection via `--bind` / env; trace redaction | **Experimental** |
| `funk get` (clone a package by URL, resolve by bare name) | **Experimental** |
| `window` extras — `every` on **event-time** · `lateness` · `on-late` | **Proposal** |
| `use "<addr>" <semver> as <alias>` (namespaced/versioned resolution) | **Proposal** |
| Secret **broker** / **egress** control; `with { … }` / project `object` | **Proposal** |

## 7. Streams — the reactive core

- Every edge carries a **stream**: zero or more `onNext`, then `onComplete` or `onError`.
- **A value is a stream of length 1.**
- **Infinite streams** are allowed — a source may never complete; the run is then a **live
  pipeline** that runs until stopped.
- **Aggregation over an unbounded stream requires windowing** — you cannot `collect` infinity.

**Event-time (decided).** Windows are cut by each event's own timestamp, not by arrival:

- **timestamp source** — each item carries a `time` field; if absent, the engine stamps arrival
  time (that stream falls back to processing-time). `by <field>` selects the field:
  `(window xs 5s by time)`.
- **watermark** — a window closes when the largest event-time seen passes `window end +
  lateness` (a small tolerance; default short). Plainly: *"I've seen everything up to T, close."*
- **late-data** — an event arriving after its window closed: **default drop**, **configurable
  per window** via `on-late` (re-emit / update, or a wider `lateness`).

**`window` — the full form** *(proposal)*:

```
(window <stream> <size> [every <slide>] [by <field>] [lateness <dur>] [on-late <policy>])
```

- **`<size>`** — a bare number is a **count** (`100`); a duration (`5s`) is **event-time**.
- **`every <slide>`** — optional; makes it **sliding**. Omitted ⇒ **tumbling** (slide = size).
- **`by <field>`** — the event-time field; omitted ⇒ the conventional `time` field, else arrival.
- **`lateness <dur>`** — tolerance before a window closes; default `0s`.
- **`on-late <policy>`** — `drop` (default) · `emit` (re-emit the updated window) · `(sink f)`
  (side-output late items to `f`).

Durations: `ms` `s` `m` `h` `d`.

```
(window xs 100)                                              ; 100 items (count)
(window clicks 1m by time)                                   ; tumbling 1-min event-time
(window clicks 5m every 1m by time lateness 30s on-late drop) ; sliding, 30s tolerance
```

**Operators.** `map` / `filter` / `take` / `merge` / `zip` / `debounce` … live in `funk/std` as
functions. **`window` and the aggregations are core** — the engine must manage event-time +
watermarks, so they cannot be pure `.funk`.

## 8. Packages & addressing

- A **package is a git repo**; a reference is an **address** (`/`-path), **local or web**:
  `funk/std/map`, `github.com/user/lib/fn`, `./localFn`. A function *is* an address; a composite
  function is a composition of addresses.
- **Published vs local.** A function is **published** when it lives in a package with a
  resolvable **address + version** (a git repo); **local** is your working project, not yet
  published. Publishing = giving it an address. This is the distinction §9's linking rule keys
  on (**local → inline**, **published → address**).
- **Import with alias:** `use "github.com/user/lib" v1.2.0 as ml` → `(ml/fn …)`.
- **Manifest** — `.funk` itself (self-hosting). Grammar *(proposal)*:
  ```
  package "<address>" {
    version <semver>                              ; this package's version
    use "<address>" v<semver> [as <alias>]        ; a dependency — repeatable
  }
  ```
  Example:
  ```
  package "github.com/user/proj" {
    version 0.1.0
    use "funk/std" v2.1.0
    use "github.com/user/lib" v1.2.0 as ml
  }
  ```
  The manifest holds **only deps** (portable, committed). Project **`object`** and **`bind`**
  blocks (doc 05) are **environment config**, not the package definition — they carry
  environment-specific values (vault refs, which concrete objects) and live in a separate
  per-environment overlay, selected with `funk run --env <name>` / `--bind` *(proposal)*.
- **Resolution:** `funk/std/*` → local project functions → imported packages.
- Versions: **semver + git tags**, **content-hash locked** (a re-pointed tag fails). Cache:
  `~/.funk/pkg/<path>@<version>/`.
- **Signing & verification.** The content-hash gives **integrity**; a package may also carry an
  author / registry **signature**, and a **verified** state (**authenticity**). Per the `03`
  isolation decision, **verified** packages run inside the daemon container, **unsigned /
  unverified** ones in their own — the trust tier keys on this.

## 9. The artifact (compiled form)

`.funk` compiles to a **portable artifact**: a `graph` *derived* from the code (never drawn by
hand) — `nodes` (the functions) joined by `edges` (the data `streams` between them).

- **`nodes`** — each typed by its role:
  - `function` — a call to a function. **Atomic:** carries its code (`engine`, `src`,
    `requires`). **Composite:** carries the functions it **calls** — **inlined** when they are
    local (unpublished, no resolvable address), or as an **address** (`funk/std/…@ver` + hash)
    when published.
  - `condition` — from `if`; gates its branches.
  - `loop` — from `while` / `for-each`.
  - `terminal` — `return` / `exit` / `break` / `continue`.
  - `input` — the function's `in` ports; the entry.
- **`edges`** — **data** (typed `streams`) and **control** (gating). No data edge bypasses a
  condition, because the graph is *derived*, not drawn.
- **resources & capabilities** — each node carries its declared `needs` / `effects` (doc 05), so
  `introspect` **aggregates** what the whole artifact requires (secrets, integrations, network)
  — for provisioning and audit.
- **signature** — the `in` / `out` of the function it was compiled from.

**Linking rule (how a called function is stored): local → inline** (there is no address to
resolve elsewhere), **published → address + hash** (the target fetches it). `funk build
--vendor` inlines everything, even published functions, to seal a 100% self-contained artifact
for export.

It is JSON, schema-defined (the `artifact` schema), and it is the single thing the engine runs,
`introspect` reads, and the server accepts. Because atomic and composite functions present the
same signature, composition is uniform — a composite is called exactly like an atomic one.

## 10. Requirements, resources & capabilities

Three declarations, distinct on purpose:

- **`requires`** — **build dependencies** (`pip` / `go` / `os`), installed into the (Docker)
  engine. → *what must exist for the code to run.*
- **`needs` / `bind`** — **runtime resources received**: `secrets` / `configs` / `volumes` /
  `env` and integration slots. Injected by the engine, least-privilege, never threaded through
  callers. → *what the function is given.* Full model in
  [`05-resources-and-integrations.md`](05-resources-and-integrations.md).
- **`effects`** — **capabilities permitted**: the network endpoints a function may reach
  (egress) and filesystem access, enforced by what the sandbox is permitted to see. The network
  capability is what makes brokering + egress (doc 05) hold. → *what the function may do.* This
  is the basis for capability-security (ambition #3).

In short: `requires` = what exists · `needs` = what it receives · `effects` = what it may do.

**`effects {}` syntax** *(proposal)* — one line per capability, `<kind> <value…>`:

```
fn scrape {
  effects {
    net  api.github.com          ; an allowed egress host
    net  *.googleapis.com
    fs   read   /data            ; filesystem access, scoped
    fs   write  /tmp/out
  }
  ...
}
```

- **`net <host>`** — an egress host the sandbox will allow (glob ok). Anything else is blocked.
- **`fs read|write <path>`** — scoped filesystem access.
- **Integrations auto-declare their egress.** A function with `needs { github a }` inherits
  `github`'s endpoints from the integration type — you only list `effects` for raw `net` / `fs`
  the function does **itself**. Least surprise, and the integration owns its own reach.

The principle is fixed — **declared, injected, least-privilege, enforced.** `needs` / `bind`
syntax is settled in doc 05; `effects` grammar above is a proposal.

## Open (for this spec)

Everything below now has a **concrete proposal in-line** (marked *(proposal)*), pending
Bruno's validation — none is a blank anymore:

- `window` full grammar — §7 *(proposal)*.
- `effects {}` grammar — §10 *(proposal)*.
- Error recovery (`on-error` / `retry`) — §6 *(proposal)*.
- `package` manifest grammar + where `object` / `bind` live — §8 *(proposal)*.
- `Stream<T>` / `List<T>` / `T` forms — §3 *(decided)*; user-defined generics deferred.
- `needs` / `bind` syntax — settled in doc 05.

Still genuinely undecided (need Bruno): the per-call `with { … }` and project `object` block
final grammar (doc 05), and the vault / broker-proxy interface (doc 05).

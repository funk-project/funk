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

- **Base:** `Num`, `Str`, `Bool`, `Bytes`, `List`, `Json` (dynamic), `Any` (unknown).
- **Named types / schemas:**
  ```
  type User { name Str  age Num }
  ```
- **Ports** (a function's in/out) carry types: `(x Num)`, `(items Stream<Json>)`.
- The compiler **type-checks** that a producer's output type is compatible with each consumer's
  input port. `Any` / `Json` are wildcards.
- **(proposal)** explicit `Stream<T>` when you want to be precise; otherwise `T` is a stream.

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
- **`let`** — `(let (name expr) body)`: bind a name (a stream) for reuse.
- **`if`** — `(if cond then else)`: `cond` is a `Bool`; the taken branch runs, the other is
  **cancelled** (its scope's context). The condition gates the branch's *operation*, so no data
  edge ever bypasses the condition.
- **`for-each`** — `(for-each coll (item) body)`: run `body` per `onNext` of `coll`.
- **`while`** — `(while (s init) cond step)`: stateful loop; `break` / `continue` control the
  loop scope.
- **`do`** — `(do a b …)`: sequence; the last is the value.
- **Terminals** — `(return v)` forwards the function's output stream; `(exit s)` ends / errors
  the scope; `(break)` / `(continue)` control the enclosing loop. All map to completing or
  cancelling a `context`.

## 7. Streams — the reactive core

- Every edge carries a **stream**: zero or more `onNext`, then `onComplete` or `onError`.
- **A value is a stream of length 1.**
- **Infinite streams** are allowed — a source may never complete; the run is then a **live
  pipeline** that runs until stopped.
- **Aggregation over an unbounded stream requires windowing** — you cannot `collect` infinity.
  `(window s 100)` (count) / `(window s 5s)` (time) **(proposal)**. The time model is
  **event-time** (decided): windows are cut by the event's own timestamp, requiring watermarks
  and a late-data policy. See `03 — Decisions`.
- **Operators** (`map`, `filter`, `take`, `merge`, `zip`, `window`, `debounce`, …) live in
  `funk/std` as functions; a few of the most primitive may be core forms.

## 8. Packages & addressing

- A **package is a git repo**; a reference is an **address** (`/`-path), **local or web**:
  `funk/std/map`, `github.com/user/lib/fn`, `./localFn`. A function *is* an address; a composite
  function is a composition of addresses.
- **Import with alias:** `use "github.com/user/lib" v1.2.0 as ml` → `(ml/fn …)`.
- **Manifest** — `.funk` itself (self-hosting):
  ```
  package "github.com/user/proj" {
    version 0.1.0
    use "funk/std" v2.1.0
    use "github.com/user/lib" v1.2.0 as ml
  }
  ```
- **Resolution:** `funk/std/*` → local project functions → imported packages.
- Versions: **semver + git tags**, **content-hash locked** (a re-pointed tag fails). Cache:
  `~/.funk/pkg/<path>@<version>/`.

## 9. The artifact (compiled form)

`.funk` compiles to a **portable artifact** — a graph *derived* from the code:

- **nodes** — typed: `function` / `condition` / `loop` / `terminal` / `input`; each with
  engine, `src`, `requires`.
- **edges** — **data** (streams) and **control** (gating).
- **subfunctions** — called composite functions, compiled in as subgraphs.
- the function's **signature** (in / out).

It is JSON, schema-defined (the `artifact` schema), and it is what the engine runs, what
`introspect` reads, and what the server accepts. Because the graph is *derived*, the compiler
never produces an edge that bypasses a condition.

## 10. Effects & requirements

- **`requires`** — build/runtime dependencies (`pip` / `go` / `os`), installed into the
  (Docker) engine.
- **`effects` (proposal)** — declared capabilities: which resources (network / fs / secrets) a
  function may touch. The engine enforces them via what the sandbox is permitted to see. This
  is the basis for capability-security (ambition #3). Syntax open.
- **Resources & integrations** — how a function receives `env` / `volumes` / `secrets` /
  `configs`, how integrations declare and inject them, and how secrets are protected
  (brokering + egress) are specified in [`05-resources-and-integrations.md`](05-resources-and-integrations.md).

## Open (for this spec)

- `Stream<T>` explicit syntax vs `T`-is-a-stream shorthand (or both).
- Windowing syntax + the time model (event-time vs processing-time).
- `effects` syntax.
- Which stream operators are **core forms** vs purely `funk/std`.
- The exact `type` / schema language (nesting, generics?).
- The `package` manifest exact grammar.

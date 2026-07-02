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
fn name { … }         ; a function
flow name { … }       ; a workflow
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
- **Ports** (function / flow in/out) carry types: `(x Num)`, `(items Stream<Json>)`.
- The compiler **type-checks** that a producer's output type is compatible with each consumer's
  input port. `Any` / `Json` are wildcards.
- **(proposal)** explicit `Stream<T>` when you want to be precise; otherwise `T` is a stream.

## 4. Functions — `fn`

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

Fields: `doc`, `in` (ports), `out` (ports), `engine` (`python` | `go` | `claude` | `builtin` |
…), `requires` (`(pip …) (go …) (os …)`), `src` (the body), `effects` **(proposal)**.

**Calling convention** — a function body is a **stream processor**: it reads **NDJSON on
stdin** (one object per `onNext`) and writes **NDJSON on stdout** (one per emitted value). The
runtime **wraps the user's per-item body in the stream loop**, so simple bodies stay simple
(`return n * 2` runs once per item). Flavors follow from arity:

- **source** — `in ()`, `out (…)`: a generator (emits N, possibly forever).
- **transform** — per-item map.
- **sink / aggregate** — collapses a (windowed) stream to a value: reduce / collect.

## 5. Flows — `flow`

A workflow: a composition of calls; the graph is **derived** from `body`.
```
flow analyze {
  in   (xs Stream<Num>)
  out  (r Num)
  body
    (if (isEmpty? xs)
      (exit "no data")
      (return (mean (window xs 100))))
}
```

`in` / `out` declare the flow's signature. `body` is a single expression (usually a control
form). Every branch ends in a terminal.

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
- **Terminals** — `(return v)` forwards the flow's output stream; `(exit s)` ends / errors the
  scope; `(break)` / `(continue)` control the enclosing loop. All map to completing or
  cancelling a `context`.

## 7. Streams — the reactive core

- Every edge carries a **stream**: zero or more `onNext`, then `onComplete` or `onError`.
- **A value is a stream of length 1.**
- **Infinite streams** are allowed — a source may never complete; the flow is then a **live
  pipeline** that runs until stopped.
- **Aggregation over an unbounded stream requires windowing** — you cannot `collect` infinity.
  `(window s 100)` (count) / `(window s 5s)` (time) **(proposal)**. The time model (event-time
  vs processing-time) is an open question.
- **Operators** (`map`, `filter`, `take`, `merge`, `zip`, `window`, `debounce`, …) live in
  `funk/std` as functions; a few of the most primitive may be core forms.

## 8. Packages & addressing

- A **package is a git repo**; a reference is an **address** (`/`-path), **local or web**:
  `funk/std/map`, `github.com/user/lib/fn`, `./localFn`. A function *is* an address; a workflow
  is a composition of addresses.
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
- **subflows** — called flows, compiled in.
- the flow's **signature** (in / out).

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

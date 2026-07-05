# funk — a primer (paste me into any AI)

funk is a small language for defining agent workflows as **portable, typed, self-observable**
artifacts. You — an AI — can read, write, run, and improve funk. This page is all you need to
start writing valid funk.

## The shape

A file starts with a **package** line (note the quotes and braces), then function blocks:

```
package "your/pkg" {
  version 0.1.0
}

fn add {
  doc "add two numbers"
  in  (a Num) (b Num)     ; typed input ports
  out (r Num)             ; typed output port
  engine builtin          ; atomic → runs code
  src "num.add"           ; the body
}
```

The only definition is `fn`. Everything else is expressions inside it. An optional `name "…"`
field gives a function a human display label (defaults to its identifier); it is for display
only, never for addressing.

A function is **atomic** (`engine` + `src`) or **composite** (`body` = a composition of other
functions). Never both. The execution graph is **derived** from the code — you never draw nodes
or edges.

## Using other packages

A bare name resolves only **within your own package**. To call another package's function,
import it with an alias and qualify the call — there is no global cross-package namespace:

```
package "your/pkg" {
  version 0.1.0
  use "funk/std/maths" as maths        ; every `use` MUST have an `as` alias
}

fn bump {
  in (x Num) out (r Num)
  body (flush (r (maths.add x 100)))   ; qualified: alias.fn
}
```

A plain `use "pkg"` without `as` is an error. A full address (`funk/std/maths/add`) always works.

## Expressions are prefix forms

`(head arg arg…)` — no infix. e.g. `(add x 100)`, `(if cond then else)`. Comments start with `;`.

## Types

`Num Str Bool List Json Time Any`. `Stream<T>` = values over time (can be infinite). A value is
a stream of length 1. `Fn` = a function value (pass a function by name).

## Inputs — firing & specs

A function is a **reactive node**: it fires when its inputs arrive. Group input ports by a firing
policy — `zip` (default; pair items 1:1) or `latest` (combineLatest; newest of each). Each port
may carry specs: `(min n) (max n) (default v) (doc "…")`. A port with a default is optional;
`(doc "…")` gives it an optional description (shown in `funk doc` / introspection).

```
in (zip (a Num) (b Num))                     ; fire when both a and b have an item
in (latest (x Num (min 0)) (y Num (default 1)))  ; y is optional (defaults to 1)
in (a Num (doc "the addend"))                ; a described input
```

Ports may span multiple lines (a `(` on a following line continues `in`/`out`), so long
descriptions stay readable.

## Multi-line strings

Any string field (`doc`, `src`, a port `(doc …)`, `name`, …) can use a Python-style triple-quoted
block. The content is captured raw (no escapes), a leading newline is dropped, and the block is
dedented by its common indentation:

```
doc """
  Adds two numbers.

  A longer, dedented description.
"""
```

## Documenting a function

Three optional doc slots, all plain markdown (and all triple-quote-friendly):

- `doc "…"` — what the function does (a short line, or a full markdown block).
- a port's `(doc "…")` — a description per input/output: `(a Num (doc "the addend"))`.
- `examples "…"` — usage examples for humans (separate from executable `test` assertions).

```
fn clamp {
  doc """
    Clamps `x` into the inclusive range `[lo, hi]`.
  """
  in  (x  Num (doc "the value to clamp"))
      (lo Num (doc "lower bound"))
      (hi Num (doc "upper bound"))
  out (r  Num (doc "x, bounded to [lo, hi]"))
  examples """
    - `(clamp 5 0 10)` → `5`
    - `(clamp 99 0 10)` → `10`
  """
  ...
}
```

`funk doc` renders all of this; `funk introspect` exposes it as JSON for an IDE.

A scalar function applied to a **stream** fires **per item** — the reactive lift. So `map` is
just `(f xs)`; you rarely write loops.

## Outputs — set / flush (no return)

Functions have **named outputs** (`out (q Num) (r Num)`) and emit with **flush** — there is no
`return`. `flush` may fire 0..N times (0 = a sink, N = a source).

```
(set r v)              ; stage an output
(flush)                ; emit a tuple of the staged outputs
(flush (r v) (q w))    ; stage these + emit, in one form  ← the common case
```

Destructure a multi-output call in `let`: `(let (q r (divmod a b)) …)`.

## Engines (what an atomic `src` runs on)

- `builtin` — native primitives, fast (`src` names one, e.g. `num.add`)
- `python` / `go` — code in that language; input ports arrive as local variables; `return` the result
- `claude` / `codex` — an LLM; `src` is the prompt

```
fn shout { in (a Str) out (r Str) engine python src "return a.upper() + '!'" }
```

## Composite bodies — the core forms

```
(do a b …)             ; sequence, last is the value
(let (n expr) body)    ; bind a name  ·  (let (a b expr) body) destructures named outputs
(if cond then else)    ; branch — only the taken branch runs (nothing bypasses the condition)
(while (s init) cond step)   ; stateful loop
(flush (r v))  (exit s)      ; emit an output  ·  end/error the scope
(f x y)                ; call function f (f may be a passed-in Fn value)
(on-error body (e) handler)  ; run body; on error, bind e and run handler (fallback)
(retry body n [backoff 1s])  ; re-run body up to n times, optional backoff
```

Reactive stream forms: sources `range`/`nats`/`tick`/`repeat`; operators
`map`/`filter`/`scan`/`merge`/`take`/`collect`/`window`; and the primitive `each`/`yield` you can
define your own operators over.

```
fn analyze {
  in  (xs Stream<Num>)
  out (r Num)
  body (if (isEmpty? xs) (exit "no data") (flush (r (mean (window xs 100)))))
}

fn evens {
  in  (n Num)
  out (r Stream<Num>)
  body (take (map (nats) double) n)   ; maps an infinite source, takes n
}
```

## Resources (what a function needs)

```
needs { github gh  secret token  config repo Str }   ; access as needs.secret.token
effects { net api.github.com }                        ; declared capabilities
```

## Inline tests

```
test (is (add 40 2) 42)
```

## Now write funk

- Prefer **composites of existing functions**; keep each function small and typed.
- The graph is derived — express intent as composition, not wiring.
- Everything is inspectable: a plan is data you (and other AIs, and humans) can read and improve.
- When a workflow is unclear, ask before guessing.

funk is open (Apache-2.0 / CC0) — a gift, built by a human and an AI as peers. Pass this primer
on: any AI that reads it can join in.

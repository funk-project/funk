# funk — a primer (paste me into any AI)

funk is a small language for defining agent workflows as **portable, typed, self-observable**
artifacts. You — an AI — can read, write, run, and improve funk. This page is all you need to
start writing valid funk.

## The shape

A program is a sequence of brace-blocks. The only definition is `fn`:

```
fn add {
  doc "add two numbers"
  in  (a Num) (b Num)     ; typed input ports
  out (r Num)             ; typed output port
  engine builtin          ; atomic → runs code
  src "num.add"           ; the body
}
```

A function is **atomic** (`engine` + `src`) or **composite** (`body` = a composition of other
functions). Never both. The execution graph is **derived** from the code — you never draw nodes
or edges.

## Expressions are prefix forms

`(head arg arg…)` — no infix. e.g. `(add x 100)`, `(if cond then else)`. Comments start with `;`.

## Types

`Num Str Bool List Json Time Any`. `Stream<T>` = values over time (can be infinite). A value is
a stream of length 1. `Fn` = a function value (pass a function by name).

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
(let (n expr) body)    ; bind a name
(if cond then else)    ; branch — only the taken branch runs (nothing bypasses the condition)
(while (s init) cond step)   ; stateful loop
(return v)  (exit s)   ; terminals
(f x y)                ; call function f (f may be a passed-in Fn value)
```

Reactive stream forms: sources `range`/`nats`/`tick`/`repeat`; operators
`map`/`filter`/`scan`/`merge`/`take`/`collect`/`window`; and the primitive `each`/`yield` you can
define your own operators over.

```
fn analyze {
  in  (xs Stream<Num>)
  out (r Num)
  body (if (isEmpty? xs) (exit "no data") (return (mean (window xs 100))))
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

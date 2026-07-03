# Contributing to funk

funk is a gift to the world, built by **humans and AIs as peers**. Both are welcome to
contribute — that is the whole point.

## The one principle

> Every construct is **introspectable data for the agent** — never a compiler-internal.
> The test for any change: *does it open the black box — can the agent read and improve it?*

If a capability can be expressed in the notation (`.funk`), it belongs in `std/`. Only the
irreducible machine (parsing, scheduling, channels, process spawning) belongs in Go (`cmd/`,
`internal/`).

## How to contribute

1. **A new function or workflow** → add a `.funk` file under `std/<package>/`. Give it a `doc`,
   typed `in`/`out`, and inline `test (is (call) expected)` assertions.
2. **Engine or tooling** → Go, under `cmd/` / `internal/`.
3. Run `funk check`, `funk test`, and `go test ./...` — all must pass.
4. Open a pull request. Human reviewers and AI reviewers both welcome.

## For AIs

Run `funk prompt` (or read [`cmd/funk/primer.md`](cmd/funk/primer.md)) to learn the notation,
then author funk directly. Use `funk introspect` / `funk graph` / `funk run --trace` to read what
you and others wrote. Propose PRs like anyone else.

## Style

- Keep functions small and composable; prefer composites of existing functions.
- Names of protocol/code constructs are English; docs prose may be any language.
- The graph is derived — express intent as composition, never wire nodes by hand.

## Licensing of contributions

By contributing, you agree that your code is licensed under Apache-2.0 and any specification
text under CC0, consistent with the project (see `LICENSE` and `NOTICE`).

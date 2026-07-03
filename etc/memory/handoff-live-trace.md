# Handoff — funkd live trace (branch `feat/funkd-live-trace`)

**Date:** 2026-07-03 · **Author:** Claude (with Bruno) · **Log entry:** I29

## Why
Bruno is starting a **funk-IDE** (Electron IDE = agent-runner shell + the funktions visual,
driven entirely by the Go CLI — no TS reimplementation of the language). The IDE's headline
view is an **animated, self-observable trace**: nodes glow when they run, turn green with their
value, branches light the taken side. For that the runtime has to emit **per-node events live**,
which it did not — `RunWithReport`/`serve.go` only returned the whole `RunReport` at the end.

## What changed (branch `feat/funkd-live-trace`, off `main`)
- `internal/funk/run.go`
  - New **`RunLive(lib, ref, inputs, opts, onEvent, onValue)`** — streams trace events *and*
    stream values as they happen, returns the final `RunReport`.
  - `evalTop` now takes an optional `sink func(TraceEvent)` (3 existing callers pass `nil`).
  - `emit()` delivers to the sink **and** accumulates the batch; new `live()` helper delivers
    **animation-only** signals.
  - A node emits an **`enter`** event (glow, *before* it runs) in `evalCall` + the atomic
    top-level. **`enter` is live-only — never appended to `RunReport.Events`.**
- `cmd/funk/serve.go` — `/run` accepts `"live":true` → `{"event":…}` per node, `{"value":…}`
  per item, `{"report":…}` at the end.
- `internal/funk/funk_test.go` — `TestRunLiveStreamsEvents`.

## Invariant to preserve
The **batch `RunReport` shape is unchanged** (no `enter` events in it) — that is why every
existing trace test still passes. If you add event kinds, keep new *animation* signals on the
`live()` (sink-only) path, not in `RunReport.Events`, unless you intend to change the report
contract.

## Verified
`go build` / `go vet` / `go test ./...` green. Live over the wire:
`bump x=3` → `enter gt` → `call gt=false` → `branch else` → `terminal return` → `value 3` → `report`.
`squares n=3` → per-item `enter`/`call square` interleaved with streamed `value`s.

## Next step (camada 3 — not done)
Events identify a node only by `fn` name. For the IDE to map an event to the **exact** graph
node under repeated calls, add a **stable node id shared** between the graph the IDE draws and
these events. Cleanest path: a `funk graph --json` that emits nodes with ids derived from the
AST `Pos` (`line:col`, already on every node), and have `TraceEvent` carry that same id.

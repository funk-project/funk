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

## Camera 3 — DONE (I30)
Stable node ids now link the graph and the trace:
- `TraceEvent` carries **`node`** — the call site's AST `Pos` (`"line:col"`), set on
  `enter` / `call` / `branch` / `terminal` events (in `evalCall`, `evalIf`, and the
  `return`/`exit` cases).
- `funk graph --json` emits the tree with the **same ids** per node (`id: "line:col"`, from
  `Pos`), plus `kind`/`head`/`value`/`children`.
- **Verified:** for `bump`, `funk graph --json` gives `gt`=`24:10`, `if`=`24:6`, else-`return`=
  `26:8`; the `--trace` events carry exactly those ids. Go test `TestTraceEventCarriesNodeID`
  asserts the event id equals the call-site `Pos`.

So the IDE can: draw from `graph --json`, then on each live event light the node whose `id`
matches — the exact node, even under repeated calls (the call site is stable).

## Next step (camera 4 — not done)
The IDE itself: consume `graph --json` + the `/run {"live":true}` stream and animate. On the
runtime side, the open question is per-**item** granularity for streams (a `map` firing `square`
N times reuses one call-site id — fine for "this node is active", but if the IDE wants to show
*which item*, events would need an item index alongside the node id).

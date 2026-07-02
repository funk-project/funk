# funk — Decisions Log

The record of decisions taken while designing funk. **Status** is either:

- **DECIDED (Bruno)** — Bruno chose it explicitly.
- **PROPOSAL (assistant)** — the assistant drafted it autonomously while Bruno was away; it is
  **vetoable** — read and confirm or change. Marked *(proposal)* in the docs too.

Dates are absolute.

---

## Architecture (`docs/03`)

| # | Decision | Status |
|---|---|---|
| A1 | **Windowing time-model = event-time** — windows cut by the event's own timestamp; deterministic replay. Commits us to event timestamps, watermarks, a late-data policy. | DECIDED (Bruno) |
| A2 | **`run` / Ctrl-C = detach (uniform)** — `funk run` streams in the foreground; Ctrl-C detaches the CLI, the pipeline keeps running on the server; `--stop-on-exit` to kill on exit. | DECIDED (Bruno) |
| A3 | **Isolation = daemon-in-container + trust tier** — verified code (std, local, signed+verified pkgs) runs in the daemon container; unsigned/unverified packages get their own container. Needs a package signature + verified state. | DECIDED (Bruno) |
| A4 | **Hot-mutation = immutable runs** — observe → test-in-isolation → stop/edit/re-run. Drain-and-switch noted as the future continuity path for infinite pipelines. | DECIDED (Bruno) |
| A5 | **Partial / scoped execution** — run a single function (input→output) or a mid-stream segment; the test/probe substrate for user and agent. | DECIDED (Bruno) |
| A6 | **One repo now** — `cmd/` (Go) + `std/` (.funk) together; `std/` graduates to its own repo later without the `funk/std/*` address changing. | DECIDED (Bruno) |

## Protocol (`docs/04`)

| # | Decision | Status |
|---|---|---|
| P1 | **Everything is a function** — one `fn` block; `flow` is gone. Atomic (`src`+`engine`) or composite (`body`), decided by which field is present (never both). Neither = a signature-only declaration (interface/slot). | DECIDED (Bruno) |
| P2 | **Parallelism is implicit** — independent calls run concurrently; no `par` form. | DECIDED (Bruno) |
| P3 | **`map` (transform) vs `for-each` (consume-by-effect)** — both kept, distinct intent. | DECIDED (Bruno) |
| P4 | **Errors propagate-and-cancel** by default. | DECIDED (Bruno) |
| P5 | **`Time` base type; `Stream<T>` (over time) vs `List<T>` (one value)**, bridged by `window`. | DECIDED (Bruno) |
| P6 | **Published vs local; signing & verification** (integrity via hash + authenticity via signature; verified ↔ isolation tier). | DECIDED (Bruno) |
| P7 | **Artifact linking rule: local → inline / published → address+hash**; `--vendor` seals a self-contained artifact. Nodes carry `needs`/`effects` so `introspect` aggregates. | DECIDED (Bruno) |
| P8 | **Three declarations: `requires` (exists) / `needs` (receives) / `effects` (may do).** | DECIDED (Bruno) |
| P9 | **`window` full grammar** — `(window s size [every slide] [by field] [lateness dur] [on-late policy])`; count vs duration; tumbling vs sliding; late-data drop/emit/sink. | PROPOSAL (assistant) |
| P10 | **`effects {}` grammar** — one line per capability (`net <host>`, `fs read|write <path>`); integrations auto-declare their egress. | PROPOSAL (assistant) |
| P11 | **Recovery forms** — `(on-error body (e) handler)` and `(retry body n [backoff dur])`, opt-in. | PROPOSAL (assistant) |
| P12 | **Manifest grammar** — `package "<addr>" { version …  use … }`; manifest holds deps only; `object`/`bind` are environment config in a separate overlay (`--env` / `--bind`). | PROPOSAL (assistant) |
| P13 | **Type/schema language** — nesting by naming types; builtin generics `List<T>`/`Stream<T>`; user-defined generics deferred. | PROPOSAL (assistant) |

## Resources & integrations (`docs/05`)

| # | Decision | Status |
|---|---|---|
| R1 | **Two axes: data (streams) vs resources (env/volumes/secrets/configs).** | DECIDED (Bruno) |
| R2 | **`config` may live in the artifact; `secret` never** — a vault reference resolved at runtime. | DECIDED (Bruno) |
| R3 | **Integrations: type → object → reference**; a type, many objects. | DECIDED (Bruno) |
| R4 | **Least-privilege** — a function gets only the resources it declares. | DECIDED (Bruno) |
| R5 | **Binding ≠ Injection** — the value injects at the leaf (from the vault); only the reference (which object) flows through composition. Secrets are never threaded parent→child. | DECIDED (Bruno) |
| R6 | **Slots (DI)** — a function declares an abstract slot; run/parent binds a concrete object. | DECIDED (Bruno) |
| R7 | **Protecting secrets** — brokering (default for integrations; the function never sees the token) + egress control (network first-class) + short-lived tokens + redaction (defense-in-depth). | DECIDED (Bruno) |
| R8 | **`needs {}` block syntax** — one line per resource (`<kind> <alias> [schema]`); access `needs.<kind>.<alias>`; aliases allow several of a kind; `bind {}` / per-call `with {}`. | DECIDED (Bruno) |

## Project / repo

| # | Decision | Status |
|---|---|---|
| G1 | **Fully open source** — Apache-2.0 (code) + CC0 (spec). A gift/invention/paper, not a product. | DECIDED (Bruno) |
| G2 | **GitHub: `funk-project/funk`, private for now** (created 2026-07-02). `LICENSE` to be added when opened. | DECIDED (Bruno) |
| G3 | **funk is Go** (reactive engine + server); engines (python/go/claude/builtin) stay — any NDJSON-speaking runtime qualifies. | DECIDED (Bruno) |

## Implementation — the Go CLI (build phase, 2026-07-02)

Bruno directed the move from design-first to **build** ("trabalhes até teres um working version
do cli"). Ported from the TS reference in the `functions` repo; funk is Go from here.

| # | Decision | Status |
|---|---|---|
| I1 | **Layout:** `cmd/funk` (CLI) + `internal/funk` (parser, model, engine, typecheck) + `std/` (.funk stdlib) + `memory/`. | Built |
| I2 | **`builtin` engine is native Go**, not the trivial reference stub — a primitive registry (`num.*`, `bool.*`, `str.*`, `list.*`, `sys.*`) so core ops are fast with no subprocess. The kernel shrinks; everything else composes or uses python. | Built |
| I3 | **Engines:** `builtin`, `python`, `go` (subprocess, NDJSON calling convention), `claude` (`claude -p --model sonnet`), `codex` (`codex exec`). LLM default = claude (Bruno). | Built |
| I4 | **Composite executor is tree-eval** — only the taken `if` branch is evaluated, so a condition gates its branch **by construction** (the old bypass bug cannot occur). | Built |
| I5 | **Skills as funk** (`std/skills`, claude engine): architect / programmer / reviewer / tester / reflect + `generate`. `funk make "<task>"` runs architect→programmer→check→reflect and adds the result to `std/generated`. **Verified live**: generated `reverse_words`, checked, ran → correct. | Built |
| I6 | **stdlib** (~90 fns, 12 packages): maths, compare, logic, strings, text, collections, mathx, stats, encoding, datetime, examples, skills; **13 types** in `std/types`. `funk check` green. | Built |
| I7 | **Reflect self-edits source** (Bruno's mid-pipeline ask): reads code + report, rewrites the `fn`; fires on parse/check failure, bounded attempts. | Built (fires on failure) |

## Still genuinely open (need Bruno)

- Per-call `with { … }` and project `object` block final grammar (doc 05).
- The vault / broker-egress-proxy interface (doc 05).
- Volume semantics (shared / persistent state) vs stateful streaming (doc 05).
- LICENSE file + opening the repo (when Bruno decides to publish).

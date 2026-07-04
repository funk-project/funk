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

## Reactive nodes (`docs/07`) — 2026-07-04

Bruno's direction: close the gap that funk is reactive at the stream layer but **not** at the
function layer (the evaluator is an eager tree-walk; a function does not fire on input arrival).
Reframe every function as a **dataflow node** that wakes when its inputs are ready, and replace
`return` with named multi-outputs + `set`/`flush`. **Breaking**; taken pre-0.1 as a clean break.
Full design: [`docs/07-reactive-nodes.md`](../../docs/07-reactive-nodes.md).

| # | Decision | Status |
|---|---|---|
| RN1 | **Firing groups** — `in` holds policy groups; **`zip` default** (pair 1:1), `latest` (combineLatest); ungrouped ports ⇒ implicit `zip`. Groups compose (`withLatestFrom`). Scalars: zip≡latest. | DECIDED (Bruno) |
| RN2 | **Port spec = prefix forms** `(name Type (min 0) (max 10) (default 1))` — no `=`, no commas (homoiconic). Specs on inputs validated on arrival; on outputs = post-conditions. | DECIDED (Bruno) |
| RN3 | **Multi-output selection = `let` destructuring** `(let ((q r) (f a b)) …)`; graph stays derived (composition, not hand-drawn edges). | DECIDED (Bruno) |
| RN4 | **Emission = `set` (stage) + `flush` (emit)**, `(flush (r v) …)` shorthand; **`return` removed**; `exit`/error stay. `flush` count = source/transform/sink taxonomy. **`yield` KEPT** as `each`'s emit primitive (folding it into flush needs an `each` rework — deferred). | DECIDED (Bruno); yield-removal deferred (assistant) |
| RN5 | **Drop `required`** — optional ⟺ has `(default)`; zip ports always required, latest ports required unless defaulted. | DECIDED (Bruno, 2026-07-04 — "ok") |
| RN6 | **`latest` = combineLatest** semantics; **RN7** input-spec violation cancels the scope (propagate-and-cancel, not clamp); **RN8** constraint v1 = `min max default`, (`len one-of pattern` later). | Built |

**I32 — Reactive nodes shipped (2026-07-04).** `model.go`: `Port` gains `Policy`/`Group`/`Min`/
`Max`/`Default`/`Optional`; `inPortsFromField` parses `(zip …)`/`(latest …)` groups + `(min/max/
default …)` specs. `run.go`: an `outFrame` (staged map + emitted list/live stream); `set`/`flush`
forms; `let` destructuring `(let (q r (f …)) …)`; input-spec validation + entry/optional defaults;
`invoke` dispatches scalar-once vs reactive; **`return` removed** (clear error via `run.go` +
`typecheck.go`). `firing.go`: `callReactive` + `fireZip`/`fireLatest` — a Stream bound to a scalar
port (`Num/Str/Bool/Time/Bytes`) drives per-item firing; other args sampled as constants.
`std/` migrated (31 `return`→`flush`, paren-aware); `map` is now `(f xs)` (the reactive lift);
`each`/`yield` kept for `filter`. Primer + `docs/07` updated. Plus **RN9 output post-conditions**
(`min`/`max` on out-ports enforced on `flush`) and **RN10 connection type-check** (`checkTypes` in
`typecheck.go` flags a scalar out→in mismatch, e.g. Str→Num; lenient on stream/list/Json/Any so no
false positives on std). **Verified**: go test / vet / `funk check` (127 fns, 0 false positives) /
`funk test` (26/26) green; live — `divmod`/`useboth`(multi-out wiring)/`pipe`(3-stage per-item
wiring)/`clamped`+`pos`(in/out min-max)/`scale`(default)/`zipAdd`(zip); `bad` (Str→Num) flagged.
**Remaining:** full goroutine-per-node scheduling (graph-level P2), fold `yield` into `flush`,
`docs/04` prose + artifact schema for specs/groups. See [`docs/07`](../../docs/07-reactive-nodes.md).

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
| I8 | **Reactive engine** (`stream.go`): `Stream = chan`; sources `range`/`nats`/`repeat`; operators `map`/`filter`/`take`/`collect` as core forms (higher-order `map`/`filter` take a function name). `take` cancels its source upstream via `context` — an infinite `nats` is bounded and stopped cleanly. `funk run` streams live. **Verified**: evens(5)=0,2,4,6,8 over an infinite source. | Built |
| I9 | **funkd server** (`serve.go`): `funk serve` runs HTTP — `POST /run` streams NDJSON flushed per item, `GET /functions` / `/introspect` / `/health`. `funk run --server URL` (or `FUNK_SERVER`) is a thin client that streams back. Realizes docs/03 §5. **Verified** over the wire. | Built |
| I10 | **Resources in the parser** (docs/05): nested `needs {}` / `effects {}` blocks parsed line-by-line into `Fn.Needs`/`Effects`; `introspect` **aggregates them up** a composite (declaration bubbles up). **Verified**: triage inherits createIssue's github/secret/config + net effect. | Built |
| I11 | **Docker sandbox**: `funk run --sandbox docker` executes python/go bodies in `python:3-slim`/`golang` (isolation, docs/03 §7). **Verified** live. | Built |
| I12 | **Runtime resource injection**: needs resolve from `--bind kind.alias=value` or env `FUNK_<KIND>_<ALIAS>`, injected into execution — python reads `needs['kind']['alias']`, funk reads `needs.kind.alias`. Closes docs/05 end-to-end; **vault/broker still future**. **Verified**. | Built |
| I13 | **Loops**: `while (s init) cond step` + `break`/`continue` implemented (were stubbed). **Verified**: powTwoLE(1000)=512. | Built |
| I14 | **`funk fmt`**: canonical formatter (decompile AST → .funk), idempotent. | Built |
| I15 | **Module system**: `funk get <git-url\|path> [name]` clones a package (a git repo) into `~/.funk/pkg`; cached packages resolve like std and compose. **Verified** with a local git package. Semver/lockfile/`use`-driven resolution still future. | Built (basic) |

| I16 | **Reactive set completed**: sources `range`/`nats`/`repeat`/`tick` (tick = real-time, verified over ~1s); operators `map`/`filter`/`take`/`collect`/`scan`/`merge`/`window`; **count + event-time windowing** (watermark closes earlier buckets). | Built |
| I17 | **`funk test`**: inline `test (is (call) expected)` assertions — funk verifies funk (6/6 green). **`funk doc`**: generated `docs/STDLIB.md` (self-describing). | Built |
| I18 | **`go` engine verified** (goAdd=42); all of builtin/python/go/claude verified live, codex wired. | Built |
| I19 | **Positional diagnostics** (the v0.1 #1): the tokenizer tracks `line:col`; `ParseError` and `Check` `Issue`s carry a `Pos` (+ source `File`), so parse/type errors print as `path:line:col: message` — specific now ("unterminated string", "expected ')' …", "unknown function"). `Pos` rides on every AST node (`json:"-"`, so `funk parse` output is unchanged). Enables the IntelliJ File Watcher (clickable errors) and is the groundwork for a future `funk lsp`. **Verified**: positions correct on 5 broken files + 2 new Go tests. | Built |
| I20 | **Editor support** (`editors/vscode/`): a TextMate grammar (+ language-config + manifest) for `.funk`, committed to the repo — drives IntelliJ (TextMate Bundles), VS Code, and later GitHub Linguist. Not in `.idea/`/`.vscode/` (both git-ignored) so it reaches contributors. `editors/README.md` has the IntelliJ setup + the `funk check` File Watcher. | Built |
| I21 | **Secrets: minimum v0.1 story** (the two moves that cost nothing; broker/vault still future — docs/05). (1) **Secret input off the command line**: `--bind k.a=value` also takes `@file`, `@-` (stdin), `env:VAR`. (2) **Trace redaction**: values from a `secret` need are masked to `***` in the `RunReport`/`--trace` (call values/errors), while the function's actual return value is left intact. (3) Docker forwards `FUNK_NEEDS` via env, not the `docker run` CLI. **Verified** live (@file + env:, grep for the raw secret = 0) + a Go test. | Built |
| I22 | **v0.1 hardening**: (a) `docs/ROADMAP.md` + a **stability table** in docs/04 §6a (Stable / Experimental / Proposal) — so a `.funk` file written today has a contract. (b) **Graceful engine degradation**: a missing engine binary now says "engine binary X not found in PATH — install it or pick another engine" instead of a raw exec error. (c) **`funk make` gated**: generated funk is kept only if it passes **check + inline tests**; on give-up the failing file is removed from `std/generated` (no blind merge, no contamination). (d) **Tests** on the fragile paths: `cmd/funk` gets its first test file (`expandBinding`), plus arity/unknown checks and the missing-binary path. All green + `go vet` clean. | Built |
| G4 | **Published v0.1.0** (2026-07-03): `main` pushed, annotated tag `v0.1.0` on the remote (3-part semver for `go install`). Prior public tag was `v0.0.1`. | DONE |
| G5 | **`memory/` → `etc/memory/`** (2026-07-03, Bruno): project-meta record moved under `etc/`; no code loads it (docs-only), layout note in docs/03 updated. | DONE |
| I29 | **funkd live trace — per-node event stream (docs/01 #4, for the funk-IDE)**: `internal/funk/run.go` gains `RunLive(lib, ref, inputs, opts, onEvent, onValue)` — streams trace events **and** stream values live *as they happen*, then returns the final `RunReport`. `evalTop` takes an optional `sink func(TraceEvent)`; `emit()` delivers to the sink in addition to accumulating the batch; a new `live()` helper delivers **animation-only** signals. A node now emits an **`enter`** event (glow — fires *before* it runs) in `evalCall` and the atomic top-level; `enter` is **live-only** (never recorded in `RunReport.Events`), so the batch report keeps its exact shape and all existing trace tests pass unchanged. `serve.go` `/run` accepts `"live":true` → emits `{"event":…}` per node, `{"value":…}` per item, `{"report":…}` at the end. This is the substrate for the funk-IDE's animated, self-observable trace (glow→green→branch→terminal + edge values). **Verified**: `go build`/`vet`/`test` green (+ `TestRunLiveStreamsEvents`) and live over the wire (`bump x=3` → enter/call gt→branch else→return→value 3; `squares n=3` → per-item enter/call interleaved with streamed values). **Next (camada 3):** stable node ids shared between `funk graph --json` and these events (via AST `Pos`) so the IDE maps an event to the exact graph node under repeated calls. | Built |
| I31 | **Trace terminal is post-order (reads in execution order)**: `return`/`exit` emitted their `terminal` event *before* evaluating the returned expression, so `(return (mul x 2))` traced as `terminal, enter mul, call mul` — backwards (Bruno spotted it in the funk-IDE debugger). Now the terminal is emitted *after* the value computes (and only on success): `enter mul, call mul=4, terminal return`. Only the ORDER changes (same event set), so existing trace tests are unaffected. Branch `fix/trace-terminal-order`. **Verified** live over funkd: `double(2)` → `enter mul`, `call mul=4`, `terminal return`. | Built |
| I30 | **Stable node ids (camada 3, for the funk-IDE)**: `TraceEvent` gains a `node` field = the call site's AST `Pos` (`"line:col"`), set on `enter`/`call`/`branch`/`terminal` events (in `evalCall`, `evalIf`, `return`/`exit`). `funk graph --json` emits the tree with the **same ids** per node (from `Pos`) + `kind`/`head`/`value`/`children`. So the IDE draws from `graph --json` and lights the exact node whose `id` matches each live event — stable even under repeated calls (the call site is fixed). **Verified**: `bump` graph ids (`gt`=24:10, `if`=24:6, else-`return`=26:8) match the `--trace` event ids; Go test `TestTraceEventCarriesNodeID`. Next (camada 4): the IDE consumes it; open Q = per-item stream index. | Built |
| I28 | **Broker Phase 2 minimal (docs/06 §5a)**: `internal/funk/broker.go` — a per-run localhost broker started by `evalTop` when a fn has integration needs. Integration-kind needs (not secret/config/env/volume) are **excluded from `FUNK_NEEDS`** and become brokered creds; the body calls `FUNK_BROKER/call` by alias, the broker injects `Authorization: Bearer <cred>` and enforces the `effects{net}` host allowlist. **Verified**: Go test (mock integration sees injected Bearer; undeclared host → 403) + live CLI (`body_has_token=False`, integration saw the Bearer, egress-denied host rebuffed). Limits: host engine only (docker→broker routing future), Bearer-only, explicit API (not the transparent proxy). | Built |
| I27 | **Egress Phase 1 (partial, docs/06 §7)**: under `--sandbox docker`, a python body with **no `net` effect** runs with `--network none` (`hasNetEffect` gate) — the raw-secret tier's "no exit" guarantee. **Verified live** (docker present): no-net body → `ENETUNREACH` (errno 101, fast); net-declared body → network stack (timeout). Plus a **`check`-time advisory** (`Issue.Warn`) for `secret + net` (`secretWithNet`, aggregated) — printed as `warning:`, does not fail check/CI, and skipped by the make gate. Remaining Phase 1: host allowlisting for net-declared bodies (needs the Phase-2 proxy). | Built |
| I26 | **Broker/egress design doc** (`docs/06-broker-egress.md`): turns docs/05's principle ("don't give the raw secret, close the network") into an implementable architecture — the broker = the run's single network exit that injects declared credentials and enforces the `effects { net … }` allowlist; raw-`secret` tier gets no network; config is *derived* from aggregated `needs`/`effects`. Threat model (defended vs honest limits), two calling conventions (explicit capability API → transparent HTTPS proxy), pluggable secret provider (direct-value today → vault later), and a 5-phase plan (Phase 0 shipped = I21; Phase 1 = egress default-deny). Linked from README/docs/05/ROADMAP. **Design only — not built.** | Doc |
| I25 | **`funk get` version pinning** (v0.2 module step, docs/ROADMAP): `funk get <url>@<ref>` pins a tag/branch/commit (shallow `--branch`, full-clone+checkout fallback for a SHA). `splitRef` peels the trailing `@ref` without tripping on `git@host` scp URLs (unit-tested). Full `use`+semver+lockfile resolution still to come. **Verified** live against a local tagged repo (pin v1.0.0, re-pin v2.0.0). | Built |
| I24 | **Sliding count windows** (v0.2, docs/04 §7 — partial): `(window s size every slide)` emits overlapping full-size count windows (1..5 size 3 every 1 → [1,2,3][2,3,4][3,4,5]); `slideCountWindow` + `windowEvery` route it. Event-time sliding + `lateness` + `on-late` stay Proposal. Example `slidingSums` in streams.funk; Go test + live verified (slidingSums 5 → 6 9 12). | Built |
| I23 | **Recovery forms** (first v0.2 item, docs/04 §6 — was *proposal*, now Experimental): `(on-error <body> (e) <handler>)` binds the error message and runs a fallback; `(retry <body> <n> [backoff <dur>])` re-runs up to n attempts with optional cancellable backoff. Wired into eval + typecheck (`coreForms`) + `graph`; `RunReport` emits `recover`/`retry` events (error field redacted). Examples in `std/examples/recovery.funk` (safeDiv/divOrDefault, inline tests) + docs/EXAMPLES.md §9; primer updated. **Verified**: on-error live + trace, 2 Go tests (recover + retry-attempt-count). Loop signals (break/continue) are not caught. | Built |

**CLI (14 commands):** `version · parse · fmt · run · list · types · check · test · doc · introspect · make · serve · get · help`. ~3600 lines Go, 110 fns / 13 types, all tests green.

**Not yet built:** `use`-driven per-package namespace resolution + semver lockfile; the vault/broker for secrets (injection is direct-value only); `bind`/`with` at the funk level; hot-mutation; persistent streaming workers (python map spawns per item).

## Still genuinely open (need Bruno)

- Per-call `with { … }` and project `object` block final grammar (doc 05).
- The vault / broker-egress-proxy interface (doc 05).
- Volume semantics (shared / persistent state) vs stateful streaming (doc 05).
- LICENSE file + opening the repo (when Bruno decides to publish).

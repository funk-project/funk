# funk — Architecture

> Status: **draft — to validate.** Captures the decisions taken so far; open questions are
> listed at the end.

## 1. Shape, in one picture

- **Open, all of it** — protocol + engine + standard library. Apache-2.0 (code), CC0 (spec).
- **Language: Go** — for the reactive engine (goroutines / channels / context) and the server.
- **Two things you install, both on disk, both git-backed and improvable:**
  - `cmd/` — Go: the engine, the CLI, the local server.
  - `std/` — `.funk`: the standard library.
- **The CLI is a thin client; the server executes.** A local server ships by default (runs in
  Docker for isolation); point it elsewhere with `--server` / config. A hosted / enterprise
  server is a *separate, future* deployment — not part of the contribution.
- **Runs are reactive, streaming pipelines** — everything is a stream; pipelines can run live
  and indefinitely.

## 2. Repository layout

```
funk/
  cmd/            # Go — the engine, CLI, and local server (the "machine")
  std/            # .funk — the standard library (the "vocabulary")
  docs/           # English docs — the idea, objective, architecture, spec
  etc/            # project meta — memory/ (the record: decisions, context)
  go.mod  README.md  LICENSE
```

Reading rule: **if it's in `cmd/`, it's Go; if it's in `std/`, it's funk.** The boundary is a
folder, not archaeology.

## 3. The three layers

1. **Protocol** (open spec) — the `.funk` grammar, the compiled **artifact** (a graph derived
   from the code), and the **schemas** (artifact, run request, run report). Language-neutral.
2. **Engine** (`cmd/`, Go) — the irreducible reactive machine: parser/compiler, the scheduler,
   streams, process I/O, cancellation, the daemon/server. The kernel is kept **as small as
   possible**.
3. **Standard library** (`std/`, `.funk`) — everything expressible as composition: stream
   operators (`map`/`filter`/`window`/`merge`), control desugars, user-facing functions. The
   stdlib is the **first program written in funk** — funk uses funk from line one.

**The Go/funk boundary:** what *must* be Go lives in `cmd/` (scheduling, channels, spawn,
context, parse/compile). Everything that *can* be funk migrates to `std/`. The kernel shrinks;
the notation-level surface grows.

## 4. The reactive engine

The reactive model is not a library we add — it is **Go's native concurrency, used directly.**

| Reactive concept | Go |
|---|---|
| stream | `chan` |
| `onNext(v)` | `ch <- v` |
| `onComplete` | `close(ch)` |
| `onError` / end a scope | cancel a `context.Context` |
| scope (function, loop iteration, `if` branch) | a child `context` |
| control terminals (`return`/`exit`/`break`/`continue`) | cancel / complete the target scope |

- **Everything is a stream.** A single value is a stream of length 1 — one uniform model; the
  engine optimizes the common case.
- **A node is a goroutine.** It reads its input channels (a *join*), passes the control gate,
  runs its body, emits on its output channel(s), and `close`s them (onComplete). On error, it
  cancels the scope's context.
- **Function bodies are cancellable processes.** Bodies run as subprocesses via
  `exec.CommandContext(ctx, …)`. When a scope cancels — because a `return` fired, or an error
  arrived upstream — the subprocess (`python3`, `go`, `claude`) **is killed automatically by
  the context.** stdout is read as a stream. Reactive cancellation propagates all the way to
  the OS process.
- **Persistent workers.** Because streams can be infinite, we cannot spawn-per-item. Each
  streaming node has a **long-lived subprocess**, fed **NDJSON on stdin**, emitting **NDJSON on
  stdout** (one line per `onNext`). Go goroutines manage the pipes as channels.
- **Backpressure** is essential (fast infinite source, slow consumer). Bounded channels give
  it for free.
- **Live pipelines.** Infinite streams mean a run may never complete — it is a *long-running
  pipeline*, which is why runs live on a server.

### The calling convention

> **stdin = input stream (NDJSON) → stdout = output stream (NDJSON).** One JSON object per
> `onNext`. Language-neutral — any language can read stdin / write stdout line by line.

Function flavors: **source** (0-in, N-out — a generator), **transform** (per-item map),
**sink/aggregate** (N-in, 1-out — reduce). The runtime **wraps the user's per-item function in
the stream loop**, so simple functions stay simple (`return a + b`) while the runtime handles
the streaming.

## 5. CLI ↔ server

- The **CLI is a client.** Pure protocol ops (`validate`, `compile`, `introspect`, `fmt`) run
  locally (they are deterministic, no I/O). **Execution (`run`) goes to a server.**
- A **local server** (`funkd`) ships and runs by default — **in Docker**, for isolation of
  untrusted package code. Override with `--server <url>` > `FUNK_SERVER` > config
  (`~/.funk/config`, with a token per server) > default local.
- **`run` starts a pipeline, it does not return a value.** A run is a **handle** + a stream.
  For finite pipelines it streams to completion; for infinite ones it is a *deployment* you
  observe and stop. Lifecycle: `funk run` (foreground stream) / detached deployment,
  `funk ps` / `funk logs` / `funk stop`.
- **Scoped runs (partial execution).** `run` can target the **whole function**, a **single
  function** (`funk run <fn> --input …` → inspect its output), or a **segment** (inject a value
  at an intermediate node and run downstream). Any node or slice is independently runnable
  because every function shares the typed NDJSON calling convention — the test/probe substrate
  for both the user and the self-improving agent.
- **The wire is schema'd:** `RunRequest {plan | plan-ref, inputs, sandbox, project-ref}` →
  `RunReport` + a live event stream. The server API is a thin transport over the same engine
  interface — swapping local ↔ remote ↔ hosted is config, not rewrite.

## 6. The module system (packages)

Go-like: **a package is a git repo; a reference is an address.**

- **Addresses are `/`-paths, local or web.** `funk/std/map`, `github.com/user/lib/fn1`,
  `./localFn`. A function *is* an address. A workflow is a composition of addresses.
- **Every import is aliased (implemented).** `use "pkg" as ml` binds a local qualifier; calls to
  that package are written `(ml.fn …)`. There is **no implicit global cross-package namespace**:
  a bare name resolves only within the caller's own package, so to reach another package you must
  `use … as` it and qualify. A plain `use "pkg"` (no `as`) is a `check` error. A fully-qualified
  address (`funk/std/maths/add`) always resolves. The alias is file-local and display-only for
  addressing — the underlying reference is still the address.
- **The manifest is `.funk`** — funk describes its own package (self-hosting):
  ```
  package "github.com/user/myproject" {
    version 0.1.0
    use "funk/std/maths" as maths
    use "github.com/user/lib" v1.2.0 as ml
  }
  ```
- **`funk/std` is a package on disk** (not embedded), so the AI can read, improve, and version
  it. It is built on the primitives `cmd/` (`funk/core`) exposes.
- **Resolution order:** `funk/std/*` (stdlib) → local project functions → imported packages.
- **Cache** (shared, Go-style): `~/.funk/pkg/<path>@<version>/`.
- **Versioning:** semver + git tags, with a **content-hash lockfile** — if a tag is re-pointed
  to a different commit, the hash mismatches and it **fails** (integrity, like `go.sum`).
- **Fetch:** `funk get <path>@<version>`.

## 7. Isolation

- **Default on:** the daemon runs in **Docker** — third-party package bodies (`src`:
  python/go/claude) execute isolated from the host.
- **Trust-tiered granularity (decided).** **Verified** code (`funk/std`, local project code,
  signed+verified packages) runs **inside the daemon container** — fast, no per-run cold-start.
  **Unsigned / unverified** packages are pushed out to their **own container**, isolated from the
  daemon and from other runs. Isolation adapts to trust rather than being uniform.
- **Package trust model.** The content-hash lockfile gives **integrity** (a re-pointed tag
  fails); a package **signature** (author / registry) plus a *verified* state gives
  **authenticity**. The engine routes execution to the right tier by this status.
- **Effects** declared by functions (capabilities: which resources — network / fs / secrets —
  a workflow may touch) are enforced by what the container is permitted to see, on either tier.

## 8. Observability & introspection

- **Static introspection** — the plan as data: signature (ins/outs), nodes, engines,
  aggregated `requires`, the functions it calls, unresolved references, type issues.
- **Dynamic** — the run as data. A **live event stream** (per node/per `onNext`) that the
  server relays (SSE/websocket) + a **current-state snapshot** (last value, throughput, counts)
  for infinite pipelines; a final **`RunReport`** (structured) for finite ones.
- This is the thesis' self-observability made concrete: the agent reasons over **structure**,
  not text — over a running system.

## 9. Self-improvement (in the open)

- The AI edits **`std/`** (`.funk`) and **`cmd/`** (Go) and opens **pull requests** — reviewed
  (by human or AI) and merged into a **new version** (semver + tag, hash-pinned). Nothing
  mutates under anyone's feet.
- Two depths: safe, versioned improvement of the **notation layer** (`std` and libraries), and
  development of the **engine** itself (`cmd`, Go) — both open, both via PRs.

## Decisions

- **Windowing time-model = event-time.** Windows are cut by the event's own timestamp, not by
  arrival — so aggregates are correct under out-of-order / delayed / replayed data, and **replay
  is deterministic** (ambition #4). This commits the engine to: **event timestamps as
  first-class** on every `onNext`, **watermarks** ("all events up to T have arrived", to know
  when to close a window), and a **late-data policy** (drop / re-emit / allowed lateness).

- **`run` semantics = Ctrl-C detaches (uniform).** `funk run` streams in the foreground;
  **Ctrl-C detaches the CLI only — the pipeline keeps running on the server** (`funk stop` kills
  it). `-d` detaches from the start; observe with `funk ps` / `funk logs`. Escape hatch:
  `--stop-on-exit` for a throwaway job you *want* to die with Ctrl-C. Same behaviour for finite
  and infinite runs — one mental model, no live pipeline lost by accident.
- **Isolation = daemon-in-container, with a trust tier.** The daemon runs in Docker (host
  isolation baseline). **Verified code** (`funk/std`, local project code, signed+verified
  packages) runs **inside the daemon container** — fast, no cold-start. **Unsigned / unverified
  packages** are pushed out to their **own container**, isolated from the daemon and from other
  runs. Isolation is thus **adaptive to trust**, not uniform. This commits us to a **package
  trust model**: the content-hash lockfile already gives *integrity*; we add *authenticity* — an
  author / registry **signature** and a *verified* state — and the engine **routes execution by
  tier**.

- **Hot-mutation = immutable runs (v1).** A run is immutable and deterministic (fits event-time
  + replay). Self-improvement is a **discrete loop**: observe → test in isolation → `stop → edit
  → re-run` — not in-place mutation of a live graph. *Drain-and-switch* (start v(n+1) and hand
  the stream off from v(n)) is noted as the future path to **continuity** for infinite pipelines,
  not v1.
- **Partial / scoped execution (v1 capability).** Because every function is an addressable
  artifact with a uniform typed NDJSON in/out, the engine runs at **any granularity**: (a) a
  **single function** — feed it an input, inspect its output (unit-test a node); (b) **start from
  the middle** — inject a value at an intermediate node and run downstream (test a segment) —
  without pulling the whole pipeline from its source. This is the test/probe substrate: the
  **user tests parts of the stream**; the **agent validates a candidate change in isolation**
  before committing the immutable re-run. It is the ergonomic complement that makes immutable
  runs iterable.

- **Repos = one repo now.** `cmd/` (Go) and `std/` (`.funk`) live in one repository while they
  co-evolve tightly (one PR, one CI, one `git bisect`). The resolver treats `std/` as the
  `funk/std` package; it **graduates to its own repo + independent semver** when the engine↔std
  interface stabilizes — **the `funk/std/*` address never changes**, only its backing store.

## Open questions (to decide)

1. **The `.funk` manifest** exact syntax (the `package` block) — deferred to the `04` / `05`
   syntax pass.

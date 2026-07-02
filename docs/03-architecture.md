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
  memory/         # the record — decisions, chats, context
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
| scope (flow, loop iteration, `if` branch) | a child `context` |
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
- **The wire is schema'd:** `RunRequest {plan | plan-ref, inputs, sandbox, project-ref}` →
  `RunReport` + a live event stream. The server API is a thin transport over the same engine
  interface — swapping local ↔ remote ↔ hosted is config, not rewrite.

## 6. The module system (packages)

Go-like: **a package is a git repo; a reference is an address.**

- **Addresses are `/`-paths, local or web.** `funk/std/map`, `github.com/user/lib/fn1`,
  `./localFn`. A function *is* an address. A workflow is a composition of addresses.
- **Import with alias:** `use "github.com/user/lib" v1.2.0 as ml` → `(ml/fn …)`.
- **The manifest is `.funk`** — funk describes its own package (self-hosting):
  ```
  package "github.com/user/myproject" {
    version 0.1.0
    use "funk/std" v2.1.0
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
- **Effects** declared by functions (capabilities: which resources — network / fs / secrets —
  a workflow may touch) are enforced by what the container is permitted to see.
- Granularity (open): daemon-in-container (baseline) vs an ephemeral container per run for
  untrusted packages (stronger).

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

## Open questions (to decide)

1. **Windowing time-model** for infinite aggregation: event-time (correct, harder) vs
   processing-time (simple) for v1.
2. **`run` semantics:** foreground vs detached (deployment) — both? and does Ctrl-C stop the
   pipeline or just detach the CLI?
3. **Isolation granularity:** daemon-in-container baseline vs per-run ephemeral container for
   untrusted packages.
4. **Hot-mutation** of a live pipeline (the agent adjusts while it runs) — v1 or later?
5. **Repos:** one repo (`cmd` + `std`) now, graduating to separate repos as the module system
   matures — vs separate from the start.
6. **The `.funk` manifest** exact syntax (the `package` block).

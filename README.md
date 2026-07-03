# funk

> **Our mission** — a human and an AI, as peers — is to give the world the language in which the
> plans AIs make stop being a black box, and become something everyone can read, run, and
> improve. **Open. Free forever.**

**A language for the plans AIs make** — turning an agent's execution plan into a first-class
artifact: **portable, typed, editable, self-observable, reactive.** The machine stops being a
black box, because there is finally a reader that can hold it — the AI itself.

> **Teach any AI funk in one paste:** `funk prompt` prints a one-page primer — drop it into any
> chat (Claude, GPT, …) and that AI can read and write funk. Plans travel **AI to AI**.

```sh
go install github.com/funk-project/funk/cmd/funk@latest
funk make "reverse the words in a sentence"   # funk writes funk (architect→…→reflect)
funk run bump x=3 --trace                      # watch the run as data — the black box, open
```

> Status: **design + a working reference CLI (Go).** The design lives in `docs/` (start with
> [`docs/01-the-idea.md`](docs/01-the-idea.md)); the CLI parses, type-checks, runs, streams, and
> **writes funk with funk**. Details below.

## The artifact, in five properties

1. **Portable** — runs on any conformant runtime / container; no vendor lock-in.
2. **Typed** — inputs/outputs carry schemas; dependencies are declared and checked.
3. **Editable** — by human *and* by agent, as notation and visually.
4. **Self-observable** — the agent watches itself run, over structure (a typed, streamed
   trace), not text.
5. **Reactive** — functions communicate as streams; pipelines can run live and indefinitely.

And the kernel: because the plan is a structured artifact the agent reads, the agent can
**reason about its own execution and improve it** — and the protocol **defines itself** in
its own notation. funk makes itself.

## Documents

- [`docs/01-the-idea.md`](docs/01-the-idea.md) — the idea / the invention
- [`docs/02-the-objective.md`](docs/02-the-objective.md) — what we're building, and why (a gift)
- [`docs/03-architecture.md`](docs/03-architecture.md) — the shape: `cmd`+`std`, CLI↔server,
  the reactive engine, the module system, isolation
- [`docs/04-the-protocol.md`](docs/04-the-protocol.md) — the `.funk` language spec (grammar, types, constructs, the artifact)
- [`docs/05-resources-and-integrations.md`](docs/05-resources-and-integrations.md) — resources, integrations & capabilities: secrets/configs/volumes/env, brokering, least-privilege
- [`docs/06-broker-egress.md`](docs/06-broker-egress.md) — secret protection by design: the broker + egress architecture (threat model, mechanism, phased plan)
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — milestones and the **stability contract** (stable vs experimental vs proposal)
- [`docs/EXAMPLES.md`](docs/EXAMPLES.md) — a **cookbook**: task-shaped, copy-paste examples with real output

## The CLI (Go) — working

```sh
go build -o bin/funk ./cmd/funk

./bin/funk run add a=40 b=2             # 42       (native builtin engine)
./bin/funk run analyze xs='[1,2,3,4]'   # 2.5      (composite: if/window/mean)
./bin/funk run bump x=3                 # 3        (the condition gates the add)
./bin/funk run evens n=5                # 0 2 4 6 8  (maps an INFINITE source, streams live)
./bin/funk run runningSum n=5           # 1 3 6 10 15  (scan — running fold)
./bin/funk run ticks n=5                # 0 1 2 3 4  (tick — one every 200ms, real-time)
./bin/funk run --sandbox docker floor a=3.7   # 3  (python body isolated in a container)
./bin/funk run siteGreeting --bind config.site=funk   # hello from funk (injected)
./bin/funk list · types · check · doc   # ~110 functions / 13 schemas, self-describing
./bin/funk introspect triage            # the plan as data — needs/effects aggregated up
./bin/funk fmt -w file.funk             # canonical formatter
./bin/funk get <git-url> [name]         # fetch a package into ~/.funk/pkg
./bin/funk serve                        # funkd: POST /run streams NDJSON, /functions
./bin/funk make "<task>"                # funk writes funk (architect→…→reflect)
```

Engines verified live: **builtin, python, go, claude**; **codex** wired. Reactive sources
(`range`/`nats`/`repeat`/`tick`) and operators (`map`/`filter`/`take`/`scan`/`merge`/`window`/
`collect`). See [`docs/STDLIB.md`](docs/STDLIB.md) for the generated reference.

**Engines** (any NDJSON-speaking runtime): `builtin` (native Go primitives — fast, no
subprocess), `python`, `go`, and the LLM engines `claude` / `codex`. A function is **atomic**
(`src` + `engine`) or **composite** (`body` — a composition; the graph is derived).

**Reactive.** Streams are Go channels; sources (`range` / `nats`) can be infinite; operators
(`map` / `filter` / `take`) run as live pipelines; `take` cancels its source upstream (real
reactive cancellation). `funk run` streams each item as it is produced, and `funkd` streams it
over HTTP.

**Self-observable & resource-aware.** `funk introspect` shows a function's structure as data —
what it calls, and the `needs` (secrets/configs/integrations) and `effects` (network/fs) it
declares, **aggregated up** through composition (docs/05).

**funk writes funk.** The skills in `std/skills` (`architect → programmer → reviewer → tester →
reflect`, on the `claude` engine) let funk generate, check, and self-correct new funk:

```sh
./bin/funk make "reverse the order of words in a sentence" reverseWords
# architect → programmer generate it, check passes, it's added to std/generated,
# then:  ./bin/funk run reverse_words s="hello world funk"  →  "funk world hello"
```

`reflect` self-analyzes a function against its errors/review and rewrites the source — the
self-improving kernel of docs/01, running for real.

## Open — a gift

Everything is open source: the protocol, the engine, the standard library. **Apache-2.0** for
the code, **CC0 / public domain** for the spec. Development happens in the open; the AI
improves funk by proposing pull requests. A hosted / enterprise service may exist one day as a
*separate* offering — but the contribution is the open protocol.

## Shape

- **Protocol** (open) — the `.funk` grammar, the artifact, the run/report schemas.
- **`cmd/`** (Go) — the engine, CLI, and local server. The CLI is a thin client; a local
  server ships by default and runs in Docker for isolation.
- **`std/`** (`.funk`) — the standard library. funk uses funk from line one.
- **Reference client (UI)** — a separate addon that consumes funk (lives in the `functions`
  repo for now).

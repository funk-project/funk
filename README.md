# funk

**A language and protocol for defining workflows** — and, at its core, for turning the
execution plan of an (AI) agent into a first-class artifact: **portable, typed, editable,
self-observable, and reactive.**

> Status: **early / design.** We are (re)writing this from the idea up, document by
> document. Start with [`docs/01-the-idea.md`](docs/01-the-idea.md).

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

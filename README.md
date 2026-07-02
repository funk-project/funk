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
- `docs/02-the-objective.md` — *(to come)*
- `docs/03-architecture.md` — *(to come)*

## Shape (to come)

- **Protocol** — open: the `.funk` grammar, the artifact schema, the run/report schemas.
- **Engine + CLI + server** — Go. The CLI is a thin client; a local server ships by default;
  enterprise/hosted is a separate server build.
- **Reference client (UI)** — a separate addon that consumes funk (lives in the `functions`
  repo for now).

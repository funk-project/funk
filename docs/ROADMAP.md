# funk — Roadmap

funk is **design + a working reference CLI (Go)**. This file says what "done" means for the next
milestones, and — crucially — **what is stable vs experimental**, so a `.funk` file written today
keeps working tomorrow.

The spec (`docs/04`, `docs/05`) is the north star; it deliberately runs ahead of the
implementation and marks unbuilt parts *(proposal)*. This roadmap is the honest map of what the
**reference CLI actually does**, and in what order the gap closes.

## Stability contract

- **Stable** — covered by tests, and we will not break it without a major-version bump.
- **Experimental** — implemented but may change shape; use it, but pin your funk version.
- **Proposal** — in the spec, **not yet implemented**. Writing it will not run.

See [`docs/04` §6a — Stability](04-the-protocol.md#6a-stability-what-is-stable-today) for the
per-construct table.

## v0.1 — "trust it for small, real workflows"

The bar: a newcomer can write, check, and run funk without surprises, and a credential does not
leak by accident. Not "production platform" — "a language you can rely on for a task."

**In (done):**

- [x] **Parse / type-check / run** — atomic (builtin/python/go/claude/codex) + composite, with
      tree-eval so a condition gates its branch by construction.
- [x] **Reactive core** — sources `range`/`nats`/`tick`/`repeat`; operators
      `map`/`filter`/`take`/`scan`/`merge`/`collect`/`window` (count + event-time); `take` cancels
      its source upstream.
- [x] **Positional diagnostics** — parse/check errors as `path:line:col: message` (I19), so an
      editor / File Watcher can navigate to them.
- [x] **Secrets, v1** — `--bind` reads from `@file` / `@-` / `env:VAR` (off the command line);
      the trace redacts `secret` values to `***` (I21). *Broker/egress is a v0.2 item.*
- [x] **Self-observability** — `introspect`, `--trace` / `RunReport`, `graph`.
- [x] **funk writes funk** — `funk make` (architect→programmer→check→reflect).
- [x] **Editor support** — TextMate grammar for IntelliJ / VS Code (`editors/`).

**Still to close before tagging v0.1:**

- [ ] **Grammar stability declared** — the §6a table below is the contract (this change).
- [ ] **Test coverage on the fragile paths** — stream cancellation, resource injection,
      arity/positional diagnostics, and the CLI binding-input logic. (Ongoing; growing.)
- [ ] **Graceful engine degradation** — a clear message when an engine binary (python/go/claude/
      docker) is absent, instead of a raw subprocess error.
- [ ] **`funk make` gated** — generated funk enters `std/generated` only when `check` + `test` are
      green (no blind merge).

## v0.2 — "reproducible & safe with real credentials"

- [~] **Module resolution for real** — `funk get <url>@<ref>` now pins a tag/branch/commit (a
      first reproducibility step); still to come: `use "<addr>" <semver> as <alias>` driving
      per-package namespaces + a lockfile.
- [~] **Secret broker + egress control** — the function receives a *capability*, not the token;
      the sandbox reaches only declared endpoints (docs/05 A–C). Redaction stops being the only
      line of defense. **Design:** [`docs/06-broker-egress.md`](06-broker-egress.md). **Phase 1
      partly shipped:** no-`net` bodies run `--network none` (verified) + a `secret+net` check
      warning. Remaining: host allowlisting (needs the Phase-2 proxy), then the broker itself.
- [x] **Recovery forms** — `(on-error <body> (e) <handler>)` and `(retry <body> <n> [backoff
      <dur>])`, implemented + tested (Experimental). *Landed early.*
- [~] **Full `window`** — sliding **count** windows (`every <slide>`) are in + tested;
      event-time sliding, `lateness`, and `on-late` remain (docs/04 §7, proposal).

## v0.3+ — "live pipelines & shared state"

- [ ] **Persistent streaming workers** — today a python `map` spawns a subprocess per item.
- [ ] **Volumes / stateful streaming** — shared and persistent state semantics (docs/05).
- [ ] **`with { … }` / project `object` blocks** — per-call binding grammar (docs/05).
- [ ] **Hot-mutation** — edit a running plan.

## Non-goals (for now)

- A hosted/enterprise runtime is a *separate* offering; the contribution here is the **open
  protocol + reference engine**. Apache-2.0 (code) / CC0 (spec).

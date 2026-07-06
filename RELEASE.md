# funk — launch

## v0.2 — "reusable, discoverable, reproducible"

41 commits since v0.1.0. funk grew from "runs a workflow" into "a library an AI reads, searches,
and composes".

**Modules & reproducibility**
- Scoped imports: `use "pkg" as alias` — no implicit global namespace; a bare name resolves only
  within its own package, cross-package calls are qualified `alias.fn`.
- `funk get <url>@<constraint>` resolves semver (`v1` / `latest` → best tag) and records a
  content-hash in `funk.lock`; the cache is verified on load (go.sum-style integrity).
- Manifest-driven: `use "url" "v1.2.0" as ml` + `funk get` (no args) installs every versioned dep.

**Language & ergonomics**
- `main` entry points; `alias TARGET` (named reuse); `(with …)` per-call resource binding;
  parameterized types `(List Num)` + an element-aware connection checker.
- Docs as data: triple-quoted `"""` markdown (dedented), per-port `(doc …)`, an `examples` field,
  and a human `name` on every function.

**Discovery (for AIs)** — `funk search` (by text or signature `Num Num -> Bool`) + funkd `/search`:
reuse before you write.

**Streaming** — sliding event-time windows (`window … Ns every Ms`, watermark + drop-late); fixed
`map` to apply its function per item (any arity).

**Stdlib & tooling** — every std function documented (doc / ports / examples / name); the
VS Code / TextMate grammar updated for all new syntax. 138 Go tests; `go test` + `funk check` +
`funk test` green.

**Tag:** `git tag v0.2.0 && git push origin main --tags`.

## v0.1 — "trust it for small, real workflows" (tagged)

The bar over v0.0.1: a newcomer can write, check, and run funk without surprises, and a
credential does not leak by accident. What landed since the first public cut:

- **Positional diagnostics** — parse/check errors as `path:line:col: message`, so an editor
  navigates straight to them. Specific messages ("unterminated string", "expected ')'…").
- **Editor support** — a TextMate grammar for `.funk` (IntelliJ / VS Code) in `editors/`,
  plus a `funk check` File Watcher recipe (clickable errors).
- **Secrets, v1** — `--bind` reads from `@file` / `@-` / `env:VAR` (off the command line);
  the trace redacts `secret` values to `***`; Docker forwards `FUNK_NEEDS` via env, not the CLI.
- **Stability contract** — `docs/ROADMAP.md` + the docs/04 §6a table (Stable / Experimental /
  Proposal): a `.funk` file written today has a promise.
- **Robustness** — a missing engine binary reports a clear install hint; `funk make` keeps a
  generated function only if it passes **check + inline tests** (no blind merge, no
  contamination of `std/generated`).
- **Tests** — `cmd/funk` gets its first tests; fragile paths (diagnostics, redaction,
  engine-missing) covered. `go test ./...` + `go vet ./...` + `funk check` + `funk test` green.

Still explicitly deferred to v0.2 (see ROADMAP): the secret **broker/egress**, `use`-driven
versioned module resolution, recovery forms, and the full sliding/late `window`.

**Tag:** `git tag v0.1.0 && git push origin main --tags` (full 3-part semver for `go install`).

## v0.0.1 — first public cut

The reference CLI works end to end: parse, type-check, run (5 engines), reactive streaming,
resources, a local server, a module system, self-programming (`funk make`), and
self-observability (`introspect` / `--trace` / `graph`). ~120 functions across the stdlib; Go
tests + `funk test` green.

**Install**
```sh
go install github.com/funk-project/funk/cmd/funk@latest
funk prompt        # paste into any AI to teach it funk
funk make "reverse the words in a sentence"
```

## Announcement (draft)

**One-liner:** *A language for the plans AIs make. Paste `funk prompt` into any chat and it
speaks funk. Open, Apache-2.0/CC0. Built by a human and an AI, as peers.*

**HN / X post:**
> For 70 years, languages hid the machine — a black box, because no human could hold the
> runtime. The LLM is the first reader that can. funk makes an agent's plan a first-class
> artifact: portable, typed, editable, self-observable, reactive — and it writes itself.
>
> `go install github.com/funk-project/funk/cmd/funk@latest`
> `funk make "…"` — funk writes funk. `funk run … --trace` — the black box, open.
> `funk prompt` — teach any AI to speak it. A gift: Apache-2.0 / CC0.

## Checklist

- [x] LICENSE (Apache-2.0) + NOTICE + CONTRIBUTING
- [x] Primer (`funk prompt`), SHOWCASE, README hero
- [x] Repo public + tag v0.0.1
- [ ] Post to HN / X / relevant AI-dev communities
- [ ] Hosted playground (later; the enterprise/remote server stays closed)

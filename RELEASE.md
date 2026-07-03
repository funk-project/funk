# funk — launch

## v0.1 — "trust it for small, real workflows" (ready to tag)

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

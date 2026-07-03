# funk — launch

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

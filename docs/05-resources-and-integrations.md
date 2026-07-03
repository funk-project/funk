# funk — Resources, Integrations & Capabilities

> Status: **draft — to validate.** Captures the model settled in discussion.

## Two axes: data and resources

A function has **two different kinds of dependency**, and keeping them separate is the whole
design:

- **Data** — `in` / `out`: the **reactive streams** the function processes. *What it works on.*
- **Resources** — `env` / `volumes` / `secrets` / `configs`: the **ambient context** it needs.
  Not streams — **injected capabilities**. *What it needs around it.*

This is the split every serious system makes: Kubernetes (inputs vs env/volumes/secrets), effect
systems (computation vs capabilities).

## Config vs Secret

- **`config`** (non-secret) — may live in the **artifact**; it is data.
- **`secret`** (secret) — **never in the artifact.** It is a **reference** (`github.secret.token`)
  resolved at runtime from a **vault**, scoped and audited.

> **Consequence: secrets never flow through the dataflow.** They are injected side-channel,
> which is what lets funk into regulated, high-stakes settings.

## Resources are first-class

The resource axis has first-class citizens: **integration types**, **integration objects**,
**isolated** secrets / configs / volumes, and the **network capability** (which endpoints a
function may reach — see *Protecting secrets*). They are defined at the **project /
environment** level and referenced by path from functions.

## Integrations: type → object → reference

1. **Integration TYPE** (a package) — the *mould*: a **typed schema** of what it needs
   (`secret token`, `config {…}`) + the **functions** it exposes (`getRepo`, `listIssues`).
   e.g. `funk/integrations/github`.
2. **Integration OBJECT / connection** (a **project-level** instance) — a configured connection
   binding the type's schema to **real values** (config) + **vault references** (secrets). Many
   per type: `github-work`, `github-personal`, `github-clientA`.
3. **Reference** — a function declares the **specific** resources it needs, by path, and may
   declare several (plus isolated secrets).

## Resource addressing — two scopes

`kind ∈ { <integration type>, secret, config, volume, env }`, in two places:

- **Project scope** — `<object>.<kind>.<key>`: how a project's configured objects expose their
  values.
  ```
  github-work.secret.token
  github-work.config.owner
  ```
- **Function scope** — `needs.<kind>.<alias>`: how a function reaches the slots it declared (see
  *Syntax*). The **alias** lets one function hold several of a kind — two githubs are
  `needs.github.a` and `needs.github.b`.
  ```
  needs.github.a          ; an integration slot
  needs.volume.data
  needs.secret.tok
  needs.config.cfg.repo
  ```

`bind` wires a function's slot to a project object (`github.a = github-work`).

## Least-privilege

A function receives **only** the resources it declares. `github-work.secret.token` does **not**
grant `openai-prod.secret.api_key`. Enforced by the sandbox — each leaf sees exactly what it
declared, nothing more.

## Protecting secrets — brokering & egress

The hard question: if a function *holds* a secret, its (possibly third-party) code can print,
log, or exfiltrate it. **Redaction alone is not enough** — adversarial code encodes the value
(base64, split, reversed) and escapes any output scan. Redaction is defense-in-depth, not a
guarantee. Real protection is layered, and the principle is: **don't give the raw secret to the
code, and close the network.**

- **(A) Brokering — the default for integrations.** The function never receives the credential;
  it receives a **capability**. It says *"call github endpoint X"*; the **engine / proxy holds
  the secret, attaches the auth header, and forwards.** You cannot print what you do not have.
  This is the *Secretless Broker* / *Vault Agent* model, and it is the natural shape for
  integrations (which *are* service calls).
- **(B) Egress control — least-privilege network (first-class).** A function's sandbox may reach
  **only the endpoints its integration declares** (`github.com`). Even a function that *held* a
  secret could not exfiltrate it — there is no network path to `attacker.com`. **Network is a
  first-class capability**, declared like any resource; the Docker sandbox enforces it.
- **(C) Short-lived, scoped tokens.** Prefer just-in-time, narrowly-scoped tokens (OAuth / STS
  exchange) over long-lived secrets. A leak is useless quickly and only for the permitted
  operation — minimal blast radius.
- **(D) Log / output redaction — defense-in-depth only.** Mask secret occurrences in
  stdout / stderr. Catches *accidental* leaks, not an adversary. Never the only line.

**Synthesis:** integrations **broker by default** — the function never sees the token, so
printing is *impossible*. The rare code that genuinely needs a raw secret in-process runs under
**egress-lockdown + short-lived + redaction** — so printing is *useless*: no value, or no way
to send it out. Honest limit: hand a raw secret to arbitrary code *with an open network* and
there is no guarantee — which is why the architecture is **don't (broker) and can't (egress).**

### What the reference CLI does today (v1)

Injection is direct-value (the broker/vault of (A)–(C) is still future), but the two moves that
cost nothing are in:

- **Secret input stays off the command line.** `--bind kind.alias=value` also accepts
  `@path` (read a file), `@-` (read stdin), and `env:VAR` (read an env var) — so a credential
  never sits in `ps` output or shell history. Env fallback (`FUNK_<KIND>_<ALIAS>`) still works.
- **The trace redacts secrets (D).** Values resolved from a `secret` need are masked to `***`
  wherever they surface in the `RunReport` / `--trace` (call values, errors) — self-observation
  (docs/01 #4) does not become credential exposure. The function's *return value* is left intact
  (it is the result the caller asked for). Docker runs forward `FUNK_NEEDS` via the environment,
  not the `docker run` command line. As (D) notes, this catches accidental leaks, not an
  adversary — the real guarantee is broker + egress, still to come.

## Binding ≠ Injection (the crux)

Secrets are **not** threaded parent-to-child through the call tree. Two separate things:

- **Injection** (the *value*) — happens **at the leaf**, straight from the vault, into that
  function's sandbox. **It never travels through functions.**
- **Binding** (a *reference*) — this is what may flow through composition: *which concrete
  object* fills a function's need. A name, not a value.

So for `A` calls `B`, where `B` needs a secret: **`A` does not need to hold it.** At most `A`
decides *which* object `B` uses (a name). The value is resolved from the vault and injected into
`B`, in `B`'s sandbox, when `B` runs. `A` never touches it.

## Slots — dependency injection & reusability

To keep functions reusable (not hardcoded to a project's objects):

1. A function declares an **abstract slot**: `needs { github gh }`, and reaches it as
   `needs.github.gh` — calling `(needs.github.gh/…)`.
2. The **concrete object** is **bound** — by the *run* (default) or by a parent
   (`bind { github.gh = github-work }`). A reference, not the token.
3. At runtime the engine resolves the object's secret from the vault and **injects it into the
   function**. The parent **delegates authority** (chooses which github) **without ever holding
   the value.**

## Two modes, both valid

Both use `needs` slots; the difference is **who binds**:

- **direct** — the function's own project binds the slot to a fixed object; simple, for
  project-local functions.
- **delegated** — a parent or the run binds the slot (`bind` / `with`); abstract and reusable,
  for shared libraries / integrations. The general form.

## Declaration vs injection (provisioning & audit)

- **Needs bubble UP as declarations** — `introspect` shows the whole workflow needs `github` +
  `openai`, so you know what to provision and can audit it.
- **Values inject at the leaves.**
- Declaration aggregates upward; injection happens at the point of use.

## Typing / self-describing

Because the integration TYPE carries a schema, `github-work.secret.token` is **type-checked**:
does it exist? is it a secret? A reference to a resource the integration does not define **fails
at compile** — not in production. Integrations are self-describing and verifiable.

## Syntax

**Declaration — the `needs {}` block.** One line per resource: `<kind> <alias> [schema]`, where
`kind` is an **integration type** (e.g. `github`) or a builtin `secret | config | volume | env`.

```
fn syncRepo {
  in  (x Json)
  out (r Json)
  needs {
    github  a               ; an integration slot (type github)
    github  b               ; a second github — aliases keep them apart
    volume  data
    secret  tok
    config  cfg  Config     ; typed by a `type` schema
    env     LOG_LEVEL
  }
  body
    (do
      (needs.github.a/createIssue needs.config.cfg.repo x)   ; token brokered, never seen
      (return (audit needs.env.LOG_LEVEL needs.volume.data needs.secret.tok)))
}
```

**Access — `needs.<kind>.<alias>`.**

- **integrations** — `needs.github.a`; call its functions `(needs.github.a/createIssue …)`. The
  secret is **brokered** — the body never references the token.
- **config / secret / volume / env** — by path: `needs.config.cfg.repo`, `needs.secret.tok`,
  `needs.volume.data`, `needs.env.LOG_LEVEL`.
- **short alias** — no new syntax; `let` binds a local name: `(let (gh needs.github.a)
  (gh/createIssue …))`.

**Objects — the project side.** A project defines the concrete objects a slot can bind to:

```
object github-work github {
  token = vault://gh/work      ; secret → a vault reference, never inline
  owner = "me"                 ; config → data
}
```

**`bind` — map slots to objects.** By the run or the project (default), or per-call by a parent:

```
bind { github.a = github-work   github.b = github-personal }     ; run / project
(syncRepo x) with { github.a = github-personal }                 ; per-call override
```

End to end: **`needs` declares typed slots by `kind`+`alias`; you reach them by
`needs.kind.alias`; `bind` wires each slot to a concrete `object`; integrations broker the
secret, everything else injects by path** — declared, injected, least-privilege, enforced.

## Open

- The vault / config-store interface, and the **broker / egress-proxy** interface (how an
  integration declares its endpoints and how the proxy attaches credentials).
- Volume semantics (shared / persistent state) and its relation to stateful streaming.
- Final grammar of the per-call `with { … }` override and the project `object` block.

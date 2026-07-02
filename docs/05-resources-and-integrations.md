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

## Resource addressing: `<object>.<kind>.<key>`

Uniform, with `kind ∈ { secret, config, volume, env }`:

```
github-work.secret.token
github-work.config.owner
db.volume.data
app.env.LOG_LEVEL
```

The same grammar serves **integrations and isolated resources** (an isolated secret is a
standalone object, or `secret.my_key`), and lines up with the `/`-addressing of functions.

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

1. A function declares an **abstract need**: `needs gh: github`, and uses `gh.secret.token`.
2. The **concrete object** is **bound** — by the *run* (default) or by a parent
   (`bind B.gh = github-work`). A reference, not the token.
3. At runtime the engine resolves `github-work.secret.token` from the vault and **injects it
   into the function**. The parent **delegates authority** (chooses which github) **without ever
   holding the value.**

## Two modes, both valid

- **direct** — `github-work.secret.token`: hardcoded to a project object; simple, for
  project-local functions.
- **slot** — `needs gh: github` + `bind`: abstract and reusable; for shared libraries /
  integrations, and for a parent to delegate. The general form.

## Declaration vs injection (provisioning & audit)

- **Needs bubble UP as declarations** — `introspect` shows the whole workflow needs `github` +
  `openai`, so you know what to provision and can audit it.
- **Values inject at the leaves.**
- Declaration aggregates upward; injection happens at the point of use.

## Typing / self-describing

Because the integration TYPE carries a schema, `github-work.secret.token` is **type-checked**:
does it exist? is it a secret? A reference to a resource the integration does not define **fails
at compile** — not in production. Integrations are self-describing and verifiable.

## Open

- Exact syntax: declaring needs (`needs gh: github`), the `bind` form, and a function's resource
  declarations.
- The vault / config-store interface, and the **broker / egress-proxy** interface (how an
  integration declares its endpoints and how the proxy attaches credentials).
- Volume semantics (shared / persistent state) and its relation to stateful streaming.

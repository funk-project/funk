# funk — Broker & Egress (secret protection, by design)

`docs/05` states the principle: **don't give the raw secret to the code, and close the network.**
This document turns that principle into an *implementable architecture* — the interface, the
enforcement mechanism, the threat model, and a phased plan from where the reference CLI is today
(direct-value injection + trace redaction, `docs/05` "What the reference CLI does today").

> Status: **design**. None of Phase 1+ is built yet. This is the contract we build against.

## 1. The problem, precisely

A function body (`src`: python / go / an LLM prompt) may be third-party code. If it *holds* a
credential, nothing stops it from printing, logging, base64-encoding, or POSTing it somewhere.
**Redaction is defense-in-depth, not a guarantee** (`docs/05` D) — adversarial code encodes the
value and escapes any output scan.

So the design does not rely on trusting the body. It removes the two things the body needs to
leak a secret:

1. **the secret** — replace it with a *capability* it cannot read (the **broker**);
2. **the exit** — allow the body to reach *only* the hosts it declared (**egress control**).

Together: **don't (broker) and can't (egress).**

## 2. Threat model

**Defended:**

- A package body exfiltrating a credential it was given to use (it never receives it).
- A body calling an **undeclared** host (e.g. `attacker.com`) — no network route exists.
- An accidental leak into logs / the `RunReport` (redaction, already shipped — `docs/05` D).
- A silently re-pointed dependency (content-hash lockfile — `docs/03` §7, `docs/ROADMAP`).

**Explicitly NOT defended (honest limits):**

- A malicious **daemon / broker / host** — these are the trusted computing base.
- Side channels (timing, DNS-tunnel through a *declared* host, covert channels in allowed
  responses). Least-privilege shrinks blast radius; it is not information-theoretic.
- A user who **declares** `effects { net attacker.com }` and hands that body a raw secret — that
  is a declaration error the architecture surfaces (it is visible in `introspect`), not a bug it
  silently prevents.

The architecture's job: make the *safe* path the *default* path, and make the dangerous
combination (raw secret + open egress) **unnecessary** (broker) or **impossible** (egress-lock).

## 3. The key idea: one network exit

The broker and the egress enforcer are the **same component** — the run's *only* route to the
network. A function's sandbox has no direct internet; its sole outbound path is the broker, which:

- **injects** credentials for **declared integrations** (the body sends a request with no auth;
  the broker attaches it), and
- **allows** only **declared hosts** (`effects { net <host> }`), default-deny everything else.

```
  ┌─────────────────────────────┐        declared creds (never leave here)
  │  function sandbox (per run) │        ┌───────────────────────────────┐
  │  - body: python/go/llm      │        │  broker  (= egress proxy)     │
  │  - NO secrets               │──────▶ │  · matches request → need      │──▶ api.github.com  ✓
  │  - NO direct internet       │  only  │  · attaches Authorization      │──▶ attacker.com    ✗ (undeclared)
  │  - route: broker only       │  path  │  · enforces net allowlist      │
  └─────────────────────────────┘        └───────────────────────────────┘
```

## 4. What drives it — `needs` and `effects` (already in the language)

The broker configuration for a run is **derived**, not hand-written — from the same declarations
`funk introspect` already aggregates up a composite (`docs/05`).

- **`needs { <integration> <alias> }`** (e.g. `github gh`) → a **brokered capability**. The body
  gets a handle (`needs.github.gh`), never the token. The broker holds the credential and
  attaches it when the body calls that integration's host.
- **`needs { secret <alias> }`** → a **raw secret** the body genuinely needs in-process (signing,
  a custom protocol). This is the dangerous case; see §6.
- **`effects { net <host> }`** → the **egress allowlist**. Aggregated up the call graph; the run's
  container may reach exactly this set, nothing else.

Because these aggregate through composition, a composite that calls `createIssue` inherits its
`github` need and `api.github.com` net effect — the broker config is the aggregate at the run root.

## 5. The broker — two calling conventions

**(a) Explicit capability API (simplest to build first).** The body calls a local broker endpoint
instead of the service directly:

```
POST http://broker.local/call
{ "capability": "github.gh", "method": "POST", "path": "/repos/o/r/issues", "body": {…} }
```

The broker matches `github.gh` to the declared `need`, looks up the credential, forwards to
`api.github.com` with the `Authorization` header, and returns the response. The body never holds
the token, and cannot address any other host. Downside: the body is written against the broker,
not the raw SDK.

**(b) Transparent egress proxy (ergonomic end state).** The sandbox has `HTTPS_PROXY=broker.local`
and trusts the broker's CA. The body calls `https://api.github.com/...` *normally* (any HTTP
client / SDK). The broker terminates TLS, recognises the declared host, injects auth, re-encrypts
upstream, and blocks undeclared hosts. Downside: the broker MITMs TLS inside the sandbox (standard
for Secretless-Broker / Vault-Agent-style proxies), so it must be a trusted component — which it
is (it holds the secrets anyway).

**Recommendation:** ship (a) first (no TLS interception, easy to reason about and test), then add
(b) so existing SDK code runs unmodified. Both are the same broker with two front ends.

## 6. The raw-secret tier (`needs { secret … }`)

Some code legitimately needs the bytes (HMAC signing, a non-HTTP protocol). It cannot be brokered.
For it, the guarantee comes from **removing the exit**:

- The secret is injected (env / file, as today — `docs/05`), **but the sandbox has no `net`
  effect ⇒ no network at all** (default-deny with an empty allowlist). Printing the secret is
  *useless*: there is nowhere to send it.
- Pair with **short-lived, narrowly-scoped tokens** (`docs/05` C): a leaked value is useless
  quickly and only for the permitted operation.
- Redaction still masks accidental appearances in the trace (`docs/05` D, shipped).

If a function declares **both** a raw `secret` **and** an open `net` host, that is the one
combination with no guarantee (`docs/05` synthesis). The engine should **warn at `check` time**
("raw secret + network egress: no exfiltration guarantee") — surfacing the risk rather than
hiding it. `funk introspect` already makes the combination visible.

## 7. Egress enforcement — mechanism

Per-run, default-deny, allowlist = aggregated `effects { net <host> }`:

- **Baseline:** the run's container joins a network with **no default route**; the broker is its
  only reachable address. Enforcement lives at the broker (host allowlist by SNI / Host header)
  — so no per-container iptables gymnastics are required.
- **Belt-and-suspenders (later):** a container-level egress policy (docker `--network` + a filtered
  bridge, or nftables) so even a broker bug cannot be bypassed by raw sockets. DNS is served by
  the broker too (a body cannot resolve `attacker.com`).
- **Trust tiers (`docs/03` §7).** Verified code (`funk/std`, signed packages) may run *in* the
  daemon container for speed; unverified packages get their **own** container. Broker + egress
  apply on **either** tier — trust changes *where* the body runs, not *whether* it is confined.

## 8. Secret storage (behind the broker)

The broker resolves a capability to a credential from a **provider**, pluggable:

- **v0.2 dev provider:** the value the CLI already accepts — `--bind`, `@file`, `env:VAR`
  (`docs/05` today). No behaviour change for users; the difference is the *body no longer sees it*.
- **later:** Vault / cloud secret managers / an STS exchange for short-lived tokens (§6, C).

The provider interface is small: `resolve(capability) → {header injection | raw bytes}`. This is
the seam that lets "direct value today" become "vault tomorrow" without touching funk code.

## 9. Phased plan (maps to the reference CLI)

- **Phase 0 — shipped (I21).** Direct-value injection; **trace redaction**; `FUNK_NEEDS` off the
  `docker run` command line. Redaction-grade only.
- **Phase 1 — egress default-deny.** *Partly shipped (I27):* under `--sandbox docker` a body with
  **no `net` effect runs with `--network none`** — the raw-secret tier's guarantee, verified live
  (a no-net body gets `ENETUNREACH`; a net-declared one keeps a network stack). And a `check`-time
  **warning for `secret + net`** (advisory, non-fatal). *Remaining:* host-level allowlisting for
  net-declared bodies (reach *only* the declared hosts) needs the proxy from Phase 2.
- **Phase 2 — broker, explicit API (§5a).** *Minimal version shipped (I28):* a per-run broker
  (`internal/funk/broker.go`) started by `funk run`; `needs { <integration> … }` becomes a
  brokered capability (excluded from `FUNK_NEEDS`), reached via `FUNK_BROKER/call` by alias. The
  broker injects the credential and enforces the `effects{net}` host allowlist. **Verified live:**
  the body runs with `body_has_token=False`, the integration receives the injected `Bearer`, and an
  undeclared host is denied (403). *Limits:* host engine only (routing a `--sandbox docker`
  container to the broker is future); `Bearer` auth scheme; explicit API, not the transparent
  proxy (§5b) yet.
- **Phase 3 — transparent proxy (§5b).** `HTTPS_PROXY` + broker CA, so existing SDK code is
  unmodified.
- **Phase 4 — lifecycle & trust.** Short-lived/scoped tokens (STS/OAuth exchange); wire package
  **signature/verified** state (`docs/03` §7) to execution routing; pluggable vault provider (§8).

Each phase is independently shippable and testable; none breaks a `.funk` file written for an
earlier phase (the declarations are the same — only enforcement tightens).

## 10. Open questions

- **Capability granularity.** Is a `need` scoped to a host (`github`), an endpoint set
  (`github:issues`), or a single operation? Finer scope = smaller blast radius, more declaration.
- **TLS interception policy (§5b).** Ship the broker CA per-run only; never persist it on the host.
- **Non-HTTP integrations** (databases, gRPC, message brokers) — the capability API (§5a)
  generalises; the transparent proxy (§5b) is HTTP-shaped. Which non-HTTP protocols are v1?
- **Response scanning.** Should the broker optionally redact secrets from *responses* too, or is
  that out of scope (the body is allowed to see what it asked a declared host for)?
- **Enterprise split.** The broker is a natural place for an audit log / policy engine — likely
  where a hosted offering adds value, while the interface stays open (`docs/05` "Open — a gift").

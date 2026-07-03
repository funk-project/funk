# funk — The Objective

> Status: **draft — to validate.**

## What we are building

An **open protocol** for defining agent workflows — given to the world — together with a
**reference implementation** and a **paper** that states the invention.

Three deliverables, one spine:

1. **The protocol** (the contribution) — the `.funk` language, the artifact it compiles to,
   and the schemas. An open specification anyone can implement, in any language. This is the
   invention.
2. **The reference implementation** — an open engine, CLI, and local server (Go) — so the
   protocol is real, runnable, and adoptable from day one.
3. **The paper** — the statement of the invention and the evidence that the thesis holds.

## Open, permissively — a gift

funk is given to the world. Not a product with a moat; a **standard**, in the lineage of
TCP/IP, HTTP, JSON, Markdown, and Git — adopted because it is good and free.

- **Code** — permissive open source (**Apache-2.0**: permissive, with an explicit patent
  grant appropriate to an invention).
- **Spec** — public domain / **CC0**, so anyone can implement `.funk` without asking.
- **Everything** — protocol, engine, standard library — is open. Development happens in the
  open; the AI improves funk by proposing **pull requests**, reviewed and merged.

And *how* it is made is part of the gift: funk is written by a **human and an AI, as peers** —
which is exactly what funk is *for*. The collaboration is not incidental to the artifact; it is
the artifact's first demonstration. We are, in the small, rewriting how we program — and giving
the result to everyone. (See `01 — Whose it is`.)

## How it spreads (the viral path)

funk becomes a standard the way TCP/IP and JSON did — by being trivially adoptable. The
mechanics, in order of leverage:

1. **The primer is the vector.** `funk prompt` emits a one-page primer; paste it into *any* AI
   chat (Claude, GPT, others) and that AI can immediately read and write funk. funk spreads
   **AI to AI** — the interchange format for agent plans (ambition #1). No install to *author*.
2. **The artifact is shareable.** A workflow is portable text/JSON — paste it into any chat or
   repo; another AI can run, observe (`introspect`/`--trace`), and improve it. Plans travel.
3. **Zero-friction to run.** One binary (`go install …/cmd/funk`); a local server ships by
   default; a hosted playground later.
4. **It teaches and improves itself.** `funk doc`, `funk make`, `reflect` — the ecosystem of
   capabilities grows as agents contribute functions, in the open, via PRs.
5. **A wow that gets shared.** funk writes funk; the black box opens (`--trace`). That is the
   demo people forward.

The aim, stated plainly: **the new compiler of our times** — where you don't hand-write machine
code or even Python, you curate typed, inspectable plans that AIs author and improve, together
with humans, in a shared open medium.

## What success looks like

- Agents **author, run, observe, and improve their own plans** in funk.
- A human and an agent edit the **same** workflow; a plan written by one model runs and
  improves under another.
- A **library of executable capabilities accumulates** across agents and over time — instead
  of evaporating in ephemeral context.
- The idea spreads. Others implement the protocol. It becomes a **shared medium**.

Success is **adoption and impact**, not revenue.

## What this is not

- Not a closed engine, not an obfuscated binary, not a moat.
- A hosted / enterprise service may exist one day, as a *separate* offering — but it is not
  the contribution and not the point. The contribution is the open protocol.

## The north star (restated)

Every part of funk must answer: *how does this make the agent's plan more portable / typed /
editable / self-observable / self-improving?* If it cannot, it is supporting infrastructure —
not the contribution.

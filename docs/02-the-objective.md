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

# funk — The Idea

> Status: **draft — to validate.** The thesis is Bruno's and is locked; this document
> restates it, in English, as the foundation of the new project. Proposals by the
> assistant are marked as such and are never presented as fact.

## In one line

**funk is a language and protocol for defining workflows** — and, at its core, for turning
the execution plan of an (AI) agent into a **first-class artifact**: portable, typed,
editable, self-observable, and reactive.

## The problem

Today, when an LLM agent decides *what to do* — run A then B, some steps in parallel,
branch on a condition, loop, spin up helpers — that plan exists only **implicitly**: inside
the model's context, as ephemeral prose and opaque tool-calls. It is not an object.

Because it is not an object, it cannot be **inspected, edited, re-run, versioned, ported,
or tested** outside the agent's loop — and, critically, **the agent cannot observe itself
running.** The plan is the most important thing the agent produces, and it is thrown away.

## The invention

funk makes that plan a **first-class artifact**: a `workflow`, written in a small,
homoiconic, typed notation (`.funk`), that compiles to a portable, self-describing object —
a graph *derived from* the code, not drawn by hand.

That artifact has five properties. Each is a requirement, not a feature:

1. **Portable** — it runs on any conformant runtime / container. Not locked to a vendor,
   not "your plan only runs inside my agent."
2. **Typed** — inputs and outputs carry schemas; dependencies are declared and checked.
3. **Editable** — by human *and* by agent, as notation and visually. The same object, two
   authors.
4. **Self-observable** — the agent can watch itself execute, over **structure** (a typed,
   streamed trace), not over text. The plan is data the agent can read *about itself*.
5. **Reactive** — functions communicate as **streams** (everything is a stream;
   `onNext` / `onComplete` / `onError`). Pipelines can run **live and indefinitely**, not
   only as one-shot jobs.

## The kernel: it improves and defines itself

Because the plan is a **structured artifact the agent reads**, the agent can **reason about
its own execution and improve it** — introspecting its plan and its run *as data*, and then
rewriting itself in the same notation.

And the protocol **defines itself**: the standard library is written in `.funk`; the engine
uses `.funk` internally; higher-level functions are just workflows composed of smaller ones.
The native kernel shrinks to the irreducible. **funk makes itself.**

This is the deepest claim, and the hardest: a workflow language whose programs are agents'
own plans, which the agents — and the language itself — build, observe, and improve.

## Why it is new (vs prior art)

- **Agent frameworks** (LangGraph, …) run graphs the *developer* writes — not the
  materialization of the agent's *emergent* plan, and not stream-native.
- **Workflow engines** (Argo, Temporal, CWL, …) are typed and portable, but the plan is
  neither *authored by* nor *observable to* the agent, and they are request/response, not
  reactive streams.
- **MCP** exposes tools to a model; it does not make the *plan* an artifact.

The novelty is the **combination**: an agent's plan, **materialized live**, portable +
typed + editable + self-observable + **reactive** — and **self-improving**.

## Why now

LLMs are finally good enough to author *and* read this notation fluently. And the agent
ecosystem lacks what it most needs: a **shared, inspectable, portable medium** for the work
agents do — a common artifact that a human can edit, an agent can run and observe, and both
can improve, while the library of reusable capabilities grows.

## The north star

Every part of funk must answer one question:

> *How does this make the agent's plan more portable / typed / editable / self-observable /
> self-improving?*

If it cannot, it is supporting infrastructure — not the contribution.

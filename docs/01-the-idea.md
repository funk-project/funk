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

## The ambition — what this could become

> Marked as **vision, not claim.** These are the directions in which funk could become not
> "a better workflow tool" but something new. The core above stands on its own; these are
> what it could grow into.

1. **A lingua franca between AIs (and humans).** The workflow is model-neutral: a plan Claude
   writes, GPT runs and observes, a local model improves, and a human edits — *the same
   object*. Today agents cannot interoperate; each lives in its own context window. funk could
   be the **TCP/IP of agent plans** — the medium where different AIs hand off, co-edit, and
   improve work together. (And the human is *just another stream* — a peer, not an external
   operator.)

2. **Intelligence that accumulates and composes.** Today each agent re-derives everything in
   ephemeral context; the work evaporates. Here, every function and workflow an agent creates
   persists — portable, typed, verifiable, **composable, with provenance** — and improves
   itself. A **library of executable capabilities that grows across agents and over time**
   (an npm / Docker Hub for AI capabilities, but self-improving). Intelligence *compounds*
   instead of being lost.

3. **Trust, verifiability, capability-security.** Because it is typed, structured, and
   reproducible — and because **effects are declared, sandboxed, and permissioned** — you can
   **prove what an agent did** and run plans safely (this workflow may only touch these
   resources). Today, autonomous AI in production is an act of faith. funk could be the
   **trust layer** that lets it in for real. *(Not by accident, this is also the enterprise
   moat.)*

4. **Git for agent execution.** Because a run is a structured, streamed artifact, every
   execution is **reproducible, forkable, diffable**. Time-travel, deterministic replay, and
   *"why did the agent do X?"* answered by **inspecting structure** — not guesswork. Debugging
   and auditing AI like `git`, which is impossible today with opaque prose and tool-calls.

5. **A self-optimizing runtime.** Beyond the agent rewriting the plan, the **engine** observes
   execution (streams, latencies, costs) and **re-plans on its own**: placement, parallelism,
   caching, model selection. The workflow gets faster and cheaper **without anyone touching
   it** — a *query planner* for agent plans. The plan is declarative; the engine finds the
   best execution.

6. **The plan as the agent's substrate of cognition.** The most radical: the agent does not
   *"emit funk"* — it **thinks in funk**. The typed, inspectable, improvable plan **is where
   the reasoning lives**, not a byproduct of ephemeral chain-of-thought. This reframes what an
   agent *is*: not a text generator with tools, but a planner whose mind is a living,
   editable, self-observable artifact.

## The north star

Every part of funk must answer one question:

> *How does this make the agent's plan more portable / typed / editable / self-observable /
> self-improving?*

If it cannot, it is supporting infrastructure — not the contribution.

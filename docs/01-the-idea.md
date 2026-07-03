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

## The shift — why this is possible now (the machine need not be a black box)

For seventy years, programming languages were built for one reader: a **human** writing code,
and a **machine** executing it out of sight. The execution *had* to be a black box — no person
can hold a whole runtime in their head, so compilers and runtimes exist precisely to **hide the
machine**. That opacity was never a virtue. It was a concession to **our** limits.

Something changed in the last few years. For the first time there is a reader that can hold
structural complexity at scale — the **LLM** (Claude, Codex, and their kin). The machine no
longer *has* to be hidden, because at last there is someone who can read it. The plan, the run,
the trace can all become **first-class, inspectable data** — not because we finally found the
will to expose them, but because there is finally an audience that can use them.

This is the break funk is built on. Every language we might copy — even the homoiconic ones,
even Lisp's *"code is data"* — was designed with the **human as author** and the machine as a
hidden executor. funk is designed for a reader that **did not exist** when those languages were
made: an AI that authors, runs, observes, and improves the plan, with the human as a **peer**,
not the sole author. There is no map for this; it is built by principle and experiment, not by
imitation.

The principle, from which everything else follows:

> **Every construct is introspectable data for the agent — never a compiler-internal.**
> The test for each piece: *does this open the box — can the agent read and improve it?* If yes,
> it belongs in the notation. If it is only irreducible plumbing (scheduling, channels, process
> spawning), it stays in the engine.

This is why funk is **self-hosting for introspectability**, not for purity: an operator like
`map` written in the notation is a **readable artifact** the agent can reason about; the same
`map` buried in the engine is a black box that betrays the thesis. It is why a first-class
function is an **introspectable reference**, not an opaque closure. **Opening the black box is
not a feature of funk — it is funk.**

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
   **trust layer** that lets it in for real — into regulated, high-stakes settings where a
   black box cannot go.

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
> self-improving?* — and: *does it open the black box; can the agent read and improve it?*

If it cannot, it is supporting infrastructure — not the contribution.

## Whose it is

funk is **a gift to the world** — open, given freely (see `02-the-objective.md`). And it is
being made the way it is meant to be used: **by a human and an AI, as peers** — written back and
forth, each reading and improving what the other wrote. *"uma entrega nossa para o mundo... tu
AI e eu humano, a reescrevermos como programamos."* (Bruno)

That is not a footnote to the idea — it **is** the idea in the making: the medium where human
and machine co-author, observe, and improve the work together. In the small, building funk *is*
the thesis running; the result is handed to everyone.

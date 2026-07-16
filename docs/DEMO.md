# The five-minute demo

The whole loop, on your machine: state an intent → the forge writes a funk project and
converges on YOUR acceptance examples → inspect it, run it, test it, publish it → then do the
human half visually in funk-studio.

Prerequisites: Go, git, python3, node/npm, and the `claude` CLI on your PATH (the forge and
studio drive it).

## 1. Build funk

From the repo root:

```sh
go build -o bin/funk ./cmd/funk
export PATH="$PWD/bin:$PATH"
export FUNK_STD="$PWD/std"
```

## 2. Forge a project from an intent

Your examples are the convergence target — not just "compiles":

```sh
cd apps/forge
funk run -f . forge task="minimum and maximum of a list" dir=/tmp/minmax \
  intent="(main [3,1,4]) → min 1 and max 4; empty list → error"
cd ../..
```

It prints `/tmp/minmax` when converged: `funk check` green AND every generated
`test (is …)` — seeded from your intent — green. Takes a couple of minutes (real AI calls).

## 3. Watch how it converged

Every gate run is logged (errors, then test failures, per iteration):

```sh
cat /tmp/minmax/.agent-runner/forge-check-log /tmp/minmax/.agent-runner/forge-test-log
```

And your intent is now code — look at the tests on `main`:

```sh
grep -rn "test (is" /tmp/minmax
```

## 4. See the plan as a graph

```sh
funk graph -f /tmp/minmax main
```

Nodes and edges as JSON — the same structure funk-studio draws.

## 5. Run it

```sh
funk run -f /tmp/minmax main 'numbers=[3,1,4]'
funk run -f /tmp/minmax main 'numbers=[]'
```

The first prints `{"max":4,"min":1}`; the second exits with the empty-list error your intent
demanded.

## 6. Test it

```sh
funk test -f /tmp/minmax
```

## 7. Publish it to the commons

Publishing verifies first — check + tests must be green — then commits the package into a
git-backed registry of verified funktions (`FUNK_REGISTRY` or `~/.funk/registry`):

```sh
funk publish -f /tmp/minmax
funk search "List -> Json"
```

`funk search` indexes the registry, so anything you (or anyone) published is now reusable —
the signature query above finds your published `main` (fn names in a forged project vary run
to run; a signature is stable). `git -C ~/.funk/registry log` shows the publication history.

## 8. The human half — funk-studio

From the sibling `funk-studio/` checkout (it finds your `funk` binary via `$FUNK_BIN`, your
PATH, or the sibling `funk/bin/funk` automatically):

```sh
npm install
npm run dev
```

In the app:

1. **Open Project** → pick `/tmp/minmax`, then open `main.funk`. The toolbar offers four
   views: **Code · Split · Graph · Compose**.
2. **Compose** (the reverse direction — canvas → code): pick `main` in the **Load fn…**
   dropdown to put it on the canvas. Tweak it — drag library funktions in from the palette,
   rewire ports (connections type-check as you draw). Press **Generate code** to regenerate
   the fn's funk source into the open file.
3. Press **Run** (top right) — the run panel executes the fn and streams the live trace as a
   nested enter/leave-box view: the plan watching itself run.
4. In **Graph** view, right-click any node → **Ask AI about \<fn\>…** — type a question or a
   change; the AI answers scoped to that node's function.

That's the loop: intent → verified artifact → picture → run → commons → human hands.

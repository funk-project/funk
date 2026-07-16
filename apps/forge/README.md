# funk/forge — a forge that writes funk, in funk

The forge decomposes a task into small functions, writes them, and gates them
(check + test + src-lint) before shipping. It is **funk building funk** — the
dogfood.

## Layout convention (this repo AND what the forge generates)

The `main` entry lives at the **root**; everything else is grouped by **kind**
in a subfolder. All files share the same `package "funk/forge"`, so functions
call each other **by name, no imports** (funk loads the whole tree as one
package). `use` is only for the std / external packages.

```
forge.funk            # the main orchestrator (root)
scaffold/             # laying out & writing files
  slug.funk           #   name → filename stem
  scaffoldPath.funk   #   (kind, name) → project-relative path  ← the layout policy
skills/               # engine=claude — the "thinking" steps (decompose, write, review)
gates/                # deterministic verification (parse/check, test, src-lint)
types/                # shared types (FnSpec, Plan, GateReport)
```

The same shape is what `scaffoldPath` produces for a **generated** project:
`main` at the root, `skills/…`, `gates/…`, etc. — so every forged program reads
the same way.

## Conventions

- **One idea per file, small functions** — shallow nesting (no parenthesis wall),
  and each file is testable/writable in isolation (parallel-friendly).
- **`main` on its own line** — `fn f {\n  main\n  in …` (a funk parser quirk drops
  inputs if `main` shares a line with `in`; `funk fmt` keeps them apart).
- **Every function carries `test (is …)`** where it can be exercised
  deterministically — `funk test` is the safety net.
- **Indent embedded `src`** to match the surrounding funk — a triple-quoted
  string is **dedented** (the common leading whitespace is stripped, relative
  indentation preserved), so the engine still gets clean code:

  ```
  src """
      import textwrap
      try:
          return compile(...)
      except SyntaxError:
          return False
      """
  ```

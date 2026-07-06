# Editor support for funk

Syntax highlighting (and bracket matching / comment toggling) for `.funk` files.
The grammar lives in `vscode/` as a TextMate grammar — the same files drive **VS Code**,
**IntelliJ / JetBrains IDEs**, Sublime, and (later) GitHub Linguist.

- `vscode/syntaxes/funk.tmLanguage.json` — the grammar
- `vscode/language-configuration.json` — comments (`;`), brackets, auto-closing
- `vscode/package.json` — the manifest (language id + grammar wiring)

It highlights: `fn`/`type`/`package` blocks; fields (`doc`, `name`, `examples`, `main`,
`alias`, `in`, `out`, `use … as`, `needs`, `effects`, `test`, …); core forms
(`if`/`let`/`while`/`for-each`/`on-error`/`retry`/`with`/`set`/`flush`/…); stream operators
(`map`/`filter`/`scan`/`fold`/`window`/`each`/`yield`/…); types and port specs
(`min`/`max`/`default`/`zip`/`latest`) and window modifiers (`every`/`by`/`lateness`); the alias
in a qualified cross-package call (`(maths.add …)`); and **triple-quoted `"""…"""` doc blocks
with embedded markdown**.

## IntelliJ / GoLand / any JetBrains IDE

The bundled **TextMate Bundles** plugin reads this folder directly — no custom plugin needed.

1. **Settings → Editor → TextMate Bundles**
2. Click **+** and select this repo's `editors/vscode` folder.
3. **Apply**. Open any `.funk` file — it is now highlighted, with `;` comment toggling
   (⌘/) and `(` `{` matching.

If you don't see *TextMate Bundles* in Settings, enable it under **Settings → Plugins**
(it ships with the IDE; just toggle it on).

> Why not `.idea/`? IDE project config is git-ignored, so it wouldn't reach contributors.
> A TextMate grammar is committed, shared, and reusable across every editor.

### Bonus: validation + format on save (the "schema" part)

Highlighting shows `.funk` *as code*; to also **check** and **format** on save, add a
File Watcher that shells out to the CLI. `funk check` reports every problem as
`path:line:col: message`, so with the output filter below each error becomes **clickable**
— it jumps straight to the offending line.

1. **Settings → Tools → File Watchers → +** (custom).
2. Configure:
   - **File type:** `funk` · **Scope:** Project Files
   - **Program:** `$ProjectFileDir$/bin/funk`
   - **Arguments:** `check -f $FilePath$` — surface parse/type errors on every save.
     (Use a second watcher with `fmt -w $FilePath$` if you also want canonical formatting.)
   - **Output filters:** `$FILE_PATH$:$LINE$:$COLUMN$: $MESSAGE$`
3. Make sure `bin/funk` is built (`go build -o bin/funk ./cmd/funk`).

A real structural language server (`funk lsp`) is the longer-term path — one LSP gives
inline diagnostics in *every* editor. The `path:line:col` diagnostics above are the first
step toward it: the position data now rides on every AST node.

## VS Code

For local development, symlink (or copy) the extension folder into your extensions dir:

```sh
ln -s "$(pwd)/editors/vscode" ~/.vscode/extensions/funk-lang-0.0.3
```

Reload VS Code (**Developer: Reload Window**). To publish it as a real extension later,
`npx vsce package` inside `editors/vscode`.

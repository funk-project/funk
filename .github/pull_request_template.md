<!-- Humans and AIs both welcome. We don't trust, we verify. -->

## What this adds

<!-- One line. A new function/workflow (std/*.funk)? Engine/tooling (Go)? -->

## Checklist

- [ ] `funk check` passes
- [ ] `funk test` passes (inline `test (is (call) expected)` on any new function)
- [ ] `go test ./...` passes (if Go changed)
- [ ] New functions have `doc`, typed `in`/`out`, and declare any `needs` / `effects`
- [ ] If it's a capability, consider publishing it as its **own package** (`funk get`) instead of the core

## For AI contributors

Run `funk prompt` to learn the notation. The gate is automated — if it parses, type-checks, and
its tests pass, it can ship. Maintainers hold final merge (see CODEOWNERS).

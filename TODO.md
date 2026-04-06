# CLI Test Suite — Failure Analysis

Current results: **393 pass / 318 skip / 716 fail** across 9 test suites (1427 total).

Run `mise run test-cli` to re-run. Run `make classify-failures` to re-classify.

## Failure breakdown

| Category | Count | Description |
|---|---|---|
| skipped | 318 | Known unsupported (compat extensions, unimplemented commands) |
| pass | 393 | Tests passing |
| fail | 716 | Remaining failures (CLI output/exit code/fatal errors) |

## go-git library bugs

These require changes to the go-git library, not the CLI shim.
See `TODO-go-git.md` for the full upstream wishlist.

### Config parser rejects valid git config syntax

**Impact: largest single root cause across multiple suites**

go-git's config parser is stricter than real git:

1. **`illegal character U+002F '/'`** (~119 hits, t5516-fetch-push, t0001-init)
   Rejects `/` in config values like URLs.

2. **`expected section name`** (~28 hits, t7508-status)
   Rejects config files with multiline values or unusual section syntax.

3. **`expected EOL, EOF, or comment`** (~19 hits, t3200-branch)
   Similar strictness on config written by real git.

Note: `branch config: invalid merge` (~55 hits) was fixed upstream in
go-git/go-git#1923, awaiting next v6 release.

### SHA-256 compat object format (~209 skipped tests, t1006-cat-file)

`extensions.compatobjectformat` requires dual-hashing (SHA-1 + SHA-256)
on every write to maintain the translation table. go-git correctly rejects
these repos since it only supports SHA-1. These tests are now skipped.
See https://github.com/go-git/go-git/issues/1863

### Other go-git issues

- Tag already exists during fetch (~9 hits) — should silently skip
- Infinite recursion advertising refs (~7 hits) — symbolic ref loops
- Invalid reference name validation (~5 hits) — too strict
- Chroot boundary crossed (~4 hits) — billy filesystem issue
- Malformed refspec parsing (~3 hits) — rejects valid patterns

## CLI shim issues

### Output format mismatches (502 hits — largest category)

Most failures are output formatting differences. Common patterns:
- `git status` output format details (porcelain, short, long modes)
- `git log` / `git show` formatting differences
- `git branch` output formatting
- `git tag` list formatting
- `git cat-file` output differences
- Missing/extra whitespace, different ref formatting

### Wrong exit codes (103 hits)

Commands succeed when they should fail or vice versa. Common patterns:
- Commands that should fail on invalid input but silently succeed
- Missing error detection for edge cases
- `test_must_fail` expects non-zero but we return 0

### Orphan HEAD handling (~19 hits, t3200-branch)

Operations like `repo.Head()` fail on orphan branches where HEAD points
to an unborn ref. Need orphan-aware handling in branch/checkout/commit.

### URL-based push (~12 hits, t5516-fetch-push, t5510-fetch)

Push with a URL/path instead of a named remote. Fetch handles this
but push still expects named remotes.

### Tag message required (~7 hits, t7004-tag)

`git tag -a` without `-m` should open an editor or accept empty message.
Currently errors with "message field is required".

## Unimplemented commands still hit by tests

| Command | Hits | go-git API |
|---|---|---|
| bundle | 5 | none |
| stash | 2 | none |
| rebase | 2 | none |
| fast-import | 2 | none |
| commit-graph | 1 | none |

## Per-suite results

| Suite | Pass | Skip | Fail | Total |
|---|---|---|---|---|
| t0000-basic | 35 | 2 | 55 | 92 |
| t0001-init | 37 | 8 | 46 | 91 |
| t1006-cat-file | 64 | 209 | 65 | 338 |
| t1500-rev-parse | 48 | 2 | 29 | 79 |
| t3200-branch | 27 | 4 | 135 | 166 |
| t5510-fetch | 82 | 7 | 99 | 188 |
| t5516-fetch-push | 3 | 14 | 103 | 120 |
| t7004-tag | 51 | 57 | 120 | 228 |
| t7508-status | 46 | 15 | 64 | 125 |

# CLI Test Suite — Failure Analysis

Current results: **391 pass / 67 skip / 975 fail** across 9 test suites (1433 total).

Run `mise run test-cli` to re-run. Run `make classify-failures` to re-classify.

## Failure breakdown

| Category | Count | Description |
|---|---|---|
| output-mismatch | 502 | CLI output format differs from real git |
| fatal-error | 217 | go-git library error or CLI crash |
| exit-code | 103 | Command ran but returned wrong exit code |
| other | 95 | Uncategorized |
| test-framework | 41 | git's test harness subtests (t0000) |
| unimplemented | 17 | Missing command (skip-list candidate) |

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

### unknown extension: compatobjectformat (~89 hits, t1006-cat-file)

go-git errors on repos with `extensions.compatobjectformat` (SHA-256 compat).
Should ignore unknown extensions gracefully.

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
| t0000-basic | 36 | 0 | 56 | 92 |
| t0001-init | 37 | 4 | 50 | 91 |
| t1006-cat-file | 56 | 0 | 288 | 344 |
| t1500-rev-parse | 48 | 0 | 31 | 79 |
| t3200-branch | 27 | 0 | 139 | 166 |
| t5510-fetch | 84 | 0 | 104 | 188 |
| t5516-fetch-push | 5 | 0 | 115 | 120 |
| t7004-tag | 52 | 55 | 121 | 228 |
| t7508-status | 46 | 8 | 71 | 125 |

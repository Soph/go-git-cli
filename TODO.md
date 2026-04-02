# CLI Test Suite — Remaining Fatal Errors

Analysis of the 226 `fatal-error` failures across all 9 test suites.
These are cases where go-git returns an unexpected error or the CLI crashes.
Grouped by root cause, with estimated difficulty and impact.

Run `make classify-failures` to regenerate the breakdown.

## go-git library bugs

These require changes to the go-git library, not the CLI shim.

### Config parser rejects valid git config syntax

**Impact: ~195 failures across 4 suites (largest single root cause)**

go-git's config parser is stricter than real git. Three distinct issues:

1. **`branch config: invalid merge` (55 hits, t3200-branch)**
   `config/branch.go:47` — `Branch.Validate()` rejects merge values that don't
   start with `refs/`. Real git allows any value during config read; it only
   validates when the value is actually used. Tests write `branch.main.merge = foo`
   and then every subsequent go-git operation fails to open the repo.
   **Fix: ~3 lines** — remove or relax the merge prefix check in `Validate()`.

2. **`illegal character U+002F '/'` (89+24+6 hits, t5516-fetch-push, t0001-init)**
   The config parser rejects lines containing `/` in unexpected positions.
   Likely triggered by URL values or path-like config entries that real git handles.
   **Fix: needs investigation** — trace the exact config line that triggers this.

3. **`expected section name` (28 hits, t7508-status)**
   Config parser fails on config files that contain lines real git wrote and can
   parse. Likely an issue with multiline values or unusual section syntax.
   **Fix: needs investigation** — trace the exact config content.

4. **`expected EOL, EOF, or comment` (14+5 hits, t3200-branch)**
   Similar config parser strictness. The config file was written by real git
   commands in the test but go-git can't re-read it.
   **Fix: needs investigation** — same class of issue as above.

### unknown extension: compatobjectformat (89 hits, t1006-cat-file)

go-git errors when opening a repo whose config has `extensions.compatobjectformat`.
This is a git extension for SHA-256 compat mode. go-git should either support it
or ignore unknown extensions gracefully.
**Fix: ~5 lines** — skip unknown extensions instead of erroring.

### tag already exists during fetch (9 hits, t7004-tag)

go-git's fetch returns `ErrTagExists` when fetching tags that already exist in
the destination. Real git silently skips them. The CLI works around this with a
string match, but the library should handle it natively.
**Fix: in `remote.go`** — skip existing tags during ref update instead of erroring.

### advertising references: max. recursion level (7 hits, t3200-branch)

go-git hits infinite recursion when advertising refs during fetch. Likely a
symbolic ref loop or similar edge case in the test repo.
**Fix: needs investigation** — reproduce and trace the recursion.

### chroot boundary crossed (2+2 hits, t5510-fetch, t5516-fetch-push)

go-git's billy filesystem errors when a path resolves outside the chroot.
This happens during fetch/clone with certain path configurations.
**Fix: needs investigation** — may be a billy bug or a path resolution issue.

### malformed refspec (3 hits, t5510-fetch)

go-git rejects refspecs that real git accepts. Likely edge cases in refspec
syntax (negation, globbing patterns).
**Fix: needs investigation** — check which refspec format is rejected.

### invalid reference name (5 hits, t7004-tag, t0001-init)

go-git rejects ref names that real git allows, such as `refs/tags/v*` or
`refs/heads/with space`. go-git's ref name validation is stricter.
**Fix: ~10 lines** — relax ref name validation rules.

## CLI shim issues

These can be fixed in the CLI shim code without changing go-git.

### not a valid object name: 'HEAD' (19 hits, t3200-branch)

Happens on orphan branches where HEAD points to an unborn branch (no commits).
Operations like `repo.Head()` fail because the target ref doesn't exist yet.
The CLI should detect orphan HEAD state and handle it gracefully.
**Fix: medium** — add orphan-aware HEAD handling in branch/checkout/commit.

### not a valid object name: 'main' / 's' / 'HEAD^' (10 hits, t3200-branch)

`ResolveRevision` fails on valid rev-parse syntax. `HEAD^` should work but
doesn't — likely a go-git revision parser limitation. `main` fails when
the branch was renamed or deleted by a previous test.
**Fix: varies** — `HEAD^` is likely a go-git limitation, others are test state.

### remote not found (6+6 hits, t5516-fetch-push, t5510-fetch)

Fetch/push commands receive a URL or path instead of a named remote.
The CLI now handles URL-based fetch but push may still expect named remotes.
**Fix: small** — add URL-based push support (same pattern as fetch).

### message field is required (7 hits, t7004-tag)

`git tag -a` for annotated tags requires a message. Some tests expect the
command to open an editor or read from stdin; our CLI just errors.
**Fix: small** — default to empty message or read from stdin when `-m` not given.

### couldn't find remote ref (3 hits, t5510-fetch)

Fetch for a specific ref that doesn't exist on the remote. Might be a CLI
issue (wrong refspec construction) or go-git returning wrong error.
**Fix: needs investigation**.

## Summary table

| Root cause | Hits | Type | Effort |
|---|---|---|---|
| Config: invalid merge validation | 55 | go-git bug | ~3 lines |
| Config: illegal character '/' | 119 | go-git bug | investigate |
| Config: expected section name | 28 | go-git bug | investigate |
| Config: expected EOL/EOF | 19 | go-git bug | investigate |
| unknown extension: compatobjectformat | 89 | go-git bug | ~5 lines |
| Orphan HEAD handling | 19 | CLI shim | medium |
| tag already exists on fetch | 9 | go-git bug | small |
| advertising refs recursion | 7 | go-git bug | investigate |
| Remote not found (URL push) | 12 | CLI shim | small |
| message field required | 7 | CLI shim | small |
| Rev-parse limitations | 10 | mixed | varies |
| Invalid ref name validation | 5 | go-git bug | ~10 lines |
| chroot boundary crossed | 4 | go-git/billy | investigate |
| malformed refspec | 3 | go-git bug | investigate |

**Quick wins (< 10 lines each):** invalid merge validation, compatobjectformat extension.
These two alone would fix ~144 failures.

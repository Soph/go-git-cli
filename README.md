# git-cli

A drop-in replacement for the `git` command-line interface, built entirely on top of [go-git](https://github.com/go-git/go-git). This is **not** intended as a production Git client — its purpose is to validate the go-git library implementation against the official Git CLI by running Git's own test suite against it.

## How it works

The binary implements Git subcommands (init, commit, push, log, etc.) by translating CLI flags and arguments into go-git library calls. It uses a multicall binary pattern — a single `git` binary plus `git-<command>` symlinks — so it can be swapped into Git's test harness transparently.

### Implemented commands

add, branch, cat-file, checkout, clone, commit, commit-tree, config, fetch, for-each-ref, hash-object, init, log, ls-files, ls-tree, pack-refs, push, read-tree, reflog, remote, reset, rev-parse, show, show-ref, status, switch, symbolic-ref, tag, update-index, update-ref, version, worktree, write-tree

### Not yet implemented

diff, diff-files, diff-index, merge, pull, rm

## Building

```
make build
```

This compiles the binary to `build/bin/git` and runs `git install` to create the symlinks.

## Testing against upstream Git

The test infrastructure clones upstream Git and runs its shell-based test suite with this binary substituted in:

```
# Run all test suites
make test-cli

# Run a specific test file
make test-cli T=t0001-init.sh

# Verbose output for a single test
make test-cli-verbose T=t0001-init.sh
```

Requires an internet connection on first run to clone the upstream Git source (cached in `.git-dist/`).

## Requirements

- Go 1.24+
- A local clone of [go-git](https://github.com/go-git/go-git) at `../go-git` (see `replace` directive in `go.mod`)

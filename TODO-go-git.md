# go-git upstream wishlist

Issues and missing features in go-git that limit the CLI.

## Merge

- [ ] More than fast-forward merge strategy (recursive/ort)
- [ ] Squash merge support
- [ ] No-ff merge (create merge commit even when ff is possible)

## Config parser

- [x] ~~Rejects valid `branch.*.merge` values not starting with `refs/`~~ — fixed upstream (go-git/go-git#1923), awaiting next v6 release
- [ ] Rejects `/` in config values like URLs (~119 test failures)
- [ ] Rejects valid section syntax — `expected section name` (~28 test failures)
- [ ] Rejects valid lines — `expected EOL, EOF, or comment` (~19 test failures)

## Extensions & compatibility

- [ ] Errors on `extensions.compatobjectformat` instead of ignoring unknown extensions (~89 test failures)
- [ ] Ref name validation too strict — rejects names real git allows (~5 test failures)
- [ ] Refspec parsing rejects valid patterns (~3 test failures)

## Fetch / push

- [ ] `ErrTagExists` on fetch when tag already exists — real git silently skips
- [ ] Infinite recursion advertising refs with symbolic ref loops (~7 test failures)
- [ ] Chroot boundary crossed during fetch/clone (~4 test failures)

## Worktree

- [ ] `Worktree.Add()` has no force option

## Bundle

- [ ] No bundle support (`git bundle create` / `git bundle unbundle`) — needs packfile generation with prerequisite header

## Fsck / object integrity

- [ ] No API to get raw object bytes for re-hashing (needed to verify SHA matches content)
- [ ] No built-in fsck / object integrity check method

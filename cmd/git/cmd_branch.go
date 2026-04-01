package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdBranch(args []string) int {
	var (
		doDelete       bool
		doMove         bool
		force          bool
		showCurrent    bool
		createReflog   bool
		setUpstreamTo  string
		unsetUpstream  bool
		positional     []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-d", "--delete":
			doDelete = true
		case "-D":
			doDelete = true
			force = true
		case "-m", "--move":
			doMove = true
		case "-M":
			doMove = true
			force = true
		case "-f", "--force":
			force = true
		case "--show-current":
			showCurrent = true
		case "-l", "--list":
			// default behavior
		case "-a", "--all":
			// show all (including remotes) — simplified
		case "-v", "--verbose":
			// accepted, ignored
		case "-q", "--quiet":
			// accepted, ignored
		case "--create-reflog":
			createReflog = true
		case "--set-upstream-to", "-u":
			i++
			if i < len(args) {
				setUpstreamTo = args[i]
			}
		case "--unset-upstream":
			unsetUpstream = true
		default:
			if strings.HasPrefix(a, "--set-upstream-to=") {
				setUpstreamTo = strings.TrimPrefix(a, "--set-upstream-to=")
			} else if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	repo := openRepoOrDie()

	if showCurrent {
		head, err := repo.Head()
		if err == nil && head.Name().IsBranch() {
			fmt.Println(head.Name().Short())
		}
		return 0
	}

	if setUpstreamTo != "" {
		return branchSetUpstream(repo, setUpstreamTo, positional)
	}
	if unsetUpstream {
		return branchUnsetUpstream(repo, positional)
	}

	if doDelete {
		for _, name := range positional {
			refName := plumbing.NewBranchReferenceName(name)
			if wtPath := branchCheckedOutIn(repo, refName); wtPath != "" {
				fmt.Fprintf(os.Stderr, "error: cannot delete branch '%s' used by worktree at '%s'\n", name, wtPath)
				return 1
			}
			err := repo.Storer.RemoveReference(refName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: branch '%s' not found.\n", name)
				return 1
			}
			deleteReflog(repo, refName)
			fmt.Printf("Deleted branch %s.\n", name)
		}
		return 0
	}

	if doMove {
		var oldRef, newRef plumbing.ReferenceName
		var hash plumbing.Hash

		if len(positional) == 2 {
			oldRef = plumbing.NewBranchReferenceName(positional[0])
			newRef = plumbing.NewBranchReferenceName(positional[1])

			// Check if the target branch is checked out in a worktree.
			if wtPath := branchCheckedOutIn(repo, newRef); wtPath != "" {
				fmt.Fprintf(os.Stderr, "fatal: '%s' is already checked out at '%s'\n", positional[1], wtPath)
				return 128
			}

			ref, err := repo.Storer.Reference(oldRef)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: branch '%s' not found.\n", positional[0])
				return 1
			}
			hash = ref.Hash()
		} else if len(positional) == 1 {
			newRef = plumbing.NewBranchReferenceName(positional[0])
			// Check if HEAD is a symbolic ref pointing to a branch.
			headRef, err := repo.Storer.Reference(plumbing.HEAD)
			if err != nil || headRef.Type() != plumbing.SymbolicReference || !headRef.Target().IsBranch() {
				fmt.Fprintln(os.Stderr, "fatal: cannot rename the current branch while not on any")
				return 128
			}
			oldRef = headRef.Target()
			head, err := repo.Head()
			if err != nil {
				fmt.Fprintln(os.Stderr, "fatal: not on any branch")
				return 128
			}
			hash = head.Hash()
		} else {
			fmt.Fprintln(os.Stderr, "fatal: too few arguments for branch rename")
			return 128
		}

		newReference := plumbing.NewHashReference(newRef, hash)
		if err := repo.Storer.SetReference(newReference); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		if err := repo.Storer.RemoveReference(oldRef); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}

		// Update HEAD if it pointed to the old branch.
		headRef, _ := repo.Storer.Reference(plumbing.HEAD)
		if headRef != nil && headRef.Type() == plumbing.SymbolicReference && headRef.Target() == oldRef {
			if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, newRef)); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
			msg := fmt.Sprintf("Branch: renamed %s to %s", oldRef, newRef)
			appendReflog(repo, plumbing.HEAD, hash, hash, msg)
		}

		// Update any linked worktree HEADs that pointed to the old branch.
		updateLinkedWorktreeHEADs(oldRef, newRef)

		// Copy reflog from old to new, then delete old.
		deleteReflog(repo, oldRef)
		appendReflog(repo, newRef, plumbing.ZeroHash, hash,
			fmt.Sprintf("Branch: renamed %s to %s", oldRef, newRef))

		return 0
	}

	// Create branch.
	if len(positional) >= 1 {
		name := positional[0]

		// Resolve start point (default: HEAD).
		var targetHash plumbing.Hash
		startName := "HEAD"
		if len(positional) >= 2 {
			startName = positional[1]
			h, err := repo.ResolveRevision(plumbing.Revision(startName))
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: not a valid object name: '%s'\n", startName)
				return 128
			}
			targetHash = *h
		} else {
			head, err := repo.Head()
			if err != nil {
				fmt.Fprintln(os.Stderr, "fatal: not a valid object name: 'HEAD'")
				return 128
			}
			targetHash = head.Hash()
			startName = head.Name().Short()
		}

		refName := plumbing.NewBranchReferenceName(name)

		// Check if branch already exists.
		if _, err := repo.Storer.Reference(refName); err == nil {
			if !force {
				fmt.Fprintf(os.Stderr, "fatal: a branch named '%s' already exists.\n", name)
				return 128
			}
			// --force: refuse to overwrite a branch that is checked out.
			if wtPath := branchCheckedOutIn(repo, refName); wtPath != "" {
				fmt.Fprintf(os.Stderr, "fatal: cannot force update the branch '%s' which is the current branch.\n", name)
				return 128
			}
		}

		ref := plumbing.NewHashReference(refName, targetHash)
		if err := repo.Storer.SetReference(ref); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}

		if createReflog || shouldLogRefUpdates(repo) {
			appendReflog(repo, refName, plumbing.ZeroHash, targetHash,
				fmt.Sprintf("branch: Created from %s", startName))
		}
		return 0
	}

	// List branches.
	head, _ := repo.Head()
	currentBranch := ""
	if head != nil && head.Name().IsBranch() {
		currentBranch = head.Name().Short()
	}

	iter, err := repo.Branches()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	var branches []string
	iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	sort.Strings(branches)

	for _, b := range branches {
		if b == currentBranch {
			fmt.Printf("* %s\n", b)
		} else {
			fmt.Printf("  %s\n", b)
		}
	}

	return 0
}

// branchSetUpstream sets the upstream tracking branch for the current branch
// (or the branch named in positional[0]).
// upstream can be: "origin/main", "upstream", "origin/feature", etc.
func branchSetUpstream(repo interface{}, upstream string, positional []string) int {
	// Determine which branch to configure.
	branchName := ""
	if len(positional) > 0 {
		branchName = positional[0]
	} else {
		r := openRepoOrDie()
		head, err := r.Head()
		if err != nil || !head.Name().IsBranch() {
			fmt.Fprintln(os.Stderr, "fatal: could not set upstream of HEAD when it does not point to any branch.")
			return 128
		}
		branchName = head.Name().Short()
	}

	// Parse the upstream spec. It can be:
	//   "origin/main"  → remote=origin, merge=refs/heads/main
	//   "upstream"     → a local branch, remote=., merge=refs/heads/upstream
	remoteName := "."
	mergeBranch := upstream

	// Check if upstream is a remote tracking ref (remote/branch).
	if idx := strings.IndexByte(upstream, '/'); idx > 0 {
		candidateRemote := upstream[:idx]
		candidateBranch := upstream[idx+1:]
		// Check if this remote exists.
		cfg, err := readRawConfig()
		if err == nil {
			for _, s := range cfg.Sections {
				if s.IsName("remote") {
					for _, ss := range s.Subsections {
						if ss.IsName(candidateRemote) {
							remoteName = candidateRemote
							mergeBranch = candidateBranch
						}
					}
				}
			}
		}
	}

	mergeRef := "refs/heads/" + mergeBranch

	// Write branch.<name>.remote and branch.<name>.merge to config.
	key1 := fmt.Sprintf("branch.%s.remote", branchName)
	key2 := fmt.Sprintf("branch.%s.merge", branchName)
	if rc := configSet(key1, remoteName); rc != 0 {
		return rc
	}
	if rc := configSet(key2, mergeRef); rc != 0 {
		return rc
	}

	return 0
}

// branchUnsetUpstream removes the upstream tracking config for a branch.
func branchUnsetUpstream(repo interface{}, positional []string) int {
	branchName := ""
	if len(positional) > 0 {
		branchName = positional[0]
	} else {
		r := openRepoOrDie()
		head, err := r.Head()
		if err != nil || !head.Name().IsBranch() {
			fmt.Fprintln(os.Stderr, "fatal: could not unset upstream of HEAD when it does not point to any branch.")
			return 128
		}
		branchName = head.Name().Short()
	}

	key1 := fmt.Sprintf("branch.%s.remote", branchName)
	key2 := fmt.Sprintf("branch.%s.merge", branchName)
	configUnset(key1)
	configUnset(key2)

	return 0
}

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdCheckout(args []string) int {
	var (
		create bool
		force  bool
		orphan bool
		branch string
	)

	positional := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-b":
			create = true
			if i+1 < len(args) {
				i++
				branch = args[i]
			}
		case "-B":
			create = true
			force = true
			if i+1 < len(args) {
				i++
				branch = args[i]
			}
		case "--orphan":
			orphan = true
			if i+1 < len(args) {
				i++
				branch = args[i]
			}
		case "-f", "--force":
			force = true
		case "--detach":
			// Accepted — hash checkouts already detach HEAD.
		case "-q", "--quiet":
			// accepted, ignored
		case "-l":
			// accepted, ignored (create reflog — we always do)
		case "--":
			// remaining args are paths, stop flag parsing
			i++
			for i < len(args) {
				positional = append(positional, args[i])
				i++
			}
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	repo := openRepoOrDie()

	// --orphan: create a new branch with no history.
	if orphan && branch != "" {
		ref := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))
		if err := repo.Storer.SetReference(ref); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if create && branch != "" {
		opts := &git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(branch),
			Create: true,
			Force:  force,
		}
		// If a start-point was given (e.g., checkout -b topic HEAD~1), resolve it.
		if len(positional) > 0 {
			h, err := repo.ResolveRevision(plumbing.Revision(positional[0]))
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: not a valid object name: '%s'\n", positional[0])
				return 128
			}
			opts.Hash = *h
		}
		if err := wt.Checkout(opts); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	if len(positional) == 0 {
		return 0
	}

	target := positional[0]

	// Try as branch name first.
	branchRef := plumbing.NewBranchReferenceName(target)
	if _, err := repo.Storer.Reference(branchRef); err == nil {
		opts := &git.CheckoutOptions{
			Branch: branchRef,
			Force:  force,
		}
		if err := wt.Checkout(opts); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	// Try as tag.
	tagRef := plumbing.NewTagReferenceName(target)
	if _, err := repo.Storer.Reference(tagRef); err == nil {
		opts := &git.CheckoutOptions{
			Branch: tagRef,
			Force:  force,
		}
		if err := wt.Checkout(opts); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	// Try as hash (detached HEAD).
	hash, err := repo.ResolveRevision(plumbing.Revision(target))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: pathspec '%s' did not match any file(s) known to git\n", target)
		return 1
	}

	opts := &git.CheckoutOptions{
		Hash:  *hash,
		Force: force,
	}
	if err := wt.Checkout(opts); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

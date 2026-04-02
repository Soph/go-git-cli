package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdMerge(args []string) int {
	var (
		ffOnly  bool
		noFF    bool
		squash  bool
		message string
		quiet   bool
		abort   bool
	)

	positional := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--ff-only":
			ffOnly = true
		case "--no-ff":
			noFF = true
		case "--ff":
			ffOnly = false
			noFF = false
		case "--squash":
			squash = true
		case "--abort":
			abort = true
		case "-m":
			if v, ok := nextVal(args, &i); ok {
				message = v
			}
		case "-q", "--quiet":
			quiet = true
		case "--no-edit":
			// accepted, ignored
		case "--no-stat", "--no-log", "--no-signoff":
			// accepted, ignored
		case "--":
			i++
			for i < len(args) {
				positional = append(positional, args[i])
				i++
			}
		default:
			if strings.HasPrefix(a, "-m") {
				message = a[2:]
			} else if strings.HasPrefix(a, "--strategy=") || a == "-s" {
				// accept strategy flag but only ff is supported
				if a == "-s" {
					i++ // skip strategy name
				}
			} else if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	repo := openRepoOrDie()

	// --abort: remove MERGE_HEAD and reset
	if abort {
		mergeHeadPath := gitDir() + "/MERGE_HEAD"
		if err := os.Remove(mergeHeadPath); err != nil {
			fmt.Fprintln(os.Stderr, "fatal: There is no merge to abort (MERGE_HEAD missing).")
			return 128
		}
		return 0
	}

	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "fatal: No remote for the current branch.")
		return 128
	}

	// Resolve the branch/ref to merge.
	target := positional[0]
	var ref *plumbing.Reference

	// Try as branch name first.
	branchRef := plumbing.NewBranchReferenceName(target)
	if r, err := repo.Storer.Reference(branchRef); err == nil {
		ref = r
	}

	// Try as remote tracking branch.
	if ref == nil {
		remoteRef := plumbing.NewRemoteReferenceName("origin", target)
		if r, err := repo.Storer.Reference(remoteRef); err == nil {
			ref = r
		}
	}

	// Try as tag.
	if ref == nil {
		tagRef := plumbing.NewTagReferenceName(target)
		if r, err := repo.Storer.Reference(tagRef); err == nil {
			ref = r
		}
	}

	// Try as full ref name.
	if ref == nil {
		if r, err := repo.Storer.Reference(plumbing.ReferenceName(target)); err == nil {
			ref = r
		}
	}

	// Try resolving as a revision (hash, HEAD~1, etc).
	if ref == nil {
		h, err := repo.ResolveRevision(plumbing.Revision(target))
		if err != nil {
			fmt.Fprintf(os.Stderr, "merge: %s - not something we can merge\n", target)
			return 1
		}
		ref = plumbing.NewHashReference(plumbing.ReferenceName("refs/merge-target"), *h)
	}

	_ = message // message used for merge commits; ff merges don't create one
	_ = squash  // not supported by go-git yet
	_ = noFF    // not supported by go-git yet

	// go-git currently only supports fast-forward merge.
	opts := git.MergeOptions{
		Strategy: git.FastForwardMerge,
	}

	if err := repo.Merge(*ref, opts); err != nil {
		if ffOnly || (!noFF && !squash) {
			fmt.Fprintf(os.Stderr, "fatal: Not possible to fast-forward, aborting.\n")
			return 128
		}
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if !quiet {
		head, _ := repo.Head()
		if head != nil {
			fmt.Printf("Updating to %s\n", head.Hash().String()[:7])
			fmt.Println("Fast-forward")
		}
	}

	return 0
}

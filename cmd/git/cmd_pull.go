package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdPull(args []string) int {
	var (
		remoteName string
		refspec    string
		ffOnly     bool
		noFF       bool
		rebase     bool
		depth      int
		force      bool
		tags       bool
		noTags     bool
		quiet      bool
	)

	i := 0
	positional := []string{}
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
		case "--rebase", "-r":
			rebase = true
		case "--no-rebase":
			rebase = false
		case "-f", "--force":
			force = true
		case "--tags", "-t":
			tags = true
		case "--no-tags":
			noTags = true
		case "-q", "--quiet":
			quiet = true
		case "--depth":
			if v, ok := nextVal(args, &i); ok {
				if _, err := fmt.Sscanf(v, "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", v)
					return 128
				}
			}
		default:
			if v, ok := strings.CutPrefix(a, "--depth="); ok {
				if _, err := fmt.Sscanf(v, "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", v)
					return 128
				}
			} else if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	if len(positional) >= 1 {
		remoteName = positional[0]
	}
	if len(positional) >= 2 {
		refspec = positional[1]
	}

	_ = ffOnly // go-git pull is always ff-only
	_ = noFF   // not supported by go-git yet
	_ = rebase // not supported by go-git yet
	_ = tags
	_ = noTags

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	// Determine remote name from branch tracking config if not specified.
	if remoteName == "" {
		remoteName = trackingRemote(repo)
	}
	if remoteName == "" {
		remoteName = "origin"
	}

	opts := &git.PullOptions{
		RemoteName: remoteName,
		Force:      force,
		Depth:      depth,
	}

	if refspec != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(refspec)
		opts.SingleBranch = true
	}

	err = wt.Pull(opts)
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			if !quiet {
				fmt.Println("Already up to date.")
			}
			return 0
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

// trackingRemote returns the remote name for the current branch's
// upstream tracking configuration, or "" if none is set.
func trackingRemote(repo *git.Repository) string {
	head, err := repo.Head()
	if err != nil || !head.Name().IsBranch() {
		return ""
	}
	cfg, err := repo.Config()
	if err != nil {
		return ""
	}
	branch, ok := cfg.Branches[head.Name().Short()]
	if !ok || branch.Remote == "" {
		return ""
	}
	return branch.Remote
}

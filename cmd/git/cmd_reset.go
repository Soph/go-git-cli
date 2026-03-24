package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdReset(args []string) int {
	mode := git.MixedReset
	var commitish string
	var files []string
	seenDash := false

	for _, a := range args {
		if seenDash {
			files = append(files, a)
			continue
		}
		switch a {
		case "--hard":
			mode = git.HardReset
		case "--soft":
			mode = git.SoftReset
		case "--mixed":
			mode = git.MixedReset
		case "--merge":
			mode = git.MergeReset
		case "-q", "--quiet":
			// accepted, ignored
		case "--":
			seenDash = true
		default:
			if !strings.HasPrefix(a, "-") {
				if commitish == "" {
					commitish = a
				} else {
					files = append(files, a)
				}
			}
		}
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	opts := &git.ResetOptions{
		Mode:  mode,
		Files: files,
	}

	if commitish != "" {
		h, err := repo.ResolveRevision(plumbing.Revision(commitish))
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: Failed to resolve '%s' as a valid ref.\n", commitish)
			return 128
		}
		opts.Commit = *h
	} else {
		head, err := repo.Head()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		opts.Commit = head.Hash()
	}

	if err := wt.Reset(opts); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdShowRef(args []string) int {
	var (
		showHead  bool
		tagsOnly  bool
		headsOnly bool
		exists    bool
		verify    bool
		quiet     bool
		pattern   string
	)

	for _, a := range args {
		switch a {
		case "--head":
			showHead = true
		case "--tags":
			tagsOnly = true
		case "--heads":
			headsOnly = true
		case "--exists":
			exists = true
		case "-q", "--quiet":
			quiet = true
		case "--verify":
			verify = true
		default:
			if !strings.HasPrefix(a, "-") {
				pattern = a
			}
		}
	}

	repo := openRepoOrDie()

	// --exists or --verify mode: check if a specific ref exists.
	if (exists || verify) && pattern != "" {
		ref, err := repo.Reference(plumbing.ReferenceName(pattern), true)
		if err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "error: '%s' - not a valid ref\n", pattern)
			}
			return 1
		}
		if !quiet {
			fmt.Printf("%s %s\n", ref.Hash(), pattern)
		}
		return 0
	}

	found := false

	if showHead {
		head, err := repo.Head()
		if err == nil {
			fmt.Printf("%s HEAD\n", head.Hash())
			found = true
		}
	}

	refs, err := repo.References()
	if err != nil {
		return 1
	}

	refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()

		if tagsOnly && !strings.HasPrefix(name, "refs/tags/") {
			return nil
		}
		if headsOnly && !strings.HasPrefix(name, "refs/heads/") {
			return nil
		}

		// Skip HEAD (handled separately with --head).
		if ref.Name() == plumbing.HEAD {
			return nil
		}

		// If pattern specified, filter.
		if pattern != "" && name != pattern && !strings.HasSuffix(name, "/"+pattern) {
			return nil
		}

		hash := ref.Hash()
		if ref.Type() == plumbing.SymbolicReference {
			resolved, err := repo.Reference(ref.Name(), true)
			if err != nil {
				return nil
			}
			hash = resolved.Hash()
		}

		fmt.Printf("%s %s\n", hash, name)
		found = true
		return nil
	})

	if !found {
		return 1
	}
	return 0
}

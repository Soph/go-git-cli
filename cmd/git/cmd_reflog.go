package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

func cmdReflog(args []string) int {
	// Default subcommand is "show".
	if len(args) == 0 {
		return reflogShow([]string{"HEAD"})
	}

	subcmd := args[0]
	rest := args[1:]

	switch subcmd {
	case "show":
		return reflogShow(rest)
	case "exists":
		return reflogExists(rest)
	default:
		// If the first arg isn't a known subcommand, treat it as a ref for "show".
		if !strings.HasPrefix(subcmd, "-") {
			return reflogShow(args)
		}
		fmt.Fprintf(os.Stderr, "error: unknown reflog subcommand: %s\n", subcmd)
		return 1
	}
}

func reflogShow(args []string) int {
	ref := "HEAD"
	noAbbrev := false

	positional := []string{}
	for _, a := range args {
		switch a {
		case "--no-abbrev-commit":
			noAbbrev = true
		case "-q", "--quiet":
			// accepted, ignored
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
	}
	if len(positional) > 0 {
		ref = positional[0]
	}

	repo := openRepoOrDie()

	rs, ok := repo.Storer.(storer.ReflogStorer)
	if !ok {
		fmt.Fprintln(os.Stderr, "fatal: reflog is not supported by this storage backend")
		return 128
	}

	refName := resolveRefName(ref)

	entries, err := rs.Reflog(refName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if len(entries) == 0 {
		return 0
	}

	// Reflog entries are stored oldest-first; git shows newest-first.
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		hash := e.NewHash.String()
		if !noAbbrev {
			hash = hash[:7]
		}
		fmt.Printf("%s %s@{%d}: %s\n", hash, ref, len(entries)-1-i, e.Message)
	}

	return 0
}

func reflogExists(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: reflog exists requires a ref argument")
		return 1
	}

	ref := args[len(args)-1]

	repo := openRepoOrDie()

	rs, ok := repo.Storer.(storer.ReflogStorer)
	if !ok {
		return 1
	}

	entries, err := rs.Reflog(plumbing.ReferenceName(ref))
	if err != nil || len(entries) == 0 {
		return 1
	}

	return 0
}

// resolveRefName converts a short ref name to a full reference name.
func resolveRefName(ref string) plumbing.ReferenceName {
	if ref == "HEAD" {
		return plumbing.HEAD
	}
	if strings.HasPrefix(ref, "refs/") {
		return plumbing.ReferenceName(ref)
	}
	return plumbing.NewBranchReferenceName(ref)
}

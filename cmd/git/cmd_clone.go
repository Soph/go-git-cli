package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdClone(args []string) int {
	var (
		bare              bool
		mirror            bool
		depth             int
		branch            string
		origin            string
		noCheckout        bool
		noTags            bool
		shared            bool
		recurseSubmodules bool
	)

	positional := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--bare":
			bare = true
		case a == "--mirror":
			mirror = true
		case a == "--no-checkout", a == "-n":
			noCheckout = true
		case a == "--no-tags":
			noTags = true
		case a == "--shared", a == "-s":
			shared = true
		case a == "--recurse-submodules", a == "--recursive":
			recurseSubmodules = true
		case a == "-b" || a == "--branch":
			if i+1 < len(args) {
				i++
				branch = args[i]
			}
		case strings.HasPrefix(a, "--branch="):
			branch = strings.TrimPrefix(a, "--branch=")
		case a == "-o" || a == "--origin":
			if i+1 < len(args) {
				i++
				origin = args[i]
			}
		case strings.HasPrefix(a, "--origin="):
			origin = strings.TrimPrefix(a, "--origin=")
		case a == "--depth":
			if i+1 < len(args) {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", args[i])
					return 128
				}
			}
		case strings.HasPrefix(a, "--depth="):
			v := strings.TrimPrefix(a, "--depth=")
			if _, err := fmt.Sscanf(v, "%d", &depth); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", v)
				return 128
			}
		case a == "--single-branch":
			// accepted — go-git handles this via Depth or SingleBranch
		case a == "--no-single-branch":
			// accepted, ignored
		case strings.HasPrefix(a, "--template=") || a == "--template":
			if a == "--template" && i+1 < len(args) {
				i++ // skip value
			}
		case a == "--no-local", a == "--no-hardlinks":
			// accepted, ignored — go-git never does hardlinks
		case a == "-q", a == "--quiet":
			// accepted, ignored
		case a == "-v", a == "--verbose":
			// accepted, ignored
		case a == "--progress":
			// accepted, ignored
		case a == "-l", a == "--local":
			// accepted, ignored — go-git auto-detects local
		case a == "--":
			i++
			for i < len(args) {
				positional = append(positional, args[i])
				i++
			}
		case !strings.HasPrefix(a, "-"):
			positional = append(positional, a)
		default:
			// Unknown flag, ignore gracefully.
		}
		i++
	}

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "fatal: You must specify a repository to clone.")
		return 128
	}

	url := positional[0]

	// Determine destination directory.
	var dest string
	if len(positional) >= 2 {
		dest = positional[1]
	} else {
		dest = defaultCloneDir(url, bare || mirror)
	}

	absURL := resolveLocalURL(url)

	opts := &git.CloneOptions{
		URL:        absURL,
		Bare:       bare || mirror,
		Mirror:     mirror,
		NoCheckout: noCheckout,
		Depth:      depth,
		Shared:     shared,
	}

	if origin != "" {
		opts.RemoteName = origin
	}

	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
		opts.SingleBranch = true
	}

	if noTags {
		opts.Tags = plumbing.NoTags
	}

	if recurseSubmodules {
		opts.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}

	// Create destination directory if needed.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: could not create directory '%s': %s\n", dest, err)
		return 128
	}

	_, err := git.PlainClone(dest, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

// defaultCloneDir derives the destination directory from the clone URL,
// matching git's convention: strip trailing slashes, .git suffix, and take
// the basename.
func defaultCloneDir(url string, bare bool) string {
	name := strings.TrimRight(url, "/")
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, ".git")
	if bare {
		name += ".git"
	}
	return name
}

// resolveLocalURL makes a local path absolute so go-git can open it
// regardless of any cwd changes. Remote URLs are returned as-is.
func resolveLocalURL(url string) string {
	// Schemes → remote
	if strings.Contains(url, "://") || strings.HasPrefix(url, "git@") {
		return url
	}
	abs, err := filepath.Abs(url)
	if err != nil {
		return url
	}
	// Verify it looks like a local path (exists on disk).
	if _, err := os.Stat(abs); err == nil {
		return abs
	}
	return url
}

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"path/filepath"
)

func cmdFetch(args []string) int {
	var (
		remoteName string
		all        bool
		tags       bool
		noTags     bool
		prune      bool
		force      bool
		depth      int
		refSpecs   []string
	)

	positional := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--all":
			all = true
		case "--tags", "-t":
			tags = true
		case "--no-tags":
			noTags = true
		case "--prune", "-p":
			prune = true
		case "-f", "--force":
			force = true
		case "-q", "--quiet":
			// accepted, ignored
		case "--depth":
			if i+1 < len(args) {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", args[i])
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
		refSpecs = positional[1:]
	}
	if remoteName == "" && !all {
		remoteName = "origin"
	}

	repo := openRepoOrDie()

	if all {
		remotes, err := repo.Remotes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		for _, r := range remotes {
			opts := &git.FetchOptions{
				RemoteName: r.Config().Name,
				Force:      force,
				Prune:      prune,
			}
			setFetchTags(opts, tags, noTags)
			err := repo.Fetch(opts)
			if err != nil && !isTagExistsOrUpToDate(err) {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
		}
		return 0
	}

	// Check if remoteName is a configured remote or a URL/path.
	isURL := false
	if remoteName != "" {
		if _, err := repo.Remote(remoteName); err != nil {
			// Not a named remote — treat as URL/path.
			isURL = true
		}
	}

	if isURL {
		// Resolve local paths to absolute.
		url := remoteName
		if !strings.Contains(url, "://") && !strings.HasPrefix(url, "git@") {
			if abs, err := filepath.Abs(url); err == nil {
				url = abs
			}
		}

		// Build refspecs for direct URL fetch.
		var rs []config.RefSpec
		for _, r := range refSpecs {
			rs = append(rs, config.RefSpec(r))
		}
		if len(rs) == 0 {
			// Default: fetch all heads + tags.
			rs = append(rs, config.RefSpec("+refs/heads/*:refs/heads/*"))
			rs = append(rs, config.RefSpec("+refs/tags/*:refs/tags/*"))
		}

		remote, err := repo.CreateRemoteAnonymous(&config.RemoteConfig{
			Name: "anonymous",
			URLs: []string{url},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}

		fetchOpts := &git.FetchOptions{
			RemoteName: "anonymous",
			RefSpecs:   rs,
			Force:      force,
			Depth:      depth,
		}
		setFetchTags(fetchOpts, tags, noTags)

		err = remote.Fetch(fetchOpts)
		if err != nil && !isTagExistsOrUpToDate(err) {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	opts := &git.FetchOptions{
		RemoteName: remoteName,
		Force:      force,
		Prune:      prune,
		Depth:      depth,
	}
	setFetchTags(opts, tags, noTags)

	if len(refSpecs) > 0 {
		for _, rs := range refSpecs {
			if !strings.Contains(rs, ":") && !strings.HasPrefix(rs, "refs/") {
				rs = fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", rs, remoteName, rs)
			}
			opts.RefSpecs = append(opts.RefSpecs, config.RefSpec(rs))
		}
	}

	err := repo.Fetch(opts)
	if err != nil && !isTagExistsOrUpToDate(err) {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

func setFetchTags(opts *git.FetchOptions, tags, noTags bool) {
	if noTags {
		opts.Tags = plumbing.NoTags
	} else if tags {
		opts.Tags = plumbing.AllTags
	}
}

// isTagExistsOrUpToDate returns true if the error is a benign fetch result:
// either already up-to-date or a tag that already exists (go-git bug — real
// git silently skips existing tags during fetch).
func isTagExistsOrUpToDate(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return true
	}
	if errors.Is(err, git.ErrTagExists) {
		return true
	}
	// Fallback: string match in case the error is wrapped without %w.
	return strings.Contains(err.Error(), "tag already exists")
}

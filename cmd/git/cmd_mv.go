package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdMv(args []string) int {
	var (
		force   bool
		verbose bool
		dryRun  bool
		paths   []string
	)

	seenDash := false
	for _, a := range args {
		if seenDash {
			paths = append(paths, a)
			continue
		}
		switch a {
		case "--":
			seenDash = true
		case "-f", "--force":
			force = true
		case "-v", "--verbose":
			verbose = true
		case "-n", "--dry-run":
			dryRun = true
		case "-k":
			// skip errors, accepted and ignored (we just keep going)
		default:
			if strings.HasPrefix(a, "-") {
				// ignore unknown flags
			} else {
				paths = append(paths, a)
			}
		}
	}

	if len(paths) < 2 {
		fmt.Fprintln(os.Stderr, "usage: git mv [<options>] <source>... <destination>")
		return 1
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	dest := paths[len(paths)-1]
	sources := paths[:len(paths)-1]

	// If multiple sources, dest must be a directory.
	if len(sources) > 1 {
		info, err := os.Stat(dest)
		if err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "fatal: destination '%s' is not a directory\n", dest)
			return 128
		}
	}

	for _, src := range sources {
		target := dest

		// If dest is a directory, move into it preserving the base name.
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			target = filepath.Join(dest, filepath.Base(src))
		}

		// Check source exists.
		if _, err := os.Stat(src); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: bad source, source=%s, destination=%s\n", src, target)
			return 128
		}

		// Check dest doesn't exist (unless -f).
		if !force {
			if _, err := os.Stat(target); err == nil {
				fmt.Fprintf(os.Stderr, "fatal: destination exists, source=%s, destination=%s\n", src, target)
				return 128
			}
		}

		if dryRun {
			fmt.Printf("Renaming %s to %s\n", src, target)
			continue
		}

		// Move the file on disk.
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		if err := os.Rename(src, target); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: renaming '%s' failed: %s\n", src, err)
			return 128
		}

		// Update the index: remove old path, add new path.
		wt.Remove(src)
		wt.Add(target)

		if verbose {
			fmt.Printf("Renaming %s to %s\n", src, target)
		}
	}

	return 0
}

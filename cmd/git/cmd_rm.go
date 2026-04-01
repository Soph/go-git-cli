package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	goGit "github.com/go-git/go-git/v6"
)

func cmdRm(args []string) int {
	var (
		cached    bool
		force     bool
		recursive bool
		quiet     bool
		paths     []string
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
		case "--cached":
			cached = true
		case "-f", "--force":
			force = true
		case "-r":
			recursive = true
		case "-q", "--quiet":
			quiet = true
		case "-n", "--dry-run":
			// accepted, ignored
		default:
			if strings.HasPrefix(a, "-") {
				// ignore unknown flags
			} else {
				paths = append(paths, a)
			}
		}
	}

	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: git rm [--cached] [-f] [-r] [--] <file>...")
		return 1
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	// Expand directories if -r is given.
	var expanded []string
	for _, p := range paths {
		info, statErr := os.Stat(p)
		if statErr == nil && info.IsDir() {
			if !recursive {
				fmt.Fprintf(os.Stderr, "fatal: not removing '%s' recursively without -r\n", p)
				return 128
			}
			// Walk directory for files.
			err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !fi.IsDir() {
					expanded = append(expanded, path)
				}
				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
		} else {
			expanded = append(expanded, p)
		}
	}

	status, err := wt.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	for _, p := range expanded {
		// Check if file has local modifications (unless --force or --cached).
		if !force && !cached {
			if s, ok := status[p]; ok {
				if s.Worktree == goGit.Modified {
					fmt.Fprintf(os.Stderr, "error: the following file has local modifications:\n    %s\n", p)
					fmt.Fprintln(os.Stderr, "(use --cached to keep the file, or -f to force removal)")
					return 1
				}
			}
		}

		// Remove from index.
		_, err := wt.Remove(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: pathspec '%s' did not match any files\n", p)
			return 128
		}

		if !cached {
			// Also remove from working tree.
			os.Remove(p)
		}

		if !quiet {
			fmt.Printf("rm '%s'\n", p)
		}
	}

	return 0
}

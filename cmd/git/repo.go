package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/go-git/go-billy/v6/osfs"
)

// openRepo opens the git repository at the current directory (or GIT_DIR).
func openRepo() (*git.Repository, error) {
	dir := os.Getenv("GIT_DIR")
	if dir == "" {
		dir = "."
	}
	return git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
}

// openRepoOrDie opens the repository or prints a fatal error and exits.
func openRepoOrDie() *git.Repository {
	r, err := openRepo()
	if err != nil {
		fatal("%s", err)
	}
	return r
}

// formatTime formats a time for git log/show output.
func formatTime(t time.Time) string {
	return t.Format("Mon Jan 2 15:04:05 2006 -0700")
}

// nextVal consumes the next argument as a flag value, advancing *i.
// Returns the value and true, or empty and false if no next arg exists.
func nextVal(args []string, i *int) (string, bool) {
	if *i+1 < len(args) {
		*i++
		return args[*i], true
	}
	return "", false
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// fatal prints a fatal error message to stderr and exits with code 128.
// Note: os.Exit skips deferred calls. This is acceptable for a CLI where
// defers are only closing file handles (reclaimed by the OS anyway).
// Do not add deferred writes (lockfiles, flush) that depend on running.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fatal: "+format+"\n", args...)
	os.Exit(128)
}

// openStorage opens the git storage directly, bypassing Repository.Open()
// and its extension verification. This is useful for read-only commands
// like cat-file that only need object/ref access and can safely operate
// on repos with extensions go-git doesn't fully support.
func openStorage() (*filesystem.Storage, error) {
	gd := gitDir()
	fs := osfs.New(gd)
	return filesystem.NewStorage(fs, cache.NewObjectLRU(cache.DefaultMaxSize)), nil
}

// openStorageOrDie opens the storage or prints a fatal error and exits.
func openStorageOrDie() *filesystem.Storage {
	s, err := openStorage()
	if err != nil {
		fatal("%s", err)
	}
	// Verify it's actually a git repo by checking HEAD exists.
	if _, err := s.Reference(plumbing.HEAD); err != nil {
		fatal("not a git repository (or any parent up to mount point /)")
	}
	return s
}

// resolveOID resolves an object identifier (hash, ref name, HEAD) using
// the storage directly. Simpler than Repository.ResolveRevision but
// sufficient for commands that need basic name-to-hash resolution.
func resolveOID(s *filesystem.Storage, name string) (plumbing.Hash, error) {
	// Try as full hex hash first.
	h := plumbing.NewHash(name)
	if !h.IsZero() {
		if err := s.HasEncodedObject(h); err == nil {
			return h, nil
		}
	}

	// Try HEAD.
	if name == "HEAD" {
		ref, err := s.Reference(plumbing.HEAD)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		if ref.Type() == plumbing.SymbolicReference {
			ref, err = s.Reference(ref.Target())
			if err != nil {
				return plumbing.ZeroHash, err
			}
		}
		return ref.Hash(), nil
	}

	// Try as branch, tag, or full ref name.
	for _, prefix := range []string{
		"refs/heads/",
		"refs/tags/",
		"refs/remotes/",
		"",
	} {
		refName := plumbing.ReferenceName(prefix + name)
		ref, err := s.Reference(refName)
		if err == nil {
			return ref.Hash(), nil
		}
	}

	return plumbing.ZeroHash, errors.New("not a valid object name")
}

// gitDir returns the .git directory path for the current working directory.
// In a bare repository (no .git subdirectory, but HEAD exists), returns ".".
// Note: this duplicates the walk in openRepo/DetectDotGit, but is needed by
// cmd_config.go which reads raw config files without opening a full repo.
func gitDir() string {
	dir := os.Getenv("GIT_DIR")
	if dir != "" {
		return dir
	}
	// Walk up to find .git
	wd, err := os.Getwd()
	if err != nil {
		return ".git"
	}
	for {
		candidate := filepath.Join(wd, ".git")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		// Check for bare repo (HEAD file exists in current dir).
		if _, err := os.Stat(filepath.Join(wd, "HEAD")); err == nil {
			if _, err := os.Stat(filepath.Join(wd, "refs")); err == nil {
				return wd
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return ".git"
}

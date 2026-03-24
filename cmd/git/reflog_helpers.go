package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/reflog"
	"github.com/go-git/go-git/v6/plumbing/storer"
	xstorage "github.com/go-git/go-git/v6/x/storage"
)

// appendReflog appends a reflog entry if the storer supports reflogs.
func appendReflog(repo *git.Repository, name plumbing.ReferenceName, oldHash, newHash plumbing.Hash, msg string) {
	rs, ok := repo.Storer.(storer.ReflogStorer)
	if !ok {
		return
	}

	committerName := os.Getenv("GIT_COMMITTER_NAME")
	if committerName == "" {
		committerName = "Go-git CLI"
	}
	committerEmail := os.Getenv("GIT_COMMITTER_EMAIL")
	if committerEmail == "" {
		committerEmail = "go-git@localhost"
	}

	entry := &reflog.Entry{
		OldHash: oldHash,
		NewHash: newHash,
		Committer: reflog.Signature{
			Name:  committerName,
			Email: committerEmail,
			When:  time.Now(),
		},
		Message: msg,
	}

	rs.AppendReflog(name, entry)
}

// deleteReflog removes the reflog for a reference if the storer supports it.
func deleteReflog(repo *git.Repository, name plumbing.ReferenceName) {
	rs, ok := repo.Storer.(storer.ReflogStorer)
	if !ok {
		return
	}
	rs.DeleteReflog(name)
}

// shouldLogRefUpdates returns true if core.logAllRefUpdates is enabled.
// Git defaults to true for non-bare repos unless explicitly set to false.
func shouldLogRefUpdates(repo *git.Repository) bool {
	cfg, err := repo.Config()
	if err != nil {
		return false
	}

	// Check raw config for explicit setting.
	val := cfg.Raw.Section("core").Options.Get("logallrefupdates")
	switch val {
	case "true", "always":
		return true
	case "false":
		return false
	default:
		// Default: true for non-bare, false for bare.
		return !cfg.Core.IsBare
	}
}

// branchCheckedOutIn returns the worktree path where the given branch is
// checked out, or "" if it is not checked out in any linked worktree.
// It checks both the main worktree HEAD and all linked worktrees under
// .git/worktrees/*/HEAD.
func branchCheckedOutIn(repo *git.Repository, branch plumbing.ReferenceName) string {
	// Check the main worktree's HEAD.
	headRef, _ := repo.Storer.Reference(plumbing.HEAD)
	if headRef != nil && headRef.Type() == plumbing.SymbolicReference && headRef.Target() == branch {
		wd, _ := os.Getwd()
		return wd
	}

	// Check linked worktrees.
	wts, ok := repo.Storer.(xstorage.WorktreeStorer)
	if !ok {
		return ""
	}

	dotgitFS := wts.Filesystem()
	entries, err := dotgitFS.ReadDir("worktrees")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Read HEAD file for this worktree.
		headPath := filepath.Join("worktrees", name, "HEAD")
		f, err := dotgitFS.Open(headPath)
		if err != nil {
			continue
		}
		buf := make([]byte, 4096)
		n, _ := f.Read(buf)
		f.Close()

		content := strings.TrimSpace(string(buf[:n]))

		// HEAD can be a symbolic ref: "ref: refs/heads/foo"
		// or a detached hash.
		if after, ok := strings.CutPrefix(content, "ref: "); ok {
			if plumbing.ReferenceName(after) == branch {
				return linkedWorktreePath(name)
			}
		}
	}

	return ""
}

// updateLinkedWorktreeHEADs updates all linked worktree HEAD files that
// point to oldRef so they point to newRef instead. This is needed when
// renaming a branch that is checked out in linked worktrees.
func updateLinkedWorktreeHEADs(oldRef, newRef plumbing.ReferenceName) {
	gd := gitDir()
	wtDir := filepath.Join(gd, "worktrees")
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return
	}

	oldContent := "ref: " + string(oldRef)
	newContent := "ref: " + string(newRef) + "\n"

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		headFile := filepath.Join(wtDir, entry.Name(), "HEAD")
		data, err := os.ReadFile(headFile)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == oldContent {
			os.WriteFile(headFile, []byte(newContent), 0o644)
		}
	}
}

// linkedWorktreePath resolves a linked worktree's working directory path
// by reading .git/worktrees/<name>/gitdir.
func linkedWorktreePath(name string) string {
	gd := gitDir()
	gitdirFile := filepath.Join(gd, "worktrees", name, "gitdir")
	data, err := os.ReadFile(gitdirFile)
	if err != nil {
		return name
	}
	// gitdir points to <worktree-path>/.git
	wtDotGit := strings.TrimSpace(string(data))
	return filepath.Dir(wtDotGit)
}

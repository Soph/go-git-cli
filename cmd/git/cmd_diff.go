package main

import (
	"fmt"
	"os"
	"strings"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/diff"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdDiff(args []string) int {
	var (
		cached     bool
		stat       bool
		nameOnly   bool
		nameStatus bool
		numStat    bool
		noPatch    bool
		exitCode   bool
		quiet      bool
		revs       []string
		paths      []string
	)

	seenDash := false
	i := 0
	for i < len(args) {
		a := args[i]
		if seenDash {
			paths = append(paths, a)
			i++
			continue
		}
		switch a {
		case "--":
			seenDash = true
		case "--cached", "--staged":
			cached = true
		case "--stat":
			stat = true
		case "--name-only":
			nameOnly = true
		case "--name-status":
			nameStatus = true
		case "--numstat":
			numStat = true
		case "--no-patch", "-s":
			noPatch = true
		case "--exit-code":
			exitCode = true
		case "-q", "--quiet":
			quiet = true
			noPatch = true
		case "--raw":
			// accepted, use name-status as approximation
			nameStatus = true
		case "--diff-filter":
			i++ // skip value
		case "-M", "--find-renames":
			// accepted, ignored (go-git detects renames by default)
		case "-p", "-u", "--patch":
			// default behavior
		case "--no-color":
			// accepted, ignored (we don't color by default)
		case "--color":
			// accepted, ignored
		case "-z":
			// NUL termination, ignored for now
		default:
			if strings.HasPrefix(a, "--diff-filter=") ||
				strings.HasPrefix(a, "--color=") ||
				strings.HasPrefix(a, "-U") ||
				strings.HasPrefix(a, "--unified=") ||
				strings.HasPrefix(a, "--stat=") ||
				strings.HasPrefix(a, "--src-prefix=") ||
				strings.HasPrefix(a, "--dst-prefix=") ||
				strings.HasPrefix(a, "-M") ||
				strings.HasPrefix(a, "--find-renames=") ||
				strings.HasPrefix(a, "--inter-hunk-context=") ||
				strings.HasPrefix(a, "--abbrev") {
				// accepted, ignored
			} else if !strings.HasPrefix(a, "-") {
				revs = append(revs, a)
			}
		}
		i++
	}

	repo := openRepoOrDie()

	var fromTree, toTree *object.Tree

	switch {
	case cached:
		// diff --cached: HEAD vs index (staged changes)
		fromTree = headTree(repo)
		toTree = indexTree(repo)
	case len(revs) == 0:
		// diff: index vs worktree — approximate using HEAD vs worktree status
		return diffWorktree(repo, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet, paths)
	case len(revs) == 1:
		// diff <commit>: commit vs worktree
		fromTree = resolveTree(repo, revs[0])
		// For commit vs worktree, use status-based approach
		return diffCommitVsWorktree(repo, fromTree, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet, paths)
	case len(revs) >= 2:
		// diff <commit1> <commit2>
		fromTree = resolveTree(repo, revs[0])
		toTree = resolveTree(repo, revs[1])
	}

	if fromTree == nil && toTree == nil {
		return 0
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if len(paths) > 0 {
		changes = filterChanges(changes, paths)
	}

	if len(changes) == 0 {
		return 0
	}

	return outputDiff(changes, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet)
}

func cmdDiffFiles(args []string) int {
	// diff-files: index vs worktree (same as diff without --cached)
	return cmdDiff(args)
}

func cmdDiffIndex(args []string) int {
	// diff-index: tree vs index
	newArgs := append([]string{"--cached"}, args...)
	return cmdDiff(newArgs)
}

// headTree returns the tree for HEAD, or an empty tree if no commits.
func headTree(repo *goGit.Repository) *object.Tree {
	head, err := repo.Head()
	if err != nil {
		// No commits yet — return nil (empty tree).
		return nil
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil
	}
	return tree
}

// indexTree builds a tree object from the current index.
func indexTree(repo *goGit.Repository) *object.Tree {
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil
	}
	hash, err := buildTreeFromIndex(repo.Storer, idx, "")
	if err != nil {
		return nil
	}
	tree, err := repo.TreeObject(hash)
	if err != nil {
		return nil
	}
	return tree
}

// resolveTree resolves a revision string to its tree.
func resolveTree(repo *goGit.Repository, rev string) *object.Tree {
	h, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: bad revision '%s'\n", rev)
		os.Exit(128)
	}
	commit, err := repo.CommitObject(*h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		os.Exit(128)
	}
	tree, err := commit.Tree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		os.Exit(128)
	}
	return tree
}

// diffWorktree shows unstaged changes using worktree status.
func diffWorktree(repo *goGit.Repository, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet bool, paths []string) int {
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	status, err := wt.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	hasDiff := false
	for p, s := range status {
		if s.Worktree == goGit.Unmodified || s.Worktree == goGit.Untracked {
			continue
		}
		if len(paths) > 0 && !matchPaths(p, paths) {
			continue
		}
		hasDiff = true
		if nameOnly {
			fmt.Println(p)
		} else if nameStatus {
			fmt.Printf("%c\t%s\n", worktreeAction(s.Worktree), p)
		}
	}

	if exitCode && hasDiff {
		return 1
	}
	return 0
}

// diffCommitVsWorktree shows diff between a commit tree and the current worktree.
func diffCommitVsWorktree(repo *goGit.Repository, fromTree *object.Tree, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet bool, paths []string) int {
	// Get the current index tree to compare against fromTree.
	toTree := indexTree(repo)
	if fromTree == nil && toTree == nil {
		return 0
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if len(paths) > 0 {
		changes = filterChanges(changes, paths)
	}

	if len(changes) == 0 {
		return 0
	}

	return outputDiff(changes, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet)
}

func outputDiff(changes object.Changes, stat, nameOnly, nameStatus, numStat, noPatch, exitCode, quiet bool) int {
	if quiet {
		if len(changes) > 0 {
			return 1
		}
		return 0
	}

	if nameOnly {
		for _, c := range changes {
			name := changeName(c)
			fmt.Println(name)
		}
		if exitCode {
			return 1
		}
		return 0
	}

	if nameStatus {
		for _, c := range changes {
			action, _ := c.Action()
			name := changeName(c)
			switch action {
			case 1: // Insert
				fmt.Printf("A\t%s\n", name)
			case 2: // Delete
				fmt.Printf("D\t%s\n", name)
			default: // Modify
				fmt.Printf("M\t%s\n", name)
			}
		}
		if exitCode {
			return 1
		}
		return 0
	}

	patch, err := changes.Patch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if stat || numStat {
		stats := patch.Stats()
		if numStat {
			for _, s := range stats {
				fmt.Printf("%d\t%d\t%s\n", s.Addition, s.Deletion, s.Name)
			}
		} else {
			fmt.Print(stats)
		}
		if exitCode && len(stats) > 0 {
			return 1
		}
		return 0
	}

	if noPatch {
		if exitCode && len(changes) > 0 {
			return 1
		}
		return 0
	}

	// Full unified diff output.
	enc := diff.NewUnifiedEncoder(os.Stdout, 3)
	if err := enc.Encode(patch); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if exitCode {
		return 1
	}
	return 0
}

func changeName(c *object.Change) string {
	if c.To.Name != "" {
		return c.To.Name
	}
	return c.From.Name
}

func worktreeAction(code goGit.StatusCode) byte {
	switch code {
	case goGit.Added:
		return 'A'
	case goGit.Deleted:
		return 'D'
	case goGit.Renamed:
		return 'R'
	case goGit.Copied:
		return 'C'
	default:
		return 'M'
	}
}

func filterChanges(changes object.Changes, paths []string) object.Changes {
	var filtered object.Changes
	for _, c := range changes {
		name := changeName(c)
		if matchPaths(name, paths) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func matchPaths(name string, paths []string) bool {
	for _, p := range paths {
		if name == p || strings.HasPrefix(name, p+"/") || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

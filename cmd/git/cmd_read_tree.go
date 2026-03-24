package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdReadTree(args []string) int {
	var (
		empty    bool
		merge    bool
		treeish  string
		treeish2 string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--empty":
			empty = true
		case a == "-u":
			// update working tree — accepted, partially handled
		case a == "-m":
			merge = true
		case a == "--reset":
			merge = true
		case a == "-q", a == "--quiet":
			// accepted
		case !strings.HasPrefix(a, "-"):
			if treeish == "" {
				treeish = a
			} else {
				treeish2 = a
			}
		}
		i++
	}

	_ = merge

	repo := openRepoOrDie()

	if empty {
		// Clear the index.
		idx := &index.Index{Version: 2}
		if err := repo.Storer.SetIndex(idx); err != nil {
			fatal("%s", err)
		}
		return 0
	}

	// For two-tree merge form: read-tree -u -m HEAD <tree>
	// Use the second tree if provided.
	target := treeish
	if treeish2 != "" {
		target = treeish2
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: git read-tree [--empty | <tree-ish>]")
		return 128
	}

	h, err := repo.ResolveRevision(plumbing.Revision(target))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: not a valid object name: %s\n", target)
		return 128
	}

	// Get the tree — resolve commit to its tree if needed.
	var tree *object.Tree
	commit, err := repo.CommitObject(*h)
	if err == nil {
		tree, err = commit.Tree()
		if err != nil {
			fatal("%s", err)
		}
	} else {
		tree, err = repo.TreeObject(*h)
		if err != nil {
			fatal("not a tree object: %s", target)
		}
	}

	// Build index from tree.
	idx := &index.Index{Version: 2}
	if err := addTreeToIndex(idx, tree, ""); err != nil {
		fatal("%s", err)
	}

	if err := repo.Storer.SetIndex(idx); err != nil {
		fatal("%s", err)
	}

	return 0
}

func addTreeToIndex(idx *index.Index, tree *object.Tree, prefix string) error {
	for _, entry := range tree.Entries {
		fullpath := entry.Name
		if prefix != "" {
			fullpath = path.Join(prefix, entry.Name)
		}

		if entry.Mode == filemode.Dir {
			// Recurse into subtree.
			subtree, err := tree.Tree(entry.Name)
			if err != nil {
				return fmt.Errorf("cannot read subtree %s: %w", fullpath, err)
			}
			if err := addTreeToIndex(idx, subtree, fullpath); err != nil {
				return err
			}
		} else {
			e := idx.Add(fullpath)
			e.Hash = entry.Hash
			e.Mode = entry.Mode
		}
	}
	return nil
}

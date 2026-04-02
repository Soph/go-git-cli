package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdFsck(args []string) int {
	var (
		full        bool
		connectivity bool
		dangling    bool
		quiet       bool
	)

	dangling = true // git defaults to showing dangling objects

	for _, a := range args {
		switch a {
		case "--full":
			full = true
		case "--connectivity-only":
			connectivity = true
		case "--no-dangling":
			dangling = false
		case "--dangling":
			dangling = true
		case "-q", "--quiet":
			quiet = true
		case "--no-reflogs", "--strict", "--lost-found", "--unreachable",
			"--root", "--tags", "--cache", "--no-full", "--verbose":
			// accepted, ignored
		}
	}

	_ = full

	repo := openRepoOrDie()
	exitCode := 0

	// Collect reachable objects by walking from all refs.
	reachable := make(map[plumbing.Hash]bool)

	refs, err := repo.References()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot list references: %s\n", err)
		return 1
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		hash := ref.Hash()
		if hash.IsZero() {
			// Symbolic ref — skip, it resolves through its target.
			return nil
		}
		walkReachable(repo, hash, reachable, connectivity, quiet, &exitCode)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Also walk from HEAD if it points somewhere.
	if head, err := repo.Head(); err == nil {
		walkReachable(repo, head.Hash(), reachable, connectivity, quiet, &exitCode)
	}

	// Dangling check: find objects not in the reachable set.
	if dangling && !connectivity {
		allObjects, err := repo.Storer.IterEncodedObjects(plumbing.AnyObject)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot iterate objects: %s\n", err)
			return 1
		}

		err = allObjects.ForEach(func(obj plumbing.EncodedObject) error {
			if !reachable[obj.Hash()] && !quiet {
				fmt.Printf("dangling %s %s\n", obj.Type(), obj.Hash())
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
	}

	return exitCode
}

// walkReachable walks the object graph starting from hash, adding every
// reachable object to the set. If connectivity is false, it also decodes
// each object to validate its format.
func walkReachable(repo *git.Repository, hash plumbing.Hash, reachable map[plumbing.Hash]bool, connectivityOnly, quiet bool, exitCode *int) {
	stack := []plumbing.Hash{hash}

	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if reachable[h] {
			continue
		}
		reachable[h] = true

		// Check the object exists.
		obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, h)
		if err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "missing %s %s\n", "object", h)
			}
			*exitCode = 1
			continue
		}

		switch obj.Type() {
		case plumbing.CommitObject:
			commit, err := object.DecodeCommit(repo.Storer, obj)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "error in commit %s: %s\n", h, err)
				}
				*exitCode = 1
				continue
			}
			stack = append(stack, commit.TreeHash)
			for _, p := range commit.ParentHashes {
				stack = append(stack, p)
			}

		case plumbing.TreeObject:
			tree, err := object.DecodeTree(repo.Storer, obj)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "error in tree %s: %s\n", h, err)
				}
				*exitCode = 1
				continue
			}
			for _, entry := range tree.Entries {
				stack = append(stack, entry.Hash)
			}

		case plumbing.TagObject:
			tag, err := object.DecodeTag(repo.Storer, obj)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "error in tag %s: %s\n", h, err)
				}
				*exitCode = 1
				continue
			}
			stack = append(stack, tag.Target)

		case plumbing.BlobObject:
			if !connectivityOnly {
				// Decode to validate. For blobs this is cheap —
				// go-git just wraps the reader.
				if _, err := object.DecodeBlob(obj); err != nil {
					if !quiet {
						fmt.Fprintf(os.Stderr, "error in blob %s: %s\n", h, err)
					}
					*exitCode = 1
				}
			}
		}
	}
}

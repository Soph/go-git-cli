package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdCommitTree(args []string) int {
	var (
		treeHash string
		parents  []plumbing.Hash
		message  string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-p":
			if i+1 < len(args) {
				i++
				parents = append(parents, plumbing.NewHash(args[i]))
			}
		case "-m":
			if i+1 < len(args) {
				i++
				message = args[i]
			}
		default:
			if !strings.HasPrefix(a, "-") && treeHash == "" {
				treeHash = a
			}
		}
		i++
	}

	if treeHash == "" {
		fmt.Fprintln(os.Stderr, "usage: git commit-tree <tree> [-p <parent>]... [-m <message>]")
		return 128
	}

	// If no message from -m, read from stdin.
	if message == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("%s", err)
		}
		message = string(data)
	}

	// Deduplicate parents.
	seen := map[plumbing.Hash]bool{}
	var uniqueParents []plumbing.Hash
	for _, p := range parents {
		if !seen[p] {
			seen[p] = true
			uniqueParents = append(uniqueParents, p)
		}
	}

	now := time.Now()
	authorName := os.Getenv("GIT_AUTHOR_NAME")
	if authorName == "" {
		authorName = os.Getenv("GIT_COMMITTER_NAME")
	}
	if authorName == "" {
		authorName = "Go-git CLI"
	}
	authorEmail := os.Getenv("GIT_AUTHOR_EMAIL")
	if authorEmail == "" {
		authorEmail = os.Getenv("GIT_COMMITTER_EMAIL")
	}
	if authorEmail == "" {
		authorEmail = "go-git@localhost"
	}
	committerName := os.Getenv("GIT_COMMITTER_NAME")
	if committerName == "" {
		committerName = authorName
	}
	committerEmail := os.Getenv("GIT_COMMITTER_EMAIL")
	if committerEmail == "" {
		committerEmail = authorEmail
	}

	commit := &object.Commit{
		TreeHash:     plumbing.NewHash(treeHash),
		ParentHashes: uniqueParents,
		Author: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  now,
		},
		Committer: object.Signature{
			Name:  committerName,
			Email: committerEmail,
			When:  now,
		},
		Message: message,
	}

	repo := openRepoOrDie()

	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		fatal("%s", err)
	}

	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		fatal("%s", err)
	}

	fmt.Println(hash)
	return 0
}

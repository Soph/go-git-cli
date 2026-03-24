package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdShow(args []string) int {
	var (
		prettyRaw bool
		revision  string
	)

	for _, a := range args {
		switch {
		case a == "--pretty=raw":
			prettyRaw = true
		case strings.HasPrefix(a, "--pretty="):
			// other pretty formats, ignored
		case strings.HasPrefix(a, "--format="):
			// ignored
		case a == "-q", a == "--quiet":
			// accepted
		case a == "--no-patch", a == "-s":
			// accepted
		case !strings.HasPrefix(a, "-"):
			if revision == "" {
				revision = a
			}
		}
	}

	if revision == "" {
		revision = "HEAD"
	}

	repo := openRepoOrDie()

	h, err := repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: bad object %s\n", revision)
		return 128
	}

	// Try as commit first.
	commit, err := repo.CommitObject(*h)
	if err == nil {
		if prettyRaw {
			showCommitRaw(commit)
		} else {
			showCommitDefault(commit)
		}
		return 0
	}

	// Try as tag.
	tag, err := repo.TagObject(*h)
	if err == nil {
		showTag(tag)
		return 0
	}

	// Fallback: just print the hash.
	fmt.Println(h)
	return 0
}

func showCommitRaw(c *object.Commit) {
	fmt.Printf("commit %s\n", c.Hash)
	fmt.Printf("tree %s\n", c.TreeHash)
	for _, p := range c.ParentHashes {
		fmt.Printf("parent %s\n", p)
	}
	fmt.Printf("author %s <%s> %d %s\n", c.Author.Name, c.Author.Email,
		c.Author.When.Unix(), c.Author.When.Format("-0700"))
	fmt.Printf("committer %s <%s> %d %s\n", c.Committer.Name, c.Committer.Email,
		c.Committer.When.Unix(), c.Committer.When.Format("-0700"))
	fmt.Println()
	for _, line := range strings.Split(c.Message, "\n") {
		fmt.Printf("    %s\n", line)
	}
}

func showCommitDefault(c *object.Commit) {
	fmt.Printf("commit %s\n", c.Hash)
	fmt.Printf("Author: %s <%s>\n", c.Author.Name, c.Author.Email)
	fmt.Printf("Date:   %s\n", formatTime(c.Author.When))
	fmt.Println()
	for _, line := range strings.Split(strings.TrimRight(c.Message, "\n"), "\n") {
		fmt.Printf("    %s\n", line)
	}
	fmt.Println()
}

func showTag(t *object.Tag) {
	fmt.Printf("tag %s\n", t.Name)
	fmt.Printf("Tagger: %s <%s>\n", t.Tagger.Name, t.Tagger.Email)
	fmt.Printf("Date:   %s\n", formatTime(t.Tagger.When))
	fmt.Println()
	fmt.Println(t.Message)
}

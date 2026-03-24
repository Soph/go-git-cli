package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdCatFile(args []string) int {
	var (
		showType    bool
		showSize    bool
		prettyPrint bool
		checkExist  bool
		objectArg   string
		modeCount   int
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			showType = true
			modeCount++
		case "-s":
			showSize = true
			modeCount++
		case "-p":
			prettyPrint = true
			modeCount++
		case "-e":
			checkExist = true
			modeCount++
		case "--textconv", "--filters", "--batch", "--batch-check",
			"--batch-all-objects", "--follow-symlinks":
			// Unsupported modes — treat as usage error if combined.
			modeCount++
		case "--path=":
			// ignored
		default:
			if strings.HasPrefix(args[i], "--path=") {
				// ignored
			} else if !strings.HasPrefix(args[i], "-") {
				objectArg = args[i]
			}
		}
	}

	// Conflicting modes → usage error (exit 129).
	if modeCount > 1 {
		fmt.Fprintln(os.Stderr, "error: switch 'p' is incompatible with -t/-s/-e")
		return 129
	}

	if objectArg == "" && !checkExist {
		fmt.Fprintln(os.Stderr, "usage: git cat-file <type> <object>")
		return 129
	}

	repo := openRepoOrDie()

	// Resolve the argument to a hash.
	hash, err := repo.ResolveRevision(plumbing.Revision(objectArg))
	if err != nil {
		// Try as a raw hash.
		h := plumbing.NewHash(objectArg)
		if h.IsZero() {
			fmt.Fprintf(os.Stderr, "fatal: Not a valid object name %s\n", objectArg)
			return 128
		}
		hash = &h
	}

	obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, *hash)
	if err != nil {
		if checkExist {
			return 1
		}
		fmt.Fprintf(os.Stderr, "fatal: Not a valid object name %s\n", objectArg)
		return 128
	}

	if checkExist {
		return 0
	}

	if showType {
		fmt.Println(obj.Type().String())
		return 0
	}

	if showSize {
		fmt.Println(obj.Size())
		return 0
	}

	if prettyPrint {
		return catFilePretty(obj)
	}

	// Default: raw content
	reader, err := obj.Reader()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	defer reader.Close()
	io.Copy(os.Stdout, reader)
	return 0
}

func catFilePretty(obj plumbing.EncodedObject) int {
	switch obj.Type() {
	case plumbing.CommitObject:
		commit := &object.Commit{}
		if err := commit.Decode(obj); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		fmt.Printf("tree %s\n", commit.TreeHash)
		for _, p := range commit.ParentHashes {
			fmt.Printf("parent %s\n", p)
		}
		fmt.Printf("author %s\n", formatSignature(&commit.Author))
		fmt.Printf("committer %s\n", formatSignature(&commit.Committer))
		if commit.Signature != "" {
			fmt.Printf("gpgsig %s\n", strings.ReplaceAll(commit.Signature, "\n", "\n "))
		}
		fmt.Printf("\n%s", commit.Message)

	case plumbing.TreeObject:
		tree := &object.Tree{}
		if err := tree.Decode(obj); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		for _, e := range tree.Entries {
			fmt.Printf("%06o %s %s\t%s\n", uint32(e.Mode), modeToType(e.Mode), e.Hash, e.Name)
		}

	case plumbing.BlobObject:
		reader, err := obj.Reader()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		defer reader.Close()
		io.Copy(os.Stdout, reader)

	case plumbing.TagObject:
		tag := &object.Tag{}
		if err := tag.Decode(obj); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		fmt.Printf("object %s\n", tag.Target)
		fmt.Printf("type %s\n", tag.TargetType.String())
		fmt.Printf("tag %s\n", tag.Name)
		fmt.Printf("tagger %s\n", formatSignature(&tag.Tagger))
		fmt.Printf("\n%s", tag.Message)

	default:
		reader, err := obj.Reader()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		defer reader.Close()
		io.Copy(os.Stdout, reader)
	}

	return 0
}

func formatSignature(sig *object.Signature) string {
	return fmt.Sprintf("%s <%s> %d %s",
		sig.Name, sig.Email,
		sig.When.Unix(),
		sig.When.Format("-0700"),
	)
}


package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdForEachRef(args []string) int {
	var (
		format  string
		sortKey string
		patterns []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--format="):
			format = strings.TrimPrefix(a, "--format=")
		case a == "--format":
			if i+1 < len(args) {
				i++
				format = args[i]
			}
		case strings.HasPrefix(a, "--sort="):
			sortKey = strings.TrimPrefix(a, "--sort=")
		case a == "--sort":
			if i+1 < len(args) {
				i++
				sortKey = args[i]
			}
		case a == "--count":
			if i+1 < len(args) {
				i++ // skip value
			}
		case !strings.HasPrefix(a, "-"):
			patterns = append(patterns, a)
		}
		i++
	}

	_ = sortKey // TODO: sorting

	if format == "" {
		format = "%(objectname) %(objecttype)\t%(refname)"
	}

	repo := openRepoOrDie()
	refs, err := repo.References()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()

		// Filter by patterns.
		if len(patterns) > 0 {
			matched := false
			for _, p := range patterns {
				if strings.HasPrefix(name, p) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Skip HEAD.
		if ref.Name() == plumbing.HEAD {
			return nil
		}

		hash := ref.Hash()
		if ref.Type() == plumbing.SymbolicReference {
			resolved, err := repo.Reference(ref.Name(), true)
			if err != nil {
				return nil
			}
			hash = resolved.Hash()
		}

		// Determine object type.
		objType := "commit"
		obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, hash)
		if err == nil {
			objType = obj.Type().String()
		}

		line := format
		line = strings.ReplaceAll(line, "%(objectname)", hash.String())
		line = strings.ReplaceAll(line, "%(objecttype)", objType)
		line = strings.ReplaceAll(line, "%(refname)", name)
		line = strings.ReplaceAll(line, "%(refname:short)", ref.Name().Short())
		fmt.Println(line)

		return nil
	})

	return 0
}

package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage"
)

func cmdWriteTree(args []string) int {
	var (
		prefix     string
		missingOK  bool
	)

	for _, a := range args {
		switch {
		case a == "--missing-ok":
			missingOK = true
		case strings.HasPrefix(a, "--prefix="):
			prefix = strings.TrimPrefix(a, "--prefix=")
		}
	}

	repo := openRepoOrDie()
	idx, err := repo.Storer.Index()
	if err != nil {
		fatal("%s", err)
	}

	// Validate that all blob objects exist (unless --missing-ok).
	if !missingOK {
		for _, e := range idx.Entries {
			if prefix != "" && !strings.HasPrefix(e.Name, prefix) {
				continue
			}
			if repo.Storer.HasEncodedObject(e.Hash) != nil {
				fmt.Fprintf(os.Stderr, "error: invalid object %s '%s' for '%s'\n", e.Mode, e.Hash, e.Name)
				fmt.Fprintln(os.Stderr, "fatal: git-write-tree: error building trees")
				return 128
			}
		}
	}

	hash, err := buildTreeFromIndex(repo.Storer, idx, prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	fmt.Println(hash)
	return 0
}

// buildTreeFromIndex converts index entries into tree objects in the object store
// and returns the root tree hash. If prefix is non-empty, only entries under that
// prefix are included and the prefix directory becomes the root.
func buildTreeFromIndex(s storage.Storer, idx *index.Index, prefix string) (plumbing.Hash, error) {
	trees := map[string]*object.Tree{"": {}}
	entries := map[string]bool{}

	for _, e := range idx.Entries {
		name := e.Name
		if prefix != "" {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			name = strings.TrimPrefix(name, prefix)
		}

		parts := strings.Split(name, "/")
		var fullpath string
		for _, part := range parts {
			parent := fullpath
			fullpath = path.Join(fullpath, part)

			if _, ok := trees[fullpath]; ok {
				continue
			}
			if entries[fullpath] {
				continue
			}

			te := object.TreeEntry{Name: path.Base(fullpath)}
			if fullpath == name {
				te.Mode = e.Mode
				te.Hash = e.Hash
				entries[fullpath] = true
			} else {
				te.Mode = filemode.Dir
				trees[fullpath] = &object.Tree{}
			}
			trees[parent].Entries = append(trees[parent].Entries, te)
		}
	}

	return writeTreeRecursive(s, "", trees)
}

func writeTreeRecursive(s storage.Storer, parent string, trees map[string]*object.Tree) (plumbing.Hash, error) {
	t := trees[parent]
	sort.Slice(t.Entries, func(i, j int) bool {
		ni, nj := t.Entries[i].Name, t.Entries[j].Name
		if t.Entries[i].Mode == filemode.Dir {
			ni += "/"
		}
		if t.Entries[j].Mode == filemode.Dir {
			nj += "/"
		}
		return ni < nj
	})

	for i, e := range t.Entries {
		if e.Mode != filemode.Dir {
			continue
		}
		p := path.Join(parent, e.Name)
		var err error
		e.Hash, err = writeTreeRecursive(s, p, trees)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		t.Entries[i] = e
	}

	o := s.NewEncodedObject()
	if err := t.Encode(o); err != nil {
		return plumbing.ZeroHash, err
	}

	hash := o.Hash()
	if s.HasEncodedObject(hash) == nil {
		return hash, nil
	}
	return s.SetEncodedObject(o)
}

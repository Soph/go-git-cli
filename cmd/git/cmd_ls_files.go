package main

import (
	"fmt"
	"os"
	"strings"
)

func cmdLsFiles(args []string) int {
	var (
		stage   bool
		debug   bool
		paths   []string
	)

	for _, a := range args {
		switch a {
		case "-s", "--stage":
			stage = true
		case "--debug":
			debug = true
		case "-c", "--cached":
			// default behavior
		case "-z":
			// accepted, ignored (NUL termination)
		case "-t":
			// accepted, ignored (show status tags)
		default:
			if !strings.HasPrefix(a, "-") {
				paths = append(paths, a)
			}
		}
	}

	repo := openRepoOrDie()
	idx, err := repo.Storer.Index()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	for _, e := range idx.Entries {
		// Filter by paths if specified.
		if len(paths) > 0 {
			matched := false
			for _, p := range paths {
				if e.Name == p || strings.HasPrefix(e.Name, p+"/") || strings.HasPrefix(e.Name, p) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		if stage || debug {
			fmt.Printf("%06o %s %d\t%s\n", e.Mode, e.Hash, e.Stage, e.Name)
		} else {
			fmt.Println(e.Name)
		}
	}

	return 0
}

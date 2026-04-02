package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
)

func cmdRepack(args []string) int {
	var useRefDeltas bool

	for _, a := range args {
		switch a {
		case "-a", "-d", "-f", "-l", "--no-reuse-delta", "--no-reuse-object":
			// accepted, ignored — go-git repacks everything in one pass
		case "-q", "--quiet":
			// accepted, ignored
		case "--ref-deltas":
			useRefDeltas = true
		}
	}

	repo := openRepoOrDie()

	cfg := &git.RepackConfig{
		UseRefDeltas: useRefDeltas,
	}

	if err := repo.RepackObjects(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

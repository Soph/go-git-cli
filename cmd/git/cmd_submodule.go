package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
)

func cmdSubmodule(args []string) int {
	if len(args) == 0 {
		// bare "git submodule" = "git submodule status"
		return submoduleStatus(args)
	}

	switch args[0] {
	case "init":
		return submoduleInit(args[1:])
	case "update":
		return submoduleUpdate(args[1:])
	case "status":
		return submoduleStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "fatal: unknown submodule subcommand: %s\n", args[0])
		return 128
	}
}

func submoduleInit(args []string) int {
	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	// If specific paths given, init only those.
	if len(args) > 0 {
		for _, path := range args {
			sub, err := wt.Submodule(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: no submodule mapping found for path '%s'\n", path)
				return 128
			}
			if err := sub.Init(); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
		}
		return 0
	}

	subs, err := wt.Submodules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	if err := subs.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	return 0
}

func submoduleUpdate(args []string) int {
	var (
		init      bool
		noFetch   bool
		recursive bool
		depth     int
	)

	paths := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--init":
			init = true
		case "--no-fetch":
			noFetch = true
		case "--recursive":
			recursive = true
		case "--depth":
			if v, ok := nextVal(args, &i); ok {
				if _, err := fmt.Sscanf(v, "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", v)
					return 128
				}
			}
		case "-q", "--quiet":
			// accepted, ignored
		case "--remote", "--merge", "--rebase":
			// accepted, ignored — go-git only does checkout-style update
		default:
			if a == "--" {
				i++
				for i < len(args) {
					paths = append(paths, args[i])
					i++
				}
				continue
			}
			if a != "" && a[0] != '-' {
				paths = append(paths, a)
			}
		}
		i++
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	opts := &git.SubmoduleUpdateOptions{
		Init:    init,
		NoFetch: noFetch,
		Depth:   depth,
	}
	if recursive {
		opts.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}

	// If specific paths given, update only those.
	if len(paths) > 0 {
		for _, path := range paths {
			sub, err := wt.Submodule(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: no submodule mapping found for path '%s'\n", path)
				return 128
			}
			if err := sub.Update(opts); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
		}
		return 0
	}

	subs, err := wt.Submodules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	if err := subs.Update(opts); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	return 0
}

func submoduleStatus(args []string) int {
	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	subs, err := wt.Submodules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	statuses, err := subs.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	for _, st := range statuses {
		fmt.Println(st)
	}
	return 0
}

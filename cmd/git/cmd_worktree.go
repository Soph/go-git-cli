package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v6/osfs"

	"github.com/go-git/go-git/v6/plumbing"
	xstorage "github.com/go-git/go-git/v6/x/storage"

	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
)

func cmdWorktree(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: git worktree <add|list|remove|prune>")
		return 1
	}

	subcmd := args[0]
	rest := args[1:]

	switch subcmd {
	case "add":
		return worktreeAdd(rest)
	case "remove":
		return worktreeRemove(rest)
	case "list":
		return worktreeList(rest)
	case "prune":
		return worktreePrune(rest)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown worktree subcommand: %s\n", subcmd)
		return 1
	}
}

func worktreeAdd(args []string) int {
	var (
		branch   string
		detach   bool
		force    bool
		positional []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-b" || a == "-B":
			if i+1 < len(args) {
				i++
				branch = args[i]
			}
		case strings.HasPrefix(a, "-b"):
			branch = a[2:]
		case a == "--detach":
			detach = true
		case a == "-f", a == "--force":
			force = true
		case a == "-q", a == "--quiet":
			// accepted, ignored
		case a == "--":
			i++
			for i < len(args) {
				positional = append(positional, args[i])
				i++
			}
		case !strings.HasPrefix(a, "-"):
			positional = append(positional, a)
		default:
			// unknown flag, ignore
		}
		i++
	}

	_ = force // go-git doesn't have a force option for Add

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: git worktree add [-b <branch>] [--detach] <path> [<commit-ish>]")
		return 128
	}

	wtPath := positional[0]

	// Make path absolute.
	absPath, err := filepath.Abs(wtPath)
	if err != nil {
		fatal("cannot resolve path '%s': %s", wtPath, err)
	}
	wtPath = absPath

	// Determine the worktree name (used for .git/worktrees/<name>/).
	name := filepath.Base(wtPath)
	if branch != "" {
		name = branch
	}

	repo := openRepoOrDie()

	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		fatal("%s", err)
	}

	var opts []xworktree.Option

	// Resolve commit if specified.
	if len(positional) >= 2 {
		commitish := positional[1]
		h, err := repo.ResolveRevision(plumbing.Revision(commitish))
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: not a valid object name: '%s'\n", commitish)
			return 128
		}
		opts = append(opts, xworktree.WithCommit(*h))
	}

	if detach {
		opts = append(opts, xworktree.WithDetachedHead())
	}

	// If -b was specified, we need to handle the branch name. The library's Add
	// creates a branch with the worktree name. If -b differs from the path basename,
	// use the branch name as the worktree name so the library creates the right branch.
	if branch != "" {
		name = branch
	}

	// Create the worktree directory.
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		fatal("could not create directory '%s': %s", wtPath, err)
	}

	wtFS := osfs.New(wtPath, osfs.WithBoundOS())

	if err := wt.Add(wtFS, name, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

func worktreeRemove(args []string) int {
	var (
		force      bool
		positional []string
	)

	for _, a := range args {
		switch a {
		case "-f", "--force":
			force = true
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
	}

	_ = force

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: git worktree remove <worktree>")
		return 128
	}

	// The argument can be a path or a worktree name. Use the basename as the name.
	name := filepath.Base(positional[0])

	repo := openRepoOrDie()

	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		fatal("%s", err)
	}

	if err := wt.Remove(name); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	// Also remove the worktree directory itself if it exists.
	absPath, _ := filepath.Abs(positional[0])
	if absPath != "" {
		os.RemoveAll(absPath)
	}

	return 0
}

func worktreeList(_ []string) int {
	repo := openRepoOrDie()

	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		fatal("%s", err)
	}

	names, err := wt.List()
	if err != nil {
		fatal("%s", err)
	}

	// Also print the main worktree.
	wd, _ := os.Getwd()
	fmt.Printf("%s  (bare)\n", wd)

	for _, name := range names {
		fmt.Println(name)
	}

	return 0
}

func worktreePrune(_ []string) int {
	repo := openRepoOrDie()

	// Check if the storer supports worktrees.
	wts, ok := repo.Storer.(xstorage.WorktreeStorer)
	if !ok {
		return 0
	}

	dotgitFS := wts.Filesystem()

	entries, err := dotgitFS.ReadDir("worktrees")
	if err != nil {
		// No worktrees directory — nothing to prune.
		return 0
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Read the gitdir file to find the worktree path.
		gitdirPath := filepath.Join("worktrees", name, "gitdir")
		f, err := dotgitFS.Open(gitdirPath)
		if err != nil {
			continue
		}
		buf := make([]byte, 4096)
		n, _ := f.Read(buf)
		f.Close()

		wtGitDir := strings.TrimSpace(string(buf[:n]))
		if wtGitDir == "" {
			continue
		}

		// The gitdir points to the .git file inside the worktree.
		// Check if that path still exists.
		if _, err := os.Stat(wtGitDir); os.IsNotExist(err) {
			// Worktree no longer exists — remove the metadata.
			wt, err := xworktree.New(repo.Storer)
			if err == nil {
				wt.Remove(name)
			}
		}
	}

	return 0
}

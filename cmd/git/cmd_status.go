package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdStatus(args []string) int {
	var (
		porcelain     bool
		short         bool
		branch        bool
		nulTerminate  bool
		showUntracked = "normal" // "all", "normal", "no"
	)

	for _, a := range args {
		switch a {
		case "--porcelain", "--porcelain=v1":
			porcelain = true
		case "-s", "--short":
			short = true
		case "--no-short":
			short = false
		case "-b", "--branch":
			branch = true
		case "--no-branch":
			branch = false
		case "-z":
			nulTerminate = true
		case "-u", "--untracked-files", "--untracked-files=all", "-uall":
			showUntracked = "all"
		case "-uno", "-ufalse", "-uoff", "--untracked-files=no", "--untracked-files=false":
			showUntracked = "no"
		case "-unormal", "-utrue", "-uyes", "-uon", "--untracked-files=normal", "--untracked-files=true":
			showUntracked = "normal"
		case "--column", "--no-column":
			// accepted, ignored (we don't columnize)
		case "-v":
			// verbose — would need diff, ignored for now
		default:
			if strings.HasPrefix(a, "--column=") {
				// accepted, ignored
			}
		}
	}

	// Check status.* config options (on-disk + -c overrides).
	if rcfg, err := readRawConfig(); err == nil {
		for _, s := range rcfg.Sections {
			if !s.IsName("status") {
				continue
			}
			if !porcelain {
				if s.HasOption("short") {
					if cfgBool(s.Option("short")) {
						short = true
					}
				}
				if s.HasOption("branch") {
					if cfgBool(s.Option("branch")) {
						branch = true
					}
				}
			}
			if s.HasOption("showUntrackedFiles") {
				showUntracked = parseUntrackedMode(s.Option("showUntrackedFiles"))
			}
		}
	}
	// -c overrides take precedence over on-disk config.
	if globalConfigOverrides != nil {
		if v, ok := globalConfigOverrides["status.short"]; ok && !porcelain {
			if cfgBool(v) {
				short = true
			} else {
				short = false
			}
		}
		if v, ok := globalConfigOverrides["status.branch"]; ok && !porcelain {
			if cfgBool(v) {
				branch = true
			}
		}
		if v, ok := globalConfigOverrides["status.showuntrackedfiles"]; ok {
			showUntracked = parseUntrackedMode(v)
		}
	}

	// CLI flags override config — re-scan for explicit untracked flags.
	for _, a := range args {
		switch a {
		case "-u", "--untracked-files", "--untracked-files=all", "-uall":
			showUntracked = "all"
		case "-uno", "-ufalse", "-uoff", "--untracked-files=no", "--untracked-files=false":
			showUntracked = "no"
		case "-unormal", "-utrue", "-uyes", "-uon", "--untracked-files=normal", "--untracked-files=true":
			showUntracked = "normal"
		}
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	status, err := wt.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	// Check status.displayCommentPrefix config.
	commentPrefix := ""
	if rcfg2, err := readRawConfig(); err == nil {
		for _, s := range rcfg2.Sections {
			if s.IsName("status") && s.HasOption("displayCommentPrefix") {
				if cfgBool(s.Option("displayCommentPrefix")) {
					commentPrefix = "# "
				}
			}
		}
	}
	if globalConfigOverrides != nil {
		if v, ok := globalConfigOverrides["status.displaycommentprefix"]; ok {
			if cfgBool(v) {
				commentPrefix = "# "
			} else {
				commentPrefix = ""
			}
		}
	}

	if porcelain || short {
		if branch {
			printShortBranch(repo, nulTerminate)
		}
		printShortStatus(status, showUntracked, nulTerminate)
		return 0
	}

	// Long format.
	return printLongStatus(repo, status, showUntracked, commentPrefix)
}

// cfgBool returns true if val is a git boolean "true" value.
func cfgBool(val string) bool {
	switch strings.ToLower(val) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// parseUntrackedMode parses a status.showUntrackedFiles config value.
func parseUntrackedMode(v string) string {
	switch strings.ToLower(v) {
	case "no", "false", "off", "0":
		return "no"
	case "normal", "true", "on", "1":
		return "normal"
	case "all":
		return "all"
	}
	return "normal"
}

// branchTrackingInfo reads config to find upstream for branchName and returns
// (trackingRefName, displayName) or empty strings if not configured.
// For remote=".", trackingRef is refs/heads/<branch> (local).
// For remote="origin", trackingRef is refs/remotes/origin/<branch>.
func branchTrackingInfo(branchName string) (trackingRef, display string) {
	cfg, err := readRawConfig()
	if err != nil {
		return
	}
	remote, merge := "", ""
	for _, s := range cfg.Sections {
		if !s.IsName("branch") {
			continue
		}
		for _, ss := range s.Subsections {
			if ss.IsName(branchName) {
				remote = ss.Option("remote")
				merge = ss.Option("merge")
			}
		}
	}
	if remote == "" || merge == "" {
		return "", ""
	}
	upstream := strings.TrimPrefix(merge, "refs/heads/")
	if remote == "." {
		// Local branch tracking.
		display = upstream
		trackingRef = merge // refs/heads/<branch>
	} else {
		display = remote + "/" + upstream
		trackingRef = "refs/remotes/" + remote + "/" + upstream
	}
	return
}

// countAheadBehind returns (ahead, behind) commit counts between HEAD and
// the upstream tracking ref.
func countAheadBehind(repo *goGit.Repository, headHash plumbing.Hash, trackingRefName string) (int, int) {
	trackingRef, err := repo.Storer.Reference(plumbing.ReferenceName(trackingRefName))
	if err != nil {
		return 0, 0
	}
	upstreamHash := trackingRef.Hash()

	if headHash == upstreamHash {
		return 0, 0
	}

	// Collect commits reachable from HEAD.
	headSet := make(map[plumbing.Hash]bool)
	iter, err := repo.Log(&goGit.LogOptions{From: headHash})
	if err == nil {
		iter.ForEach(func(c *object.Commit) error {
			headSet[c.Hash] = true
			return nil
		})
	}

	// Collect commits reachable from upstream.
	upstreamSet := make(map[plumbing.Hash]bool)
	iter, err = repo.Log(&goGit.LogOptions{From: upstreamHash})
	if err == nil {
		iter.ForEach(func(c *object.Commit) error {
			upstreamSet[c.Hash] = true
			return nil
		})
	}

	ahead := 0
	for h := range headSet {
		if !upstreamSet[h] {
			ahead++
		}
	}
	behind := 0
	for h := range upstreamSet {
		if !headSet[h] {
			behind++
		}
	}
	return ahead, behind
}

// printShortBranch prints the "## branch...upstream" header for -s -b.
func printShortBranch(repo *goGit.Repository, nulTerminate bool) {
	eol := "\n"
	if nulTerminate {
		eol = "\x00"
	}
	head, err := repo.Head()
	if err != nil || !head.Name().IsBranch() {
		fmt.Printf("## HEAD (no branch)%s", eol)
		return
	}
	branchName := head.Name().Short()

	trackingRef, display := branchTrackingInfo(branchName)
	if trackingRef == "" {
		fmt.Printf("## %s%s", branchName, eol)
		return
	}

	ahead, behind := countAheadBehind(repo, head.Hash(), trackingRef)

	switch {
	case ahead > 0 && behind > 0:
		fmt.Printf("## %s...%s [ahead %d, behind %d]%s", branchName, display, ahead, behind, eol)
	case ahead > 0:
		fmt.Printf("## %s...%s [ahead %d]%s", branchName, display, ahead, eol)
	case behind > 0:
		fmt.Printf("## %s...%s [behind %d]%s", branchName, display, behind, eol)
	default:
		fmt.Printf("## %s...%s%s", branchName, display, eol)
	}
}

// printShortStatus prints porcelain/short format, with tracked changes sorted
// before untracked entries (matching git's output order).
func printShortStatus(status goGit.Status, showUntracked string, nulTerminate bool) {
	eol := "\n"
	if nulTerminate {
		eol = "\x00"
	}
	var tracked []string
	untrackedSet := make(map[string]bool)
	for p, s := range status {
		if s.Staging == goGit.Untracked && s.Worktree == goGit.Untracked {
			untrackedSet[p] = true
		} else if s.Staging != goGit.Unmodified || s.Worktree != goGit.Unmodified {
			tracked = append(tracked, p)
		}
	}
	sort.Strings(tracked)

	for _, p := range tracked {
		s := status[p]
		fmt.Printf("%c%c %s%s", byte(s.Staging), byte(s.Worktree), quotePath(p), eol)
	}
	if showUntracked != "no" {
		entries := collapseUntracked(untrackedSet, status, showUntracked)
		for _, p := range entries {
			fmt.Printf("?? %s%s", quotePath(p), eol)
		}
	}
}

// quotePath returns the path quoted with double quotes if it contains spaces
// or special characters, matching git's core.quotePath behavior.
func quotePath(p string) string {
	if strings.ContainsAny(p, " \t\"\\") {
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(p) + "\""
	}
	return p
}

// printLongStatus prints git's default long status format with sections.
func printLongStatus(repo *goGit.Repository, status goGit.Status, showUntracked string, cp string) int {
	head, headErr := repo.Head()
	initialCommit := headErr != nil // HEAD doesn't resolve → no commits yet

	if !initialCommit && head.Name().IsBranch() {
		fmt.Printf("%sOn branch %s\n", cp, head.Name().Short())
	} else if !initialCommit {
		fmt.Printf("%sHEAD detached at %s\n", cp, head.Hash().String()[:7])
	} else {
		// Try to get branch name from symbolic HEAD on unborn branch.
		symRef, err := repo.Storer.Reference(plumbing.HEAD)
		if err == nil && symRef.Type() == plumbing.SymbolicReference {
			fmt.Printf("%sOn branch %s\n", cp, symRef.Target().Short())
		}
	}

	// Check advice.statusHints config (needed for tracking info and section hints).
	showHints := true
	if rcfg, err := readRawConfig(); err == nil {
		for _, s := range rcfg.Sections {
			if !s.IsName("advice") {
				continue
			}
			if s.HasOption("statusHints") {
				if !cfgBool(s.Option("statusHints")) {
					showHints = false
				}
			}
		}
	}
	// -c overrides for advice.
	if globalConfigOverrides != nil {
		if v, ok := globalConfigOverrides["advice.statushints"]; ok {
			showHints = cfgBool(v)
		}
	}

	// Upstream tracking info (only if we have commits and a branch).
	if !initialCommit && head.Name().IsBranch() {
		branchName := head.Name().Short()
		trackingRef, display := branchTrackingInfo(branchName)
		if trackingRef != "" {
			ahead, behind := countAheadBehind(repo, head.Hash(), trackingRef)
			switch {
			case ahead > 0 && behind > 0:
				fmt.Printf("%sYour branch and '%s' have diverged,\n", cp, display)
				fmt.Printf("%sand have %d and %d different commits each, respectively.\n", cp, ahead, behind)
				if showHints {
					fmt.Printf("%s  (use \"git pull\" if you want to integrate the remote branch with yours)\n", cp)
				}
				fmt.Printf("%s\n", cp)
			case ahead > 0:
				fmt.Printf("%sYour branch is ahead of '%s' by %d commit", cp, display, ahead)
				if ahead > 1 {
					fmt.Print("s")
				}
				fmt.Println(".")
				if showHints {
					fmt.Printf("%s  (use \"git push\" to publish your local commits)\n", cp)
				}
				fmt.Printf("%s\n", cp)
			case behind > 0:
				fmt.Printf("%sYour branch is behind '%s' by %d commit", cp, display, behind)
				if behind > 1 {
					fmt.Print("s")
				}
				fmt.Println(", and can be fast-forwarded.")
				if showHints {
					fmt.Printf("%s  (use \"git pull\" to update your local branch)\n", cp)
				}
				fmt.Printf("%s\n", cp)
			}
		}
	}

	if initialCommit {
		fmt.Printf("%s\n", cp)
		fmt.Printf("%sNo commits yet\n", cp)
		fmt.Printf("%s\n", cp)
	}

	// Categorize files.
	var staged, unstaged []string
	untrackedSet := make(map[string]bool)
	for p, s := range status {
		if s.Staging != goGit.Unmodified && s.Staging != goGit.Untracked {
			staged = append(staged, p)
		}
		if s.Worktree == goGit.Modified || s.Worktree == goGit.Deleted {
			unstaged = append(unstaged, p)
		}
		if s.Staging == goGit.Untracked && s.Worktree == goGit.Untracked {
			untrackedSet[p] = true
		}
	}
	sort.Strings(staged)
	sort.Strings(unstaged)

	untracked := collapseUntracked(untrackedSet, status, showUntracked)

	hasStagedSection := len(staged) > 0
	hasUnstagedSection := len(unstaged) > 0
	hasUntrackedSection := showUntracked != "no" && len(untracked) > 0

	if hasStagedSection {
		fmt.Printf("%sChanges to be committed:\n", cp)
		if showHints {
			if initialCommit {
				fmt.Printf("%s  (use \"git rm --cached <file>...\" to unstage)\n", cp)
			} else {
				fmt.Printf("%s  (use \"git restore --staged <file>...\" to unstage)\n", cp)
			}
		}
		for _, p := range staged {
			s := status[p]
			fmt.Printf("%s\t%s%s\n", cp, longStagingPrefix(s.Staging), p)
		}
		fmt.Printf("%s\n", cp)
	}

	if hasUnstagedSection {
		fmt.Printf("%sChanges not staged for commit:\n", cp)
		if showHints {
			fmt.Printf("%s  (use \"git add <file>...\" to update what will be committed)\n", cp)
			fmt.Printf("%s  (use \"git restore <file>...\" to discard changes in working directory)\n", cp)
		}
		for _, p := range unstaged {
			s := status[p]
			fmt.Printf("%s\t%s%s\n", cp, longWorktreePrefix(s.Worktree), p)
		}
		fmt.Printf("%s\n", cp)
	}

	if hasUntrackedSection {
		fmt.Printf("%sUntracked files:\n", cp)
		if showHints {
			fmt.Printf("%s  (use \"git add <file>...\" to include in what will be committed)\n", cp)
		}
		for _, p := range untracked {
			fmt.Printf("%s\t%s\n", cp, p)
		}
		fmt.Printf("%s\n", cp)
	}

	// "Untracked files not listed" message when -uno hides them.
	if showUntracked == "no" && len(untrackedSet) > 0 {
		if showHints {
			fmt.Printf("%sUntracked files not listed (use -u option to show untracked files)\n", cp)
		} else {
			fmt.Printf("%sUntracked files not listed\n", cp)
		}
	}

	// Footer message.
	if !hasStagedSection && !hasUnstagedSection && !hasUntrackedSection {
		if showUntracked == "no" && len(untrackedSet) > 0 {
			fmt.Println("nothing added to commit but untracked files present (use -u to show)")
		} else {
			fmt.Println("nothing to commit, working tree clean")
		}
	} else if !hasStagedSection {
		if hasUnstagedSection {
			fmt.Println("no changes added to commit (use \"git add\" and/or \"git commit -a\")")
		} else if hasUntrackedSection {
			fmt.Println("nothing added to commit but untracked files present (use \"git add\" to track)")
		}
	}

	return 0
}

// collapseUntracked returns the untracked entries to display, collapsing
// directories in "normal" mode. In "all" mode, individual files are shown.
// In "normal" mode, if a directory contains only untracked files (no tracked
// files share the directory prefix), the directory name is shown instead.
func collapseUntracked(untrackedSet map[string]bool, status goGit.Status, mode string) []string {
	if mode == "no" {
		return nil
	}

	if mode == "all" || len(untrackedSet) == 0 {
		paths := make([]string, 0, len(untrackedSet))
		for p := range untrackedSet {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		return paths
	}

	// "normal" mode: collapse directories that contain only untracked files.
	// Build set of directories that contain tracked files.
	trackedDirs := make(map[string]bool)
	for p, s := range status {
		if s.Staging == goGit.Untracked && s.Worktree == goGit.Untracked {
			continue
		}
		dir := filepath.Dir(p)
		for dir != "." && dir != "" {
			trackedDirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	// For each untracked file, check if its top-level untracked directory
	// can be collapsed.
	collapsed := make(map[string]bool)
	var result []string
	for p := range untrackedSet {
		dir := topUntrackedDir(p, trackedDirs)
		if dir != "" {
			if !collapsed[dir] {
				collapsed[dir] = true
				result = append(result, dir+"/")
			}
		} else {
			result = append(result, p)
		}
	}
	sort.Strings(result)
	return result
}

// topUntrackedDir finds the top-most directory component of path that contains
// no tracked files. Returns "" if the file's immediate parent has tracked files.
func topUntrackedDir(path string, trackedDirs map[string]bool) string {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return "" // top-level file, cannot collapse
	}

	best := ""
	for d := dir; d != "." && d != ""; d = filepath.Dir(d) {
		if trackedDirs[d] {
			break
		}
		best = d
	}
	return best
}

// longStagingPrefix returns the prefix for a staged file in long format.
func longStagingPrefix(code goGit.StatusCode) string {
	switch code {
	case goGit.Added:
		return "new file:   "
	case goGit.Modified:
		return "modified:   "
	case goGit.Deleted:
		return "deleted:    "
	case goGit.Renamed:
		return "renamed:    "
	case goGit.Copied:
		return "copied:     "
	default:
		return ""
	}
}

// longWorktreePrefix returns the prefix for an unstaged change in long format.
func longWorktreePrefix(code goGit.StatusCode) string {
	switch code {
	case goGit.Modified:
		return "modified:   "
	case goGit.Deleted:
		return "deleted:    "
	default:
		return ""
	}
}

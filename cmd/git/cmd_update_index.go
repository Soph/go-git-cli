package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
)

func cmdUpdateIndex(args []string) int {
	var (
		add        bool
		remove     bool
		replace    bool
		refresh    bool
		forceRemove bool
		indexInfo  bool
		cacheInfos []string
		paths      []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--add":
			add = true
		case a == "--remove":
			remove = true
		case a == "--replace":
			replace = true
		case a == "--force-remove":
			forceRemove = true
		case a == "--refresh":
			refresh = true
		case a == "--index-info":
			indexInfo = true
		case a == "--cacheinfo":
			// Two forms:
			//   --cacheinfo <mode>,<hash>,<path>
			//   --cacheinfo <mode> <hash> <path>
			if i+1 < len(args) {
				i++
				if strings.Contains(args[i], ",") {
					cacheInfos = append(cacheInfos, args[i])
				} else if i+2 < len(args) {
					entry := args[i] + "," + args[i+1] + "," + args[i+2]
					i += 2
					cacheInfos = append(cacheInfos, entry)
				}
			}
		case a == "-q", a == "--quiet":
			// accepted, ignored
		case a == "--really-refresh":
			refresh = true
		case a == "--assume-unchanged", a == "--no-assume-unchanged",
			a == "--skip-worktree", a == "--no-skip-worktree",
			a == "--info-only", a == "--unresolve",
			a == "--chmod=+x", a == "--chmod=-x":
			// accepted, ignored for now
		case strings.HasPrefix(a, "--cacheinfo="):
			cacheInfos = append(cacheInfos, strings.TrimPrefix(a, "--cacheinfo="))
		case a == "--":
			i++
			for i < len(args) {
				paths = append(paths, args[i])
				i++
			}
		case !strings.HasPrefix(a, "-"):
			paths = append(paths, a)
		default:
			// Unknown flag, ignore gracefully.
		}
		i++
	}

	repo := openRepoOrDie()
	idx, err := repo.Storer.Index()
	if err != nil {
		fatal("%s", err)
	}

	modified := false

	// --index-info: read stdin lines in "mode SP hash TAB path" format.
	if indexInfo {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			mode, hash, path, ok := parseIndexInfoLine(line)
			if !ok {
				fmt.Fprintf(os.Stderr, "error: invalid --index-info line: %s\n", line)
				return 1
			}
			addCacheEntry(idx, mode, hash, path)
			modified = true
		}
		if err := scanner.Err(); err != nil {
			fatal("%s", err)
		}
	}

	// --cacheinfo entries.
	for _, ci := range cacheInfos {
		parts := strings.SplitN(ci, ",", 3)
		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "error: invalid --cacheinfo: %s\n", ci)
			return 1
		}
		modeStr, hashStr, path := parts[0], parts[1], parts[2]
		mode, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid mode: %s\n", modeStr)
			return 1
		}

		// Validate: path must not conflict with existing directory entries.
		// e.g., if "path5/a" is a tree, can't add "path5/a/file".
		if hasConflictingEntry(idx, path) {
			fmt.Fprintf(os.Stderr, "error: '%s' appears as both a file and as a directory\n", path)
			return 1
		}

		addCacheEntry(idx, filemode.FileMode(mode), plumbing.NewHash(hashStr), path)
		modified = true
	}

	// Process paths.
	for _, p := range paths {
		p = filepath.ToSlash(filepath.Clean(p))

		if forceRemove {
			if _, err := idx.Remove(p); err != nil {
				// force-remove ignores missing entries
			}
			modified = true
			continue
		}

		// Check if file exists on disk.
		info, statErr := os.Lstat(p)

		if statErr != nil {
			// File doesn't exist.
			if remove {
				if _, err := idx.Remove(p); err == nil {
					modified = true
				}
				continue
			}
			// Without --remove, fail if the file doesn't exist.
			fmt.Fprintf(os.Stderr, "error: %s: does not exist and --remove not passed\n", p)
			return 1
		}

		// File exists on disk.
		existing, entryErr := idx.Entry(p)
		if entryErr != nil {
			// Not in index yet.
			if !add {
				fmt.Fprintf(os.Stderr, "error: %s: cannot add to the index - missing --add option?\n", p)
				return 1
			}
			// Add new entry.
			h, err := hashAndStoreBlob(repo, p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s: %s\n", p, err)
				return 1
			}
			e := idx.Add(p)
			fillEntryFromFileInfo(e, h, info)
			modified = true
			continue
		}

		// Already in index — update it.
		if replace {
			// Remove first, then re-add.
			idx.Remove(p)
			h, err := hashAndStoreBlob(repo, p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s: %s\n", p, err)
				return 1
			}
			e := idx.Add(p)
			fillEntryFromFileInfo(e, h, info)
			modified = true
			continue
		}

		h, err := hashAndStoreBlob(repo, p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %s\n", p, err)
			return 1
		}
		existing.Hash = h
		fillEntryFromFileInfo(existing, h, info)
		modified = true
	}

	// --refresh: re-stat all entries, no object store changes.
	if refresh {
		for _, e := range idx.Entries {
			info, err := os.Lstat(e.Name)
			if err != nil {
				continue
			}
			e.ModifiedAt = info.ModTime()
			e.Size = uint32(info.Size())
		}
		modified = true
	}

	if modified {
		if err := repo.Storer.SetIndex(idx); err != nil {
			fatal("%s", err)
		}
	}

	return 0
}

// parseIndexInfoLine parses a line in the format: "mode SP sha1 TAB path"
// or "mode SP sha1 SP stage TAB path".
func parseIndexInfoLine(line string) (filemode.FileMode, plumbing.Hash, string, bool) {
	tabIdx := strings.IndexByte(line, '\t')
	if tabIdx < 0 {
		return 0, plumbing.ZeroHash, "", false
	}
	path := line[tabIdx+1:]
	left := line[:tabIdx]

	parts := strings.Fields(left)
	if len(parts) < 2 {
		return 0, plumbing.ZeroHash, "", false
	}

	mode, err := strconv.ParseUint(parts[0], 8, 32)
	if err != nil {
		return 0, plumbing.ZeroHash, "", false
	}

	hash := plumbing.NewHash(parts[1])

	return filemode.FileMode(mode), hash, path, true
}

// addCacheEntry adds or updates an entry in the index by hash (no working tree file needed).
func addCacheEntry(idx *index.Index, mode filemode.FileMode, hash plumbing.Hash, path string) {
	path = filepath.ToSlash(filepath.Clean(path))

	// Update existing entry if present.
	if e, err := idx.Entry(path); err == nil {
		e.Hash = hash
		e.Mode = mode
		e.ModifiedAt = time.Now()
		return
	}

	e := idx.Add(path)
	e.Hash = hash
	e.Mode = mode
	e.CreatedAt = time.Now()
	e.ModifiedAt = time.Now()
}

// hasConflictingEntry checks if adding path would conflict with an existing
// entry (e.g., path "a/b" when "a/b/c" already exists, or vice versa).
func hasConflictingEntry(idx *index.Index, path string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, e := range idx.Entries {
		if strings.HasPrefix(e.Name, path+"/") {
			return true
		}
		if strings.HasPrefix(path, e.Name+"/") {
			return true
		}
	}
	return false
}

// hashAndStoreBlob reads a file from the working tree, stores it as a blob,
// and returns its hash.
func hashAndStoreBlob(repo *git.Repository, path string) (plumbing.Hash, error) {
	f, err := os.Open(path)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(info.Size())

	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}

	return repo.Storer.SetEncodedObject(obj)
}

// fillEntryFromFileInfo populates an index entry's metadata from os.FileInfo.
func fillEntryFromFileInfo(e *index.Entry, h plumbing.Hash, info os.FileInfo) {
	e.Hash = h
	e.ModifiedAt = info.ModTime()
	e.Size = uint32(info.Size())

	m, err := filemode.NewFromOSFileMode(info.Mode())
	if err == nil {
		e.Mode = m
	}
}

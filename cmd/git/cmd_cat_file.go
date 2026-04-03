package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

func cmdCatFile(args []string) int {
	var (
		showType    bool
		showSize    bool
		prettyPrint bool
		checkExist  bool
		batchMode   string // "", "batch", "batch-check", "batch-command"
		batchFmt    string // custom format for --batch=<fmt>
		batchAll    bool
		allowUnknown bool
		followSymlinks bool
		bufferMode  bool
		nulInput    bool // -z
		nulOutput   bool // -Z
		pathSpec    string
		typeArg     string // positional type arg (e.g., "git cat-file blob <hash>")
		objectArg   string
	)

	singleModes := 0 // -t, -s, -p, -e, --textconv, --filters
	batchModes := 0  // --batch, --batch-check, --batch-command
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-t":
			showType = true
			singleModes++
		case a == "-s":
			showSize = true
			singleModes++
		case a == "-p":
			prettyPrint = true
			singleModes++
		case a == "-e":
			checkExist = true
			singleModes++
		case a == "--textconv":
			singleModes++
		case a == "--filters":
			singleModes++
		case a == "--batch":
			batchMode = "batch"
			batchModes++
		case strings.HasPrefix(a, "--batch="):
			batchMode = "batch"
			batchFmt = strings.TrimPrefix(a, "--batch=")
			batchModes++
		case a == "--batch-check":
			batchMode = "batch-check"
			batchModes++
		case strings.HasPrefix(a, "--batch-check="):
			batchMode = "batch-check"
			batchFmt = strings.TrimPrefix(a, "--batch-check=")
			batchModes++
		case a == "--batch-command":
			batchMode = "batch-command"
			batchModes++
		case strings.HasPrefix(a, "--batch-command="):
			batchMode = "batch-command"
			batchFmt = strings.TrimPrefix(a, "--batch-command=")
			batchModes++
		case a == "--batch-all-objects":
			batchAll = true
		case a == "--allow-unknown-type":
			allowUnknown = true
		case a == "--follow-symlinks":
			followSymlinks = true
		case a == "--buffer":
			bufferMode = true
		case a == "--no-buffer":
			bufferMode = false
		case a == "-z":
			nulInput = true
		case a == "-Z":
			nulOutput = true
		case strings.HasPrefix(a, "--path="):
			pathSpec = strings.TrimPrefix(a, "--path=")
		case a == "-h", a == "--help":
			fmt.Fprintln(os.Stderr, "usage: git cat-file <type> <object>")
			return 129
		default:
			if !strings.HasPrefix(a, "-") {
				if objectArg == "" {
					objectArg = a
				} else if typeArg == "" {
					// "git cat-file <type> <object>" — first positional is type, second is object
					typeArg = objectArg
					objectArg = a
				} else {
					fmt.Fprintln(os.Stderr, "fatal: too many arguments")
					return 129
				}
			}
		}
		i++
	}

	_ = allowUnknown

	modeCount := singleModes + batchModes

	// Two single-object modes (e.g., -e -p, -t -s, --textconv --filters).
	if singleModes > 1 {
		fmt.Fprintln(os.Stderr, "error: options cannot be used together")
		return 129
	}

	// Two batch modes.
	if batchModes > 1 {
		fmt.Fprintln(os.Stderr, "error: options cannot be used together")
		return 129
	}

	// Single-object mode + batch mode (e.g., -e --batch).
	if singleModes > 0 && batchModes > 0 {
		fmt.Fprintln(os.Stderr, "fatal: options are incompatible with each other")
		return 129
	}

	// --batch-all-objects with a single-object mode.
	if batchAll && singleModes > 0 && batchModes == 0 {
		fmt.Fprintln(os.Stderr, "error: options cannot be used together")
		return 129
	}

	// --path is incompatible with batch modes.
	if pathSpec != "" && batchMode != "" {
		fmt.Fprintln(os.Stderr, "fatal: --path is incompatible with batch mode")
		return 129
	}

	// --path with a single-object mode + extra args.
	if pathSpec != "" && singleModes > 0 {
		fmt.Fprintln(os.Stderr, "fatal: --path is incompatible with object mode")
		return 129
	}

	// Batch-only options used with single-object mode flags.
	if singleModes > 0 && batchModes == 0 {
		if followSymlinks {
			fmt.Fprintln(os.Stderr, "fatal: --follow-symlinks is incompatible with non-batch mode")
			return 129
		}
		if bufferMode {
			fmt.Fprintln(os.Stderr, "fatal: --buffer is incompatible with non-batch mode")
			return 129
		}
	}

	// Batch-only options without any mode at all.
	if batchModes == 0 && singleModes == 0 {
		if followSymlinks {
			fmt.Fprintln(os.Stderr, "fatal: --follow-symlinks needs --batch")
			return 129
		}
		if bufferMode {
			fmt.Fprintln(os.Stderr, "fatal: --buffer requires a batch mode")
			return 129
		}
		if batchAll {
			fmt.Fprintln(os.Stderr, "fatal: --batch-all-objects requires a batch mode")
			return 129
		}
		if nulInput || nulOutput {
			fmt.Fprintln(os.Stderr, "fatal: -z/-Z requires a batch mode")
			return 129
		}
	}

	// Single-object mode without object arg.
	if singleModes == 1 && batchModes == 0 && objectArg == "" {
		fmt.Fprintln(os.Stderr, "fatal: required argument missing")
		return 129
	}

	// Too many positional args with a mode flag.
	if modeCount == 1 && batchModes == 0 && typeArg != "" {
		fmt.Fprintln(os.Stderr, "fatal: too many arguments")
		return 129
	}

	// No mode and no type arg.
	if modeCount == 0 && typeArg == "" && objectArg == "" {
		fmt.Fprintln(os.Stderr, "usage: git cat-file <type> <object>")
		return 129
	}

	repo := openRepoOrDie()

	// Batch modes.
	if batchMode != "" {
		if batchAll {
			return catFileBatchAll(repo.Storer, batchMode, batchFmt, nulOutput)
		}
		delim := byte('\n')
		if nulInput {
			delim = 0
		}
		return catFileBatch(repo.Storer, batchMode, batchFmt, delim, nulOutput, bufferMode)
	}

	// Single-object modes.
	hash, err := repo.ResolveRevision(plumbing.Revision(objectArg))
	if err != nil {
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

	// "git cat-file <type> <object>" — just dump raw content.
	reader, err := obj.Reader()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	defer reader.Close()
	io.Copy(os.Stdout, reader)
	return 0
}

// catFileBatch reads object identifiers from stdin and outputs info/content.
func catFileBatch(s storer.EncodedObjectStorer, mode, format string, delim byte, nulOutput, buffered bool) int {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	scanner := bufio.NewScanner(os.Stdin)
	if delim == 0 {
		scanner.Split(splitNul)
	}

	flushAfterEach := !buffered

	for scanner.Scan() {
		line := scanner.Text()

		if mode == "batch-command" {
			if line == "flush" {
				if !buffered {
					fmt.Fprintln(os.Stderr, "fatal: flush is only for --buffer mode")
					return 128
				}
				w.Flush()
				continue
			}
			if line == "" {
				continue
			}
			cmd, rest, _ := strings.Cut(line, " ")
			switch cmd {
			case "contents":
				batchWriteObject(w, s, rest, format, true, nulOutput)
			case "info":
				batchWriteObject(w, s, rest, format, false, nulOutput)
			default:
				fmt.Fprintf(os.Stderr, "fatal: unknown command: '%s'\n", cmd)
				return 128
			}
		} else if mode == "batch" {
			oid, rest, _ := strings.Cut(line, " ")
			_ = rest
			batchWriteObject(w, s, oid, format, true, nulOutput)
		} else {
			// batch-check
			oid, rest, hasRest := strings.Cut(line, " ")
			if !hasRest {
				// Trim trailing whitespace/tabs.
				oid = strings.TrimRight(oid, " \t")
			}
			batchWriteInfo(w, s, oid, rest, format, nulOutput)
		}

		if flushAfterEach {
			w.Flush()
		}
	}
	return 0
}

// batchWriteObject writes the batch output for a single object (info + content).
func batchWriteObject(w *bufio.Writer, s storer.EncodedObjectStorer, oid, format string, includeContent, nulOutput bool) {
	h := plumbing.NewHash(oid)
	obj, err := s.EncodedObject(plumbing.AnyObject, h)
	if err != nil {
		if nulOutput {
			fmt.Fprintf(w, "%s missing%c", oid, 0)
		} else {
			fmt.Fprintf(w, "%s missing\n", oid)
		}
		return
	}

	if !includeContent {
		// Info only.
		if format != "" {
			fmt.Fprintln(w, expandBatchFormat(format, obj, ""))
		} else {
			fmt.Fprintf(w, "%s %s %d\n", obj.Hash(), obj.Type(), obj.Size())
		}
		return
	}

	// Content mode: header + content + trailing newline.
	if format != "" {
		fmt.Fprintln(w, expandBatchFormat(format, obj, ""))
	} else {
		fmt.Fprintf(w, "%s %s %d\n", obj.Hash(), obj.Type(), obj.Size())
	}

	reader, err := obj.Reader()
	if err != nil {
		return
	}
	io.Copy(w, reader)
	reader.Close()
	fmt.Fprintln(w)
}

// batchWriteInfo writes batch-check output for a single object.
func batchWriteInfo(w *bufio.Writer, s storer.EncodedObjectStorer, oid, rest, format string, nulOutput bool) {
	if oid == "" {
		if nulOutput {
			fmt.Fprintf(w, " missing%c", 0)
		} else {
			fmt.Fprintf(w, " missing\n")
		}
		return
	}

	h := plumbing.NewHash(oid)
	obj, err := s.EncodedObject(plumbing.AnyObject, h)
	if err != nil {
		if nulOutput {
			fmt.Fprintf(w, "%s missing%c", oid, 0)
		} else {
			fmt.Fprintf(w, "%s missing\n", oid)
		}
		return
	}

	if format != "" {
		fmt.Fprintln(w, expandBatchFormat(format, obj, rest))
	} else {
		fmt.Fprintf(w, "%s %s %d\n", obj.Hash(), obj.Type(), obj.Size())
	}
}

// expandBatchFormat expands %(field) placeholders in a batch format string.
func expandBatchFormat(format string, obj plumbing.EncodedObject, rest string) string {
	r := strings.NewReplacer(
		"%(objectname)", obj.Hash().String(),
		"%(objecttype)", obj.Type().String(),
		"%(objectsize)", fmt.Sprintf("%d", obj.Size()),
		"%(rest)", rest,
	)
	return r.Replace(format)
}

// catFileBatchAll iterates all objects in the store.
func catFileBatchAll(s storer.EncodedObjectStorer, mode, format string, nulOutput bool) int {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	iter, err := s.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	includeContent := mode == "batch" || mode == "batch-command"

	err = iter.ForEach(func(obj plumbing.EncodedObject) error {
		if includeContent {
			batchWriteObject(w, s, obj.Hash().String(), format, true, nulOutput)
		} else {
			if format != "" {
				fmt.Fprintln(w, expandBatchFormat(format, obj, ""))
			} else {
				fmt.Fprintf(w, "%s %s %d\n", obj.Hash(), obj.Type(), obj.Size())
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
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

// splitNul is a bufio.SplitFunc that splits on NUL bytes.
func splitNul(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == 0 {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

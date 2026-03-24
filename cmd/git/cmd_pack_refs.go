package main

// cmdPackRefs is a no-op — go-git handles packed refs internally.
func cmdPackRefs(args []string) int {
	// go-git uses packed-refs automatically, so this is a successful no-op.
	return 0
}

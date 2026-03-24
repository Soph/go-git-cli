package main

import (
	"os"
)

// cmdSwitch is an alias for checkout with slightly different flag parsing.
func cmdSwitch(args []string) int {
	// git switch is essentially git checkout for branch switching.
	// Convert switch-specific flags to checkout equivalents.
	var converted []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-c", "--create":
			converted = append(converted, "-b")
		case "-C", "--force-create":
			converted = append(converted, "-B")
		case "-d", "--detach":
			converted = append(converted, "--detach")
		case "-q", "--quiet":
			converted = append(converted, "-q")
		case "-f", "--force":
			converted = append(converted, "-f")
		case "--discard-changes":
			converted = append(converted, "-f")
		case "--":
			converted = append(converted, args[i:]...)
			break
		default:
			converted = append(converted, a)
		}
	}

	code := cmdCheckout(converted)
	if code != 0 {
		// If the error was printed to stderr by checkout, just return the code.
		os.Stderr.Sync()
	}
	return code
}

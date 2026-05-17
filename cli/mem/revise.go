package mem

import (
	"ai-hub/cli/client"
	"fmt"
	"os"
)

// RunReviseImpl executes the mem revise command.
func RunReviseImpl(c *client.Client, group string, args []string) int {
	_ = c
	_ = group
	_ = args
	fmt.Fprintln(os.Stderr, "Error: `ai-hub mem revise` has been removed and is no longer available.")
	return 1
}

// RunDeprecateImpl executes the mem deprecate command.
func RunDeprecateImpl(c *client.Client, group string, args []string) int {
	_ = c
	_ = group
	_ = args
	fmt.Fprintln(os.Stderr, "Error: `ai-hub mem deprecate` has been removed and is no longer available.")
	return 1
}

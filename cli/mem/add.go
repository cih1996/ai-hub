package mem

import (
	"ai-hub/cli/client"
	"fmt"
	"os"
)

// RunAdd executes the mem add command.
func RunAdd(c *client.Client, group string, args []string) int {
	_ = c
	_ = group
	_ = args
	fmt.Fprintln(os.Stderr, "Error: `ai-hub mem` has been removed. Use file-based memory management instead.")
	return 1
}

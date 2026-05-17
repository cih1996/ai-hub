package mem

import (
	"ai-hub/cli/client"
	"fmt"
	"os"
)

// RunFeedbackImpl executes the mem feedback command.
func RunFeedbackImpl(c *client.Client, group string, args []string) int {
	_ = c
	_ = group
	_ = args
	fmt.Fprintln(os.Stderr, "Error: `ai-hub mem feedback` has been removed and is no longer available.")
	return 1
}

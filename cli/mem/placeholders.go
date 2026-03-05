package mem

import (
	"ai-hub/cli/client"
)

// RunRetrieve executes the mem retrieve command
func RunRetrieve(c *client.Client, group string, args []string) int {
	return RunRetrieveImpl(c, group, args)
}

// RunFeedback executes the mem feedback command
func RunFeedback(c *client.Client, group string, args []string) int {
	return RunFeedbackImpl(c, group, args)
}

// RunRevise executes the mem revise command
func RunRevise(c *client.Client, group string, args []string) int {
	return RunReviseImpl(c, group, args)
}

// RunDeprecate executes the mem deprecate command
func RunDeprecate(c *client.Client, group string, args []string) int {
	return RunDeprecateImpl(c, group, args)
}

// RunSpec outputs JSON Schema for a subcommand
func RunSpec(args []string) int {
	return RunSpecImpl(args)
}

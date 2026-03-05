package commands

import "strings"

// SplitQueryAndFlags separates the first non-flag argument (query/filename)
// from flag arguments. Flags start with "-".
func SplitQueryAndFlags(args []string) (string, []string) {
	var positional string
	var flagArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else if positional == "" {
			positional = arg
		} else {
			positional += " " + arg
		}
	}
	return positional, flagArgs
}

// BuildScope constructs the full scope string from scope type and group name.
func BuildScope(scope, group string) string {
	if group != "" {
		return group + "/" + scope
	}
	return scope
}

// ValidateScope checks if scope is "knowledge" or "memory".
func ValidateScope(scope string) bool {
	return scope == "knowledge" || scope == "memory"
}

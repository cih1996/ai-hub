package commands

import (
	"fmt"
	"os"
	"strings"
)

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

// LevelToScope converts --level (session/team/global) to the appropriate scope string.
// Returns scope string and error message (empty if ok).
func LevelToScope(level string) (string, string) {
	group := os.Getenv("AI_HUB_GROUP_NAME")
	sessionID := os.Getenv("AI_HUB_SESSION_ID")

	switch level {
	case "session":
		if group == "" || sessionID == "" {
			return "", "Error: --level session requires AI_HUB_GROUP_NAME and AI_HUB_SESSION_ID environment variables"
		}
		return group + "/sessions/" + sessionID + "/memory", ""
	case "team":
		if group == "" {
			return "", "Error: --level team requires AI_HUB_GROUP_NAME environment variable"
		}
		return group + "/memory", ""
	case "global":
		return "memory", ""
	default:
		return "", "Error: --level must be 'session', 'team', or 'global'"
	}
}

// FormatTime formats an RFC3339 time string to a shorter display format.
func FormatTime(rfc3339 string) string {
	if rfc3339 == "" {
		return "-"
	}
	// Try to parse and reformat
	if len(rfc3339) >= 19 {
		return rfc3339[:10] + " " + rfc3339[11:19]
	}
	return rfc3339
}

// TruncatePreview truncates a preview string to maxLen characters.
func TruncatePreview(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

// PrintLevelUsage prints the standard --level help text.
func PrintLevelUsage() {
	fmt.Fprintf(os.Stderr, `  --level <level>     Required. session / team / global
`)
}

package cli

import (
	"ai-hub/cli/client"
	"ai-hub/cli/commands"
	"flag"
	"fmt"
	"os"
	"strconv"
)

var (
	Version = "dev"
)

// GlobalFlags holds global CLI flags
type GlobalFlags struct {
	SessionID int
	GroupName string
	Port      int
	Help      bool
	Version   bool
}

// ParseGlobalFlags parses global flags from args
func ParseGlobalFlags(args []string) (*GlobalFlags, []string, error) {
	flags := &GlobalFlags{}
	fs := flag.NewFlagSet("ai-hub", flag.ContinueOnError)

	fs.IntVar(&flags.SessionID, "session", 0, "Session ID (env: AI_HUB_SESSION_ID)")
	fs.StringVar(&flags.GroupName, "group", "", "Group name (env: AI_HUB_GROUP_NAME)")
	fs.IntVar(&flags.Port, "port", 8080, "Server port (env: AI_HUB_PORT)")
	fs.BoolVar(&flags.Help, "help", false, "Show help")
	fs.BoolVar(&flags.Version, "version", false, "Show version")

	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	// Environment variable fallback
	if flags.SessionID == 0 {
		if envSession := os.Getenv("AI_HUB_SESSION_ID"); envSession != "" {
			if id, err := strconv.Atoi(envSession); err == nil {
				flags.SessionID = id
			}
		}
	}
	if flags.GroupName == "" {
		flags.GroupName = os.Getenv("AI_HUB_GROUP_NAME")
	}
	if flags.Port == 8080 {
		if envPort := os.Getenv("AI_HUB_PORT"); envPort != "" {
			if port, err := strconv.Atoi(envPort); err == nil {
				flags.Port = port
			}
		}
	}

	return flags, fs.Args(), nil
}

// Run is the CLI entry point
func Run(args []string) int {
	globalFlags, remaining, err := ParseGlobalFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}

	// Handle --version
	if globalFlags.Version {
		fmt.Printf("ai-hub version %s\n", Version)
		return 0
	}

	// Handle --help or no command
	if globalFlags.Help || len(remaining) == 0 {
		printHelp()
		return 0
	}

	// Create API client
	c := client.NewClient(globalFlags.Port)

	// Route to subcommand
	command := remaining[0]
	commandArgs := remaining[1:]

	switch command {
	case "search":
		return commands.RunSearch(c, globalFlags.GroupName, commandArgs)
	case "write":
		return commands.RunWrite(c, globalFlags.GroupName, commandArgs)
	case "read":
		return commands.RunRead(c, globalFlags.GroupName, commandArgs)
	case "delete":
		return commands.RunDelete(c, globalFlags.GroupName, commandArgs)
	case "list":
		return commands.RunList(c, globalFlags.GroupName, commandArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printHelp()
		return 1
	}
}

func printHelp() {
	fmt.Println(`AI Hub CLI - Unified data layer interface

Usage:
  ai-hub [global flags] <command> [command flags]

Global Flags:
  --session <id>     Session ID (env: AI_HUB_SESSION_ID)
  --group <name>     Group name (env: AI_HUB_GROUP_NAME)
  --port <port>      Server port (env: AI_HUB_PORT, default: 8080)
  --help             Show this help
  --version          Show version

Commands:
  search             Search knowledge/memory by semantic similarity
  write              Write knowledge/memory file
  read               Read knowledge/memory file
  delete             Delete knowledge/memory file
  list               List knowledge/memory files

Use "ai-hub <command> --help" for more information about a command.`)
}

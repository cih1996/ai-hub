package cli

import (
	"ai-hub/cli/client"
	"ai-hub/cli/commands"
	"ai-hub/cli/mem"
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
	flags := &GlobalFlags{Port: 8080}

	// Manual parsing: extract global flags from anywhere in args, leave the rest as remaining.
	// This allows: ai-hub mem add --port 8081  (not just: ai-hub --port 8081 mem add)
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &flags.SessionID)
			}
		case "--group":
			if i+1 < len(args) {
				i++
				flags.GroupName = args[i]
			}
		case "--port":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &flags.Port)
			}
		case "--help":
			flags.Help = true
		case "--version":
			flags.Version = true
		default:
			remaining = append(remaining, args[i])
		}
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

	return flags, remaining, nil
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
		return commands.RunSearch(c, commandArgs)
	case "write":
		return commands.RunWrite(c, commandArgs)
	case "read":
		return commands.RunRead(c, commandArgs)
	case "delete":
		return commands.RunDelete(c, commandArgs)
	case "list":
		return commands.RunList(c, commandArgs)
	case "edit":
		return commands.RunEdit(c, commandArgs)
	case "sessions":
		return commands.RunSessions(c, commandArgs)
	case "send":
		return commands.RunSend(c, commandArgs)
	case "rules":
		return commands.RunRules(c, commandArgs)
	case "notes":
		return commands.RunNotes(c, commandArgs)
	case "triggers":
		return commands.RunTriggers(c, commandArgs)
	case "status":
		return commands.RunStatus(c, commandArgs)
	case "version":
		fmt.Printf("ai-hub version %s\n", Version)
		return 0
	case "mem":
		return runMem(c, globalFlags.GroupName, commandArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printHelp()
		return 1
	}
}

func runMem(c *client.Client, group string, args []string) int {
	if len(args) == 0 {
		printMemHelp()
		return 0
	}
	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "add":
		return mem.RunAdd(c, group, subArgs)
	case "retrieve":
		return mem.RunRetrieve(c, group, subArgs)
	case "feedback":
		return mem.RunFeedback(c, group, subArgs)
	case "revise":
		return mem.RunRevise(c, group, subArgs)
	case "deprecate":
		return mem.RunDeprecate(c, group, subArgs)
	case "spec":
		return mem.RunSpec(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown mem subcommand: %s\n", subCmd)
		printMemHelp()
		return 1
	}
}

func printMemHelp() {
	fmt.Println(`AI Hub Memory Runtime - Structured memory management

Usage:
  ai-hub mem <subcommand> [args]

Subcommands:
  add          Write a new structured memory record
  retrieve     Search memories with semantic + statistical reranking
  feedback     Report success/fail for a memory record
  revise       Create a new version of an existing memory
  deprecate    Mark a memory as deprecated
  spec         Output JSON Schema for a subcommand

Examples:
  echo '{"type":"procedure","title":"Deploy SOP",...}' | ai-hub mem add
  ai-hub mem retrieve --query "deploy" --types procedure
  ai-hub mem feedback --id mem_20260305_0001 --result success
  ai-hub mem spec add`)
}

func printHelp() {
	fmt.Println(`AI Hub CLI - Unified system interface

Usage:
  ai-hub [global flags] <command> [command flags]

Global Flags:
  --session <id>     Session ID (env: AI_HUB_SESSION_ID)
  --group <name>     Group name (env: AI_HUB_GROUP_NAME)
  --port <port>      Server port (env: AI_HUB_PORT, default: 8080)
  --help             Show this help
  --version          Show version

Memory:
  search             Search memory by semantic similarity
  list               List memory files
  read               Read memory file
  write              Write memory file
  edit               Edit memory file (find and replace)
  delete             Delete memory file
  mem                Structured memory runtime (add/retrieve/feedback/revise/deprecate/spec)

Sessions:
  sessions           List all sessions
  sessions <id>      Session detail
  sessions <id> messages   View recent messages
  send               Send message to a session (0=new)

System:
  rules              Manage session rules (get/set/delete)
  notes              Manage notes (list/read/write/delete)
  triggers           Manage triggers (list/create/update/delete)
  status             System status
  version            Show version

Use "ai-hub <command> --help" for more information about a command.`)
}

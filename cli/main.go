package cli

import (
	"ai-hub/cli/client"
	"ai-hub/cli/commands"
	"ai-hub/cli/mem"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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
	flags := &GlobalFlags{Port: 9527}

	// Manual parsing: extract global flags from anywhere in args, leave the rest as remaining.
	// This allows: ai-hub mem add --port 8081  (not just: ai-hub --port 8081 mem add)
	// Note: --help is only consumed before the command, after command it's passed through
	var remaining []string
	commandFound := false
	for i := 0; i < len(args); i++ {
		// Once we find a non-flag argument (the command), pass everything through
		if commandFound {
			remaining = append(remaining, args[i])
			continue
		}

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
			// First non-flag argument is the command
			if !strings.HasPrefix(args[i], "-") {
				commandFound = true
			}
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
	if flags.Port == 9527 {
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
	// Set Windows console to UTF-8 to fix Chinese character display
	if runtime.GOOS == "windows" {
		exec.Command("cmd", "/c", "chcp", "65001").Run()
	}

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
	case "groups":
		return commands.RunGroups(c, commandArgs)
	case "send":
		return commands.RunSend(c, commandArgs)
	case "rules":
		return commands.RunRules(c, commandArgs)
	case "notes":
		return commands.RunNotes(c, commandArgs)
	case "triggers":
		return commands.RunTriggers(c, commandArgs)
	case "errors":
		return commands.RunErrors(c, commandArgs)
	case "service", "services":
		return commands.RunService(c, commandArgs)
	case "status":
		return commands.RunStatus(c, commandArgs)
	case "version":
		fmt.Printf("ai-hub version %s\n", Version)
		return 0
	case "daemon":
		return commands.RunDaemon(c, commandArgs)
	case "reload":
		return commands.RunReload(c, commandArgs)
	case "skills":
		return commands.RunSkills(c, commandArgs)
	case "schemas":
		return commands.RunSchemas(c, commandArgs)
	case "mount":
		return commands.RunMount(c, commandArgs)
	case "transfer":
		return commands.RunTransfer(c, commandArgs)
	case "injection-router":
		return commands.RunInjectionRouter(c, commandArgs)
	case "hooks":
		return commands.RunHooks(c, commandArgs)
	case "changelog":
		return commands.RunChangelog(c, commandArgs)
	case "shadow-ai":
		return commands.RunShadowAI(c, commandArgs)
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
  --port <port>      Server port (env: AI_HUB_PORT, default: 9527)
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
  sessions <id> move --group <name>  Move session to group
  sessions <id> reset [--keep-last N] [--yes]  Reset session context
  sessions <id> reset --auto-threshold N  Set auto-reset threshold
  send               Send message to a session (0=new)

Groups:
  groups             List all groups
  groups <name>      Group detail
  groups create <name> [--desc <desc>]  Create group
  groups delete <name>  Delete group

Services:
  services           List all services
  services <id>      Service detail
  services create    Create a service
  services start/stop/restart <id>
  services logs <id> View service logs
  services delete <id>

Skills:
  skills             List all skills (default)
  skills read <name> Read skill full content
  skills create <name> --content "..."  Create a new skill
  skills update <name> --content "..."  Update skill content
  skills delete <name>                  Delete a skill

Schemas:
  schemas            List all schemas (default)
  schemas get <name> Show schema definition
  schemas create <name> --definition '<json>'  Create a schema
  schemas delete <name>                        Delete a schema

System:
  rules              Manage session rules (get/set/delete)
  notes              Manage notes (list/read/write/delete)
  triggers           Manage triggers (list/create/update/delete)
  status             System status
  version            Show version

Daemon:
  daemon start       Start AI Hub service
  daemon stop        Stop AI Hub service (graceful)
  daemon restart     Restart AI Hub service
  daemon install     Install as system service
  daemon uninstall   Uninstall system service
  daemon status      Show service status

Hot Reload:
  reload vector      Reload vector engine
  reload config      Reload configuration
  reload skills      Reload skill definitions

Static Mount:
  mount <path> --alias <name>   Mount local directory for static file serving
  mount list                    List all mounts
  mount remove <alias>          Remove a mount

File Transfer:
  transfer send     Upload file to remote (--file <path> --remote <url>)
  transfer pull     Download file from remote (--remote <url> --id <id> --save <path>)
  transfer list     List transfer records (--remote <url>)
  transfer status   Check transfer status (<id> --remote <url>)
  transfer delete   Delete transfer record (<id> --remote <url>)

Injection Router:
  injection-router list                                        List injection rules
  injection-router set --keywords "kw" --inject "categories"   Create injection rule
  injection-router delete <id>                                 Delete injection rule

Hooks:
  hooks list [--event <type>]         List event hooks
  hooks create --event <type> --target-session <id> --payload <msg> [--condition <cond>]
  hooks delete <id>                   Delete a hook
  hooks enable <id>                   Enable a hook
  hooks disable <id>                  Disable a hook

Changelog:
  changelog <file> [--scope <scope>] [--limit N]          View memory change history
  changelog <file> [--scope <scope>] --rollback <version> Rollback to specific version

Shadow AI:
  shadow-ai              Show shadow AI status
  shadow-ai enable       Enable shadow AI (creates session + triggers)
  shadow-ai disable      Disable shadow AI (stops triggers, keeps data)
  shadow-ai status       Show shadow AI status
  shadow-ai config       Show current config (or set: key=value ...)
  shadow-ai logs         Show work logs (--lines N)

Use "ai-hub <command> --help" for more information about a command.`)
}

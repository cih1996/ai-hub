package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// RunReload handles hot-reload commands
func RunReload(c *client.Client, args []string) int {
	if len(args) == 0 {
		printReloadHelp()
		return 0
	}

	target := args[0]

	switch target {
	case "config":
		return reloadConfig(c)
	case "skills":
		return reloadSkills(c)
	case "--help", "-h":
		printReloadHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown reload target: %s\n", target)
		printReloadHelp()
		return 1
	}
}

func printReloadHelp() {
	fmt.Println(`AI Hub Hot Reload

Usage:
  ai-hub reload <target> [options]

Targets:
  config     Reload configuration files
  skills     Reload skill definitions

Examples:
  ai-hub reload config                  # Reload configuration
  ai-hub reload skills                  # Reload skill definitions`)
}

func reloadConfig(c *client.Client) int {
	fmt.Println("Reloading configuration...")
	resp, err := c.POST("/reload/config", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reload config: %v\n", err)
		return 1
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err == nil {
		if msg, ok := result["message"].(string); ok {
			fmt.Println(msg)
			return 0
		}
	}
	fmt.Println("Configuration reloaded successfully")
	return 0
}

func reloadSkills(c *client.Client) int {
	fmt.Println("Reloading skills...")
	resp, err := c.POST("/reload/skills", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reload skills: %v\n", err)
		return 1
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err == nil {
		if msg, ok := result["message"].(string); ok {
			fmt.Println(msg)
			return 0
		}
	}
	fmt.Println("Skills reloaded successfully")
	return 0
}

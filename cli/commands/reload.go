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
	case "vector":
		return reloadVector(c)
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
  ai-hub reload <target>

Targets:
  vector     Reload vector engine (re-initialize embedding model)
  config     Reload configuration files
  skills     Reload skill definitions

Examples:
  ai-hub reload vector    # Reload vector model without restarting
  ai-hub reload config    # Reload configuration
  ai-hub reload skills    # Reload skill definitions`)
}

func reloadVector(c *client.Client) int {
	fmt.Println("Reloading vector engine...")
	resp, err := c.POST("/api/v1/reload/vector", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reload vector engine: %v\n", err)
		return 1
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err == nil {
		if msg, ok := result["message"].(string); ok {
			fmt.Println(msg)
			return 0
		}
	}
	fmt.Println("Vector engine reloaded successfully")
	return 0
}

func reloadConfig(c *client.Client) int {
	fmt.Println("Reloading configuration...")
	resp, err := c.POST("/api/v1/reload/config", nil)
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
	resp, err := c.POST("/api/v1/reload/skills", nil)
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

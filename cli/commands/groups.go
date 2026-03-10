package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// RunGroups executes the groups command
// Usage: ai-hub groups [name] [create|delete]
func RunGroups(c *client.Client, args []string) int {
	if len(args) == 0 {
		return listGroups(c)
	}

	// Check for create/delete subcommand
	if args[0] == "create" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub groups create <name> [--desc <description>]")
			return 1
		}
		return createGroup(c, args[1:])
	}

	if args[0] == "delete" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub groups delete <name>")
			return 1
		}
		return deleteGroup(c, args[1])
	}

	// Otherwise, treat first arg as group name for detail view
	return groupDetail(c, args[0])
}

func listGroups(c *client.Client) int {
	respData, err := c.GET("/groups")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var groups []struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		SessionCount int    `json:"session_count"`
		CreatedAt    string `json:"created_at"`
	}
	if err := json.Unmarshal(respData, &groups); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(groups) == 0 {
		fmt.Println("No groups found.")
		return 0
	}

	fmt.Printf("%d groups:\n\n", len(groups))
	for _, g := range groups {
		desc := g.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("%-20s  %d sessions  %s\n", g.Name, g.SessionCount, desc)
	}
	return 0
}

func groupDetail(c *client.Client, name string) int {
	respData, err := c.GET("/groups/" + name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		SessionCount int    `json:"session_count"`
		Sessions     []struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			UpdatedAt string `json:"updated_at"`
		} `json:"sessions"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("Group: %s\n", resp.Name)
	if resp.Description != "" {
		fmt.Printf("  Description: %s\n", resp.Description)
	}
	fmt.Printf("  Sessions: %d\n", resp.SessionCount)
	fmt.Printf("  Created: %s\n", FormatTime(resp.CreatedAt))

	if len(resp.Sessions) > 0 {
		fmt.Println("\nSessions:")
		for _, s := range resp.Sessions {
			fmt.Printf("  #%-4d %s  (%s)\n", s.ID, s.Title, FormatTime(s.UpdatedAt))
		}
	}
	return 0
}

func createGroup(c *client.Client, args []string) int {
	name := args[0]
	desc := ""

	// Parse --desc flag
	for i := 1; i < len(args); i++ {
		if args[i] == "--desc" && i+1 < len(args) {
			desc = args[i+1]
			break
		}
	}

	body := map[string]string{"name": name, "description": desc}
	respData, err := c.POST("/groups", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Name string `json:"name"`
	}
	json.Unmarshal(respData, &resp)
	fmt.Printf("Group '%s' created.\n", resp.Name)
	return 0
}

func deleteGroup(c *client.Client, name string) int {
	_, err := c.DELETE("/groups/" + name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Group '%s' deleted.\n", name)
	return 0
}

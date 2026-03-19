package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// RunSkills executes the skills command
func RunSkills(c *client.Client, args []string) int {
	if len(args) == 0 {
		return skillsList(c)
	}

	switch args[0] {
	case "list":
		return skillsList(c)
	case "read":
		return skillsRead(c, args[1:])
	case "create":
		return skillsCreate(c, args[1:])
	case "update":
		return skillsUpdate(c, args[1:])
	case "delete":
		return skillsDelete(c, args[1:])
	case "--help":
		printSkillsHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown skills subcommand: %s\n", args[0])
		printSkillsHelp()
		return 1
	}
}

func skillsList(c *client.Client) int {
	respData, err := c.GET("/skills")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var skills []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
		Enabled     bool   `json:"enabled"`
	}
	if err := json.Unmarshal(respData, &skills); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(skills) == 0 {
		fmt.Println("No skills found.")
		return 0
	}

	fmt.Printf("%d skills:\n", len(skills))
	for i, s := range skills {
		status := "✓"
		if !s.Enabled {
			status = "✗"
		}
		desc := s.Description
		if desc == "" {
			desc = "—"
		}
		fmt.Printf("  %d. [%s] %s (%s) - %s\n", i+1, status, s.Name, s.Source, desc)
	}
	return 0
}

func skillsRead(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub skills read <name>\n")
		return 1
	}

	name := args[0]
	respData, err := c.GET(fmt.Sprintf("/skills/%s", name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Name    string `json:"name"`
		Dir     string `json:"dir"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Print(resp.Content)
	if len(resp.Content) > 0 && resp.Content[len(resp.Content)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func skillsCreate(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub skills create <name> --content \"内容\"\n")
		return 1
	}

	name := args[0]
	var content string
	for i := 1; i < len(args); i++ {
		if args[i] == "--content" && i+1 < len(args) {
			i++
			content = args[i]
		}
	}

	if content == "" {
		fmt.Fprintf(os.Stderr, "Error: --content is required\n")
		return 1
	}

	body := map[string]string{
		"name":    name,
		"content": content,
	}
	_, err := c.POST("/skills", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Skill '%s' created.\n", name)
	return 0
}

func skillsUpdate(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub skills update <name> --content \"内容\"\n")
		return 1
	}

	name := args[0]
	var content string
	for i := 1; i < len(args); i++ {
		if args[i] == "--content" && i+1 < len(args) {
			i++
			content = args[i]
		}
	}

	if content == "" {
		fmt.Fprintf(os.Stderr, "Error: --content is required\n")
		return 1
	}

	body := map[string]string{
		"content": content,
	}
	_, err := c.PUT(fmt.Sprintf("/skills/%s", name), body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Skill '%s' updated.\n", name)
	return 0
}

func skillsDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub skills delete <name>\n")
		return 1
	}

	name := args[0]
	_, err := c.DELETE(fmt.Sprintf("/skills/%s", name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Skill '%s' deleted.\n", name)
	return 0
}

func printSkillsHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub skills [subcommand] [args]

Manage skills (SKILL.md files).

Subcommands:
  list                                 List all skills (default)
  read <name>                          Read skill full content
  create <name> --content "内容"       Create a new skill
  update <name> --content "内容"       Update skill content
  delete <name>                        Delete a skill

Examples:
  ai-hub skills
  ai-hub skills list
  ai-hub skills read ai-hub-core
  ai-hub skills create my-skill --content "---\nname: my-skill\n---\n# My Skill"
  ai-hub skills update my-skill --content "updated content"
  ai-hub skills delete my-skill
`)
}

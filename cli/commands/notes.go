package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// RunNotes executes the notes command
func RunNotes(c *client.Client, args []string) int {
	if len(args) == 0 {
		printNotesHelp()
		return 1
	}

	switch args[0] {
	case "list":
		return notesList(c)
	case "read":
		return notesRead(c, args[1:])
	case "write":
		return notesWrite(c, args[1:])
	case "delete":
		return notesDelete(c, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown notes subcommand: %s\n", args[0])
		printNotesHelp()
		return 1
	}
}

func notesList(c *client.Client) int {
	respData, err := c.GET("/files?scope=notes")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var files []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(respData, &files); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(files) == 0 {
		fmt.Println("No notes found.")
		return 0
	}

	fmt.Printf("%d notes:\n", len(files))
	for i, f := range files {
		fmt.Printf("  %d. %s\n", i+1, f.Name)
	}
	return 0
}

func notesRead(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub notes read <filename>\n")
		return 1
	}

	filename := args[0]
	respData, err := c.GET(fmt.Sprintf("/files/content?scope=notes&path=%s", filename))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if resp.Content == "" {
		fmt.Printf("Note '%s' is empty or not found.\n", filename)
		return 0
	}

	fmt.Print(resp.Content)
	if resp.Content[len(resp.Content)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func notesWrite(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub notes write <filename> --content \"内容\"\n")
		return 1
	}

	filename := args[0]
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
		"scope":   "notes",
		"path":    filename,
		"content": content,
	}
	_, err := c.PUT("/files/content", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Note '%s' written.\n", filename)
	return 0
}

func notesDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub notes delete <filename>\n")
		return 1
	}

	filename := args[0]
	_, err := c.DELETE(fmt.Sprintf("/files?scope=notes&path=%s", filename))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Note '%s' deleted.\n", filename)
	return 0
}

func printNotesHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub notes <subcommand> [args]

Manage notes files.

Subcommands:
  list                                List all notes
  read <filename>                     Read a note
  write <filename> --content "内容"   Write a note
  delete <filename>                   Delete a note

Examples:
  ai-hub notes list
  ai-hub notes read todo.md
  ai-hub notes write todo.md --content "# TODO\n- item 1"
  ai-hub notes delete todo.md
`)
}

package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// RunStatus executes the status command
func RunStatus(c *client.Client, args []string) int {
	respData, err := c.GET("/status")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var status map[string]interface{}
	if err := json.Unmarshal(respData, &status); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Print version first
	verData, err := c.GET("/version")
	if err == nil {
		var ver struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(verData, &ver) == nil {
			fmt.Printf("Version: %s\n\n", ver.Version)
		}
	}

	// Print status fields
	for k, v := range status {
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Printf("%s:\n", k)
			for sk, sv := range val {
				fmt.Printf("  %s: %v\n", sk, sv)
			}
		default:
			fmt.Printf("%s: %v\n", k, val)
		}
	}
	return 0
}

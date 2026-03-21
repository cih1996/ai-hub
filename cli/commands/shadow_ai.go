package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RunShadowAI executes the shadow-ai command
// Usage: ai-hub shadow-ai [enable|disable|status|config|logs]
func RunShadowAI(c *client.Client, args []string) int {
	if len(args) == 0 {
		// Default: show status
		return shadowStatus(c)
	}

	switch args[0] {
	case "status":
		return shadowStatus(c)
	case "enable":
		return shadowEnable(c)
	case "disable":
		return shadowDisable(c)
	case "config":
		return shadowConfig(c, args[1:])
	case "logs":
		return shadowLogs(c, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown shadow-ai subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: ai-hub shadow-ai [enable|disable|status|config|logs]")
		return 1
	}
}

func shadowStatus(c *client.Client) int {
	respData, err := c.GET("/shadow-ai/status")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var status struct {
		Enabled   bool   `json:"enabled"`
		SessionID int64  `json:"session_id"`
		Status    string `json:"status"`
		Config    struct {
			PatrolInterval        string `json:"patrol_interval"`
			ExtractInterval       string `json:"extract_interval"`
			DeepScanInterval      string `json:"deep_scan_interval"`
			SelfCleanInterval     string `json:"self_clean_interval"`
			ContextResetThreshold int    `json:"context_reset_threshold"`
		} `json:"config"`
		Triggers []struct {
			ID          int64  `json:"id"`
			Content     string `json:"content"`
			TriggerTime string `json:"trigger_time"`
			Enabled     bool   `json:"enabled"`
			Status      string `json:"status"`
			FiredCount  int    `json:"fired_count"`
		} `json:"triggers"`
	}
	if err := json.Unmarshal(respData, &status); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Display status
	enabledStr := "disabled"
	if status.Enabled {
		enabledStr = "enabled"
	}

	fmt.Printf("Shadow AI: %s\n", enabledStr)
	fmt.Printf("  Status: %s\n", status.Status)
	if status.SessionID > 0 {
		fmt.Printf("  Session: #%d\n", status.SessionID)
	}

	if status.Enabled {
		fmt.Printf("\nConfig:\n")
		fmt.Printf("  Patrol interval:    %s\n", status.Config.PatrolInterval)
		fmt.Printf("  Extract interval:   %s\n", status.Config.ExtractInterval)
		fmt.Printf("  Deep scan interval: %s\n", status.Config.DeepScanInterval)
		fmt.Printf("  Self clean interval: %s\n", status.Config.SelfCleanInterval)
		fmt.Printf("  Reset threshold:    %d messages\n", status.Config.ContextResetThreshold)

		if len(status.Triggers) > 0 {
			fmt.Printf("\nTriggers (%d):\n", len(status.Triggers))
			for _, t := range status.Triggers {
				state := "active"
				if !t.Enabled {
					state = "disabled"
				}
				preview := t.Content
				if len([]rune(preview)) > 40 {
					preview = string([]rune(preview)[:40]) + "..."
				}
				fmt.Printf("  #%-4d [%s] %s  (%s, fired:%d)\n", t.ID, state, preview, t.TriggerTime, t.FiredCount)
			}
		}
	}

	return 0
}

func shadowEnable(c *client.Client) int {
	respData, err := c.POST("/shadow-ai/enable", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		OK        bool    `json:"ok"`
		SessionID int64   `json:"session_id"`
		Triggers  []int64 `json:"triggers"`
		Message   string  `json:"message"`
	}
	json.Unmarshal(respData, &resp)

	if resp.Message != "" {
		fmt.Printf("Shadow AI: %s (session #%d)\n", resp.Message, resp.SessionID)
	} else {
		fmt.Printf("Shadow AI enabled!\n")
		fmt.Printf("  Session: #%d\n", resp.SessionID)
		fmt.Printf("  Triggers: %v\n", resp.Triggers)
	}
	return 0
}

func shadowDisable(c *client.Client) int {
	respData, err := c.POST("/shadow-ai/disable", map[string]interface{}{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	json.Unmarshal(respData, &resp)
	fmt.Println(resp.Message)
	return 0
}

func shadowConfig(c *client.Client, args []string) int {
	// If --set flags provided, update config
	if len(args) > 0 {
		configMap := make(map[string]interface{})
		for _, arg := range args {
			if strings.HasPrefix(arg, "--") {
				continue
			}
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				switch key {
				case "patrol_interval":
					configMap["patrol_interval"] = value
				case "extract_interval":
					configMap["extract_interval"] = value
				case "deep_scan_interval":
					configMap["deep_scan_interval"] = value
				case "self_clean_interval":
					configMap["self_clean_interval"] = value
				case "context_reset_threshold":
					if v, err := fmt.Sscanf(value, "%d"); err == nil && v > 0 {
						configMap["context_reset_threshold"] = v
					}
				}
			}
		}

		if len(configMap) > 0 {
			respData, err := c.PUT("/shadow-ai/config", configMap)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			var resp struct {
				OK     bool `json:"ok"`
				Config struct {
					PatrolInterval        string `json:"patrol_interval"`
					ExtractInterval       string `json:"extract_interval"`
					DeepScanInterval      string `json:"deep_scan_interval"`
					SelfCleanInterval     string `json:"self_clean_interval"`
					ContextResetThreshold int    `json:"context_reset_threshold"`
				} `json:"config"`
			}
			json.Unmarshal(respData, &resp)
			fmt.Println("Config updated:")
			fmt.Printf("  Patrol interval:     %s\n", resp.Config.PatrolInterval)
			fmt.Printf("  Extract interval:    %s\n", resp.Config.ExtractInterval)
			fmt.Printf("  Deep scan interval:  %s\n", resp.Config.DeepScanInterval)
			fmt.Printf("  Self clean interval: %s\n", resp.Config.SelfCleanInterval)
			fmt.Printf("  Reset threshold:     %d messages\n", resp.Config.ContextResetThreshold)
			return 0
		}
	}

	// Default: show current config
	return shadowStatus(c)
}

func shadowLogs(c *client.Client, args []string) int {
	lines := "50"
	for i, arg := range args {
		if arg == "--lines" && i+1 < len(args) {
			lines = args[i+1]
		}
	}

	respData, err := c.GET(fmt.Sprintf("/shadow-ai/logs?lines=%s", lines))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Content string `json:"content"`
		Exists  bool   `json:"exists"`
	}
	json.Unmarshal(respData, &resp)

	if !resp.Exists {
		fmt.Println("No shadow AI logs found. Enable shadow AI first.")
		return 0
	}

	fmt.Println(resp.Content)
	return 0
}

package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RunService executes the service command.
func RunService(c *client.Client, args []string) int {
	if len(args) == 0 {
		return serviceList(c)
	}
	switch args[0] {
	case "list":
		return serviceList(c)
	case "info":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub service info <name|id>")
			return 1
		}
		return serviceInfo(c, args[1])
	case "start":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub service start <name|id>")
			return 1
		}
		return serviceAction(c, args[1], "start")
	case "stop":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub service stop <name|id>")
			return 1
		}
		return serviceAction(c, args[1], "stop")
	case "restart":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub service restart <name|id>")
			return 1
		}
		return serviceAction(c, args[1], "restart")
	case "logs":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub service logs <name|id> [--lines N]")
			return 1
		}
		lines := 100
		for i := 2; i < len(args); i++ {
			if args[i] == "--lines" && i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &lines)
			}
		}
		return serviceLogs(c, args[1], lines)
	case "create":
		return serviceCreate(c, args[1:])
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ai-hub service delete <name|id>")
			return 1
		}
		return serviceDelete(c, args[1])
	default:
		// Treat as name/id for info
		return serviceInfo(c, args[0])
	}
}

type svcJSON struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Command   string `json:"command"`
	WorkDir   string `json:"work_dir"`
	Port      int    `json:"port"`
	LogPath   string `json:"log_path"`
	PID       int    `json:"pid"`
	Status    string `json:"status"`
	AutoStart bool   `json:"auto_start"`
}

func serviceList(c *client.Client) int {
	data, err := c.GET("/services")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	var services []svcJSON
	json.Unmarshal(data, &services)
	if len(services) == 0 {
		fmt.Println("No services configured.")
		return 0
	}
	fmt.Printf("%-4s %-20s %-10s %-8s %s\n", "ID", "Name", "Status", "PID", "Port")
	fmt.Println(strings.Repeat("-", 60))
	for _, s := range services {
		status := statusIcon(s.Status) + " " + s.Status
		pid := "-"
		if s.PID > 0 {
			pid = strconv.Itoa(s.PID)
		}
		port := "-"
		if s.Port > 0 {
			port = strconv.Itoa(s.Port)
		}
		fmt.Printf("%-4d %-20s %-10s %-8s %s\n", s.ID, s.Name, status, pid, port)
	}
	return 0
}

func serviceInfo(c *client.Client, nameOrID string) int {
	svc, err := resolveServiceCLI(c, nameOrID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Service #%d — %s\n", svc.ID, svc.Name)
	fmt.Printf("  状态: %s %s\n", statusIcon(svc.Status), svc.Status)
	fmt.Printf("  命令: %s\n", svc.Command)
	fmt.Printf("  目录: %s\n", svc.WorkDir)
	fmt.Printf("  端口: %d\n", svc.Port)
	fmt.Printf("  PID:  %d\n", svc.PID)
	fmt.Printf("  日志: %s\n", svc.LogPath)
	fmt.Printf("  自启: %v\n", svc.AutoStart)
	return 0
}

func serviceAction(c *client.Client, nameOrID, action string) int {
	svc, err := resolveServiceCLI(c, nameOrID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	data, err := c.Request("POST", fmt.Sprintf("/services/%d/%s", svc.ID, action), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	var result svcJSON
	json.Unmarshal(data, &result)
	fmt.Printf("%s %s — %s (PID: %d)\n", statusIcon(result.Status), result.Name, result.Status, result.PID)
	return 0
}

func serviceLogs(c *client.Client, nameOrID string, lines int) int {
	svc, err := resolveServiceCLI(c, nameOrID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	data, err := c.GET(fmt.Sprintf("/services/%d/logs?lines=%d", svc.ID, lines))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	var resp struct {
		Logs  string `json:"logs"`
		Error string `json:"error"`
	}
	json.Unmarshal(data, &resp)
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", resp.Error)
	}
	if resp.Logs != "" {
		fmt.Println(resp.Logs)
	} else {
		fmt.Println("(empty log)")
	}
	return 0
}

func serviceCreate(c *client.Client, args []string) int {
	var name, command, workDir string
	var port int
	var autoStart bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				i++
				name = args[i]
			}
		case "--command":
			if i+1 < len(args) {
				i++
				command = args[i]
			}
		case "--work-dir":
			if i+1 < len(args) {
				i++
				workDir = args[i]
			}
		case "--port":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &port)
			}
		case "--auto-start":
			autoStart = true
		}
	}

	if name == "" || command == "" {
		fmt.Fprintln(os.Stderr, "Usage: ai-hub service create --name <name> --command <cmd> [--port N] [--work-dir dir] [--auto-start]")
		return 1
	}

	body := map[string]interface{}{
		"name": name, "command": command, "work_dir": workDir,
		"port": port, "auto_start": autoStart,
	}
	data, err := c.Request("POST", "/services", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	var svc svcJSON
	json.Unmarshal(data, &svc)
	fmt.Printf("Created service #%d — %s\n", svc.ID, svc.Name)
	return 0
}

func serviceDelete(c *client.Client, nameOrID string) int {
	svc, err := resolveServiceCLI(c, nameOrID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	_, err = c.Request("DELETE", fmt.Sprintf("/services/%d", svc.ID), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Deleted service #%d — %s\n", svc.ID, svc.Name)
	return 0
}

// resolveServiceCLI resolves a service by name or ID via API.
func resolveServiceCLI(c *client.Client, nameOrID string) (*svcJSON, error) {
	// Try as ID first
	if id, err := strconv.ParseInt(nameOrID, 10, 64); err == nil {
		data, err := c.GET(fmt.Sprintf("/services/%d", id))
		if err == nil {
			var svc svcJSON
			if json.Unmarshal(data, &svc) == nil && svc.ID > 0 {
				return &svc, nil
			}
		}
	}
	// Try by name: list all and find
	data, err := c.GET("/services")
	if err != nil {
		return nil, err
	}
	var services []svcJSON
	json.Unmarshal(data, &services)
	for _, s := range services {
		if strings.EqualFold(s.Name, nameOrID) {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("service %q not found", nameOrID)
}

func statusIcon(status string) string {
	switch status {
	case "running":
		return "🟢"
	case "dead":
		return "🔴"
	default:
		return "⚪"
	}
}

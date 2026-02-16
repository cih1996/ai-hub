package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ClaudeCodeClient wraps the claude CLI
type ClaudeCodeClient struct {
	BinaryPath string
}

type StreamEvent struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type ClaudeCodeRequest struct {
	Query        string
	SessionID    string
	SystemPrompt string
	MaxBudget    float64
	// Provider config - injected as env vars
	BaseURL string
	APIKey  string
	ModelID string
}

func NewClaudeCodeClient() *ClaudeCodeClient {
	return &ClaudeCodeClient{BinaryPath: "claude"}
}

func (c *ClaudeCodeClient) Stream(ctx context.Context, req ClaudeCodeRequest, onData func(string)) error {
	args := []string{"-p", req.Query, "--output-format", "stream-json", "--verbose"}

	if req.SessionID != "" {
		args = append(args, "--session-id", req.SessionID)
	}
	if req.SystemPrompt != "" {
		args = append(args, "--system-prompt", req.SystemPrompt)
	}
	if req.MaxBudget > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", req.MaxBudget))
	}
	if req.ModelID != "" {
		args = append(args, "--model", req.ModelID)
	}

	// System-level, max permissions, no workspace restriction
	args = append(args, "--dangerously-skip-permissions")
	args = append(args, "--permission-mode", "full")

	cmd := exec.CommandContext(ctx, c.BinaryPath, args...)

	// Working directory: use home dir (system-level, not tied to any project)
	home, _ := os.UserHomeDir()
	cmd.Dir = home

	// Inherit current env, then override with provider config
	cmd.Env = os.Environ()
	if req.APIKey != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+req.APIKey)
	}
	if req.BaseURL != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+req.BaseURL)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude: %w", err)
	}

	go func() {
		buf, _ := io.ReadAll(stderr)
		if len(buf) > 0 {
			onData(fmt.Sprintf("[stderr] %s", string(buf)))
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		onData(line)
	}

	return cmd.Wait()
}

package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

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
	Resume       bool // true = continue existing session (--resume), false = new session (--session-id)
	SystemPrompt string
	MaxBudget    float64
	BaseURL      string
	APIKey       string
	ModelID      string
	WorkDir      string // 工作目录，空 = home
	HubSessionID int64  // AI Hub 会话 ID，注入为环境变量
}

func NewClaudeCodeClient() *ClaudeCodeClient {
	return &ClaudeCodeClient{BinaryPath: "claude"}
}

func (c *ClaudeCodeClient) Stream(ctx context.Context, req ClaudeCodeRequest, onData func(string)) error {
	// Build flags first, query last — matches documented CLI patterns
	args := []string{
		"-p",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	}

	if req.SessionID != "" {
		if req.Resume {
			// Continue existing CLI session
			args = append(args, "--resume", req.SessionID)
		} else {
			// Create new CLI session with specific UUID
			args = append(args, "--session-id", req.SessionID)
		}
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

	// Max permissions - skip all permission prompts
	args = append(args, "--dangerously-skip-permissions")

	// Query must be the last positional argument
	args = append(args, req.Query)

	cmd := exec.CommandContext(ctx, c.BinaryPath, args...)

	// Set working directory: use specified work_dir or fall back to home
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	} else {
		home, _ := os.UserHomeDir()
		cmd.Dir = home
	}

	// Inject provider config as env vars
	cmd.Env = os.Environ()
	if req.APIKey != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+req.APIKey)
	}
	if req.BaseURL != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+req.BaseURL)
	}
	if req.HubSessionID > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("AI_HUB_SESSION_ID=%d", req.HubSessionID))
	}
	if p := GetPort(); p != "" {
		cmd.Env = append(cmd.Env, "AI_HUB_PORT="+p)
	}

	log.Printf("[claude] cmd: %s %s", c.BinaryPath, strings.Join(args, " "))
	log.Printf("[claude] env: ANTHROPIC_BASE_URL=%s ANTHROPIC_API_KEY=%s...", req.BaseURL, maskKey(req.APIKey))

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

	// Collect stderr in background
	var stderrBuf strings.Builder
	var stderrMu sync.Mutex
	go func() {
		buf, _ := io.ReadAll(stderr)
		stderrMu.Lock()
		stderrBuf.Write(buf)
		stderrMu.Unlock()
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	gotData := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		gotData = true
		onData(line)
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		stderrMu.Lock()
		errOutput := strings.TrimSpace(stderrBuf.String())
		stderrMu.Unlock()
		log.Printf("[claude] exit error: %v, stderr: %s", waitErr, errOutput)
		if errOutput != "" {
			return fmt.Errorf("claude CLI error: %s", errOutput)
		}
		// If we got valid data on stdout but stderr is empty, treat as success
		// Claude CLI sometimes exits with status 1 even after producing valid output
		if gotData {
			log.Printf("[claude] ignoring exit status (got valid output)")
			return nil
		}
		return fmt.Errorf("claude CLI failed: %w", waitErr)
	}
	return nil
}

// StreamPersistent uses the process pool for persistent CLI processes.
// Falls back to one-shot Stream() if pool is unavailable or process fails.
func (c *ClaudeCodeClient) StreamPersistent(ctx context.Context, req ClaudeCodeRequest, onData func(string)) error {
	if Pool == nil {
		return c.Stream(ctx, req, onData)
	}

	proc, err := Pool.GetOrCreate(req, req.Resume)
	if err != nil {
		log.Printf("[pool] GetOrCreate failed, falling back: %v", err)
		return c.Stream(ctx, req, onData)
	}

	err = proc.SendAndStream(ctx, req.Query, onData)
	if err == nil || ctx.Err() != nil {
		return err
	}

	// Process died unexpectedly — retry with resume, then fallback to new session
	if proc.IsDead() {
		Pool.Kill(req.HubSessionID)

		// First retry: resume existing session
		log.Printf("[pool] process died, retrying with --resume for session %d", req.HubSessionID)
		req.Resume = true
		proc, err = Pool.GetOrCreate(req, true)
		if err == nil {
			err = proc.SendAndStream(ctx, req.Query, onData)
			if err == nil {
				return nil
			}
		}
	}

	// Second retry: start fresh (--session-id) — always attempt regardless of proc state
	Pool.Kill(req.HubSessionID)
	log.Printf("[pool] resume failed or process error, retrying with new session for session %d", req.HubSessionID)
	req.Resume = false
	proc, err = Pool.GetOrCreate(req, false)
	if err != nil {
		log.Printf("[pool] new session failed, falling back: %v", err)
		return c.Stream(ctx, req, onData)
	}
	return proc.SendAndStream(ctx, req.Query, onData)
}

func maskKey(key string) string {
	if len(key) < 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}

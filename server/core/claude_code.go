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
	"path/filepath"
	"strings"
	"sync"
)

// writeSystemPromptFile writes the system prompt to a UTF-8 temp file and returns
// the file path. Caller must os.Remove() the file when done.
// This avoids Windows CreateProcess 32767-char limit and GBK/UTF-8 encoding issues.
func writeSystemPromptFile(hubSessionID int64, content string) (string, error) {
	dir := filepath.Join(os.TempDir(), "ai-hub")
	os.MkdirAll(dir, 0755)
	name := filepath.Join(dir, fmt.Sprintf("sysprompt-%d.md", hubSessionID))
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write system prompt file: %w", err)
	}
	return name, nil
}

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
	AuthMode     string // "api_key" | "oauth"
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
		"--dangerously-skip-permissions",
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
	// Write system prompt to temp file to avoid Windows CreateProcess 32767-char
	// limit and GBK/UTF-8 encoding issues (Issue #44)
	var promptFile string
	if req.SystemPrompt != "" {
		var err error
		promptFile, err = writeSystemPromptFile(req.HubSessionID, req.SystemPrompt)
		if err != nil {
			return fmt.Errorf("write system prompt: %w", err)
		}
		defer os.Remove(promptFile)
		args = append(args, "--system-prompt-file", promptFile)
	}
	if req.MaxBudget > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", req.MaxBudget))
	}
	if req.ModelID != "" {
		args = append(args, "--model", req.ModelID)
	}

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
	if req.AuthMode != "oauth" {
		if req.APIKey != "" {
			cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+req.APIKey)
		}
		// Route through local proxy for precise token metering (Issue #72)
		if port := GetPort(); port != "" && req.HubSessionID > 0 {
			proxyURL := fmt.Sprintf("http://localhost:%s/api/v1/proxy/anthropic?session_id=%d", port, req.HubSessionID)
			cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+proxyURL)
		} else if req.BaseURL != "" {
			cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+req.BaseURL)
		}
	}
	// OAuth mode: no API key or base URL injection, CLI uses local OAuth token
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

	// CLI returned error_during_execution — session state corrupted, skip resume, start fresh
	if strings.HasPrefix(err.Error(), "cli_error:") {
		log.Printf("[pool] session %d: CLI error detected (%v), killing process and rebuilding fresh session", req.HubSessionID, err)
		Pool.Kill(req.HubSessionID)
		req.Resume = false
		req.SessionID = "" // force new CLI session
		proc, err = Pool.GetOrCreate(req, false)
		if err != nil {
			log.Printf("[pool] new session failed, falling back: %v", err)
			return c.Stream(ctx, req, onData)
		}
		return proc.SendAndStream(ctx, req.Query, onData)
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
			// If resume returned cli_error, go straight to new session
			if strings.HasPrefix(err.Error(), "cli_error:") {
				log.Printf("[pool] session %d: resume returned CLI error, rebuilding fresh", req.HubSessionID)
				Pool.Kill(req.HubSessionID)
				req.Resume = false
				req.SessionID = ""
				proc, err = Pool.GetOrCreate(req, false)
				if err != nil {
					return c.Stream(ctx, req, onData)
				}
				return proc.SendAndStream(ctx, req.Query, onData)
			}
		}

		// Second retry: start fresh (--session-id) if resume also failed
		if proc == nil || proc.IsDead() {
			Pool.Kill(req.HubSessionID)
			log.Printf("[pool] resume failed, retrying with new session for session %d", req.HubSessionID)
			req.Resume = false
			proc, err = Pool.GetOrCreate(req, false)
			if err != nil {
				log.Printf("[pool] new session failed, falling back: %v", err)
				return c.Stream(ctx, req, onData)
			}
			return proc.SendAndStream(ctx, req.Query, onData)
		}
		return err
	}
	return err
}

func maskKey(key string) string {
	if len(key) < 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}

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
	"time"
)

// PersistentProcess wraps a long-running Claude CLI process (stream-json mode)
type PersistentProcess struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	cancel       context.CancelFunc
	sessionID    string // Claude CLI --session-id UUID
	hubSessionID int64  // AI Hub session ID
	state        string // "idle" | "busy"
	lastActive   time.Time
	startedAt    time.Time
	dead         bool
	eventCh      chan string   // stdout NDJSON lines
	doneCh       chan struct{} // process exit signal
}

// ProcessPool manages persistent Claude CLI processes keyed by hub session ID
type ProcessPool struct {
	mu        sync.RWMutex
	processes map[int64]*PersistentProcess
	client    *ClaudeCodeClient
	stopCh    chan struct{}
}

// Pool is the global process pool instance
var Pool *ProcessPool

// InitPool creates and starts the global process pool
func InitPool(client *ClaudeCodeClient) {
	Pool = &ProcessPool{
		processes: make(map[int64]*PersistentProcess),
		client:    client,
		stopCh:    make(chan struct{}),
	}
	go Pool.idleReaper()
	log.Println("[pool] initialized")
}

// ShutdownPool kills all persistent processes
func (p *ProcessPool) ShutdownPool() {
	close(p.stopCh)
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, proc := range p.processes {
		proc.kill()
		delete(p.processes, id)
	}
	log.Println("[pool] shutdown complete")
}

// GetOrCreate returns an existing live process or spawns a new one
func (p *ProcessPool) GetOrCreate(req ClaudeCodeRequest, isResume bool) (*PersistentProcess, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if proc, ok := p.processes[req.HubSessionID]; ok {
		if !proc.IsDead() {
			return proc, nil
		}
		proc.kill()
		delete(p.processes, req.HubSessionID)
		isResume = true
	}

	proc, err := p.spawnProcess(req, isResume)
	if err != nil {
		return nil, err
	}
	p.processes[req.HubSessionID] = proc
	return proc, nil
}

// Kill terminates the process for a given hub session ID
func (p *ProcessPool) Kill(hubSessionID int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if proc, ok := p.processes[hubSessionID]; ok {
		proc.kill()
		delete(p.processes, hubSessionID)
		log.Printf("[pool] killed process for session %d", hubSessionID)
	}
}

// idleReaper periodically cleans up idle/dead processes
func (p *ProcessPool) idleReaper() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.mu.Lock()
			now := time.Now()
			for id, proc := range p.processes {
				proc.mu.Lock()
				idle := proc.state == "idle" && now.Sub(proc.lastActive) > 30*time.Minute
				isDead := proc.dead
				proc.mu.Unlock()
				if idle || isDead {
					proc.kill()
					delete(p.processes, id)
					log.Printf("[pool] reaped process for session %d (idle=%v dead=%v)", id, idle, isDead)
				}
			}
			p.mu.Unlock()
		}
	}
}

// spawnProcess starts a new persistent Claude CLI process
func (p *ProcessPool) spawnProcess(req ClaudeCodeRequest, isResume bool) (*PersistentProcess, error) {
	args := []string{
		"-p",
		"--verbose",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
	}
	if req.SessionID != "" {
		if isResume {
			args = append(args, "--resume", req.SessionID)
		} else {
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
	args = append(args, "--dangerously-skip-permissions")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, p.client.BinaryPath, args...)

	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	} else {
		home, _ := os.UserHomeDir()
		cmd.Dir = home
	}

	// Filter out CLAUDECODE env var to avoid nested session detection
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	if req.APIKey != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+req.APIKey)
	}
	if req.BaseURL != "" {
		cmd.Env = append(cmd.Env, "ANTHROPIC_BASE_URL="+req.BaseURL)
	}
	if req.HubSessionID > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("AI_HUB_SESSION_ID=%d", req.HubSessionID))
	}
	if port := GetPort(); port != "" {
		cmd.Env = append(cmd.Env, "AI_HUB_PORT="+port)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start claude: %w", err)
	}

	log.Printf("[pool] spawned pid=%d session=%d args: %s %s",
		cmd.Process.Pid, req.HubSessionID, p.client.BinaryPath, strings.Join(args, " "))

	// Collect stderr in background
	var stderrBuf strings.Builder
	var stderrMu sync.Mutex
	go func() {
		buf, _ := io.ReadAll(stderr)
		stderrMu.Lock()
		stderrBuf.Write(buf)
		stderrMu.Unlock()
		if len(buf) > 0 {
			log.Printf("[pool] session %d stderr: %s", req.HubSessionID, strings.TrimSpace(string(buf)))
		}
	}()

	proc := &PersistentProcess{
		cmd:          cmd,
		stdin:        stdin,
		cancel:       cancel,
		sessionID:    req.SessionID,
		hubSessionID: req.HubSessionID,
		state:        "idle",
		lastActive:   time.Now(),
		startedAt:    time.Now(),
		eventCh:      make(chan string, 256),
		doneCh:       make(chan struct{}),
	}
	go proc.readLoop(stdout)
	return proc, nil
}

// readLoop continuously reads stdout NDJSON lines into eventCh
func (proc *PersistentProcess) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		proc.eventCh <- line
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[pool] session %d scanner error: %v", proc.hubSessionID, err)
	}
	// Wait for process to fully exit and capture exit code
	if proc.cmd != nil {
		if waitErr := proc.cmd.Wait(); waitErr != nil {
			log.Printf("[pool] session %d process exited: %v", proc.hubSessionID, waitErr)
		}
	}
	proc.mu.Lock()
	proc.dead = true
	proc.mu.Unlock()
	close(proc.doneCh)
}

// SendAndStream writes a query to stdin and streams response events via onData
func (proc *PersistentProcess) SendAndStream(ctx context.Context, query string, onData func(string)) error {
	proc.mu.Lock()
	if proc.dead {
		proc.mu.Unlock()
		return fmt.Errorf("process is dead")
	}
	if proc.state == "busy" {
		proc.mu.Unlock()
		return fmt.Errorf("process is busy")
	}
	proc.state = "busy"
	proc.lastActive = time.Now()
	proc.mu.Unlock()

	defer func() {
		proc.mu.Lock()
		if !proc.dead {
			proc.state = "idle"
			proc.lastActive = time.Now()
		}
		proc.mu.Unlock()
	}()

	// Write NDJSON message to stdin
	// Claude CLI stream-json expects: {"type":"user","message":{"role":"user","content":"..."}}
	msg := map[string]interface{}{
		"type": "user",
		"message": map[string]string{
			"role":    "user",
			"content": query,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')
	if _, err := proc.stdin.Write(data); err != nil {
		return fmt.Errorf("write stdin: %w", err)
	}
	// Read events until "result" type or process death
	for {
		select {
		case <-ctx.Done():
			proc.kill()
			return ctx.Err()
		case line, ok := <-proc.eventCh:
			if !ok {
				return fmt.Errorf("event channel closed")
			}
			onData(line)
			var evt struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(line), &evt) == nil && evt.Type == "result" {
				return nil
			}
		case <-proc.doneCh:
			// Process died — drain remaining buffered events
			for {
				select {
				case line := <-proc.eventCh:
					onData(line)
				default:
					return fmt.Errorf("process exited unexpectedly")
				}
			}
		}
	}
}

// IsDead returns whether the process has exited
func (proc *PersistentProcess) IsDead() bool {
	proc.mu.Lock()
	defer proc.mu.Unlock()
	return proc.dead
}

// kill terminates the process
func (proc *PersistentProcess) kill() {
	proc.mu.Lock()
	defer proc.mu.Unlock()
	if proc.cancel != nil {
		proc.cancel()
	}
	if proc.cmd != nil && proc.cmd.Process != nil {
		proc.cmd.Process.Kill()
	}
	proc.dead = true
}

// ProcessInfo holds runtime info about a persistent process
type ProcessInfo struct {
	HubSessionID int64  `json:"hub_session_id"`
	Pid          int    `json:"pid"`
	State        string `json:"state"`
	UptimeSec    int64  `json:"uptime_sec"`
	IdleSec      int64  `json:"idle_sec"`
}

// Status returns info for all live processes
func (p *ProcessPool) Status() map[int64]ProcessInfo {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	now := time.Now()
	result := make(map[int64]ProcessInfo, len(p.processes))
	for id, proc := range p.processes {
		proc.mu.Lock()
		info := ProcessInfo{
			HubSessionID: id,
			State:        proc.state,
			UptimeSec:    int64(now.Sub(proc.startedAt).Seconds()),
			IdleSec:      int64(now.Sub(proc.lastActive).Seconds()),
		}
		if proc.cmd != nil && proc.cmd.Process != nil {
			info.Pid = proc.cmd.Process.Pid
		}
		proc.mu.Unlock()
		result[id] = info
	}
	return result
}

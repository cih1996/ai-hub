package core

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DepsStatus holds the status of all external dependencies
type DepsStatus struct {
	mu sync.RWMutex

	NodeInstalled   bool   `json:"node_installed"`
	NodeVersion     string `json:"node_version"`
	NpmInstalled    bool   `json:"npm_installed"`
	NpmVersion      string `json:"npm_version"`
	NpmGlobalDir    string `json:"npm_global_dir"`
	ClaudeInstalled bool   `json:"claude_installed"`
	ClaudeVersion   string `json:"claude_version"`
	Installing      bool   `json:"installing"`
	InstallError    string `json:"install_error"`
	InstallHint     string `json:"install_hint"`
}

var Deps = &DepsStatus{}

// CheckAll detects all dependencies and auto-installs claude if possible
func (d *DepsStatus) CheckAll() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. Check Node.js
	if out, err := runCmd("node", "--version"); err == nil {
		d.NodeInstalled = true
		d.NodeVersion = strings.TrimSpace(out)
	} else {
		d.NodeInstalled = false
		log.Println("[deps] Node.js not found. Please install from https://nodejs.org")
	}

	// 2. Check npm
	if out, err := runCmd("npm", "--version"); err == nil {
		d.NpmInstalled = true
		d.NpmVersion = strings.TrimSpace(out)
	} else {
		d.NpmInstalled = false
		log.Println("[deps] npm not found.")
	}

	// 3. Get npm global prefix for cleanup
	if d.NpmInstalled {
		if out, err := runCmd("npm", "root", "-g"); err == nil {
			d.NpmGlobalDir = strings.TrimSpace(out)
		}
	}

	// 4. Check claude CLI
	d.checkClaude()

	d.logStatus()
}

func (d *DepsStatus) checkClaude() {
	if out, err := runCmd("claude", "--version"); err == nil {
		d.ClaudeInstalled = true
		d.ClaudeVersion = strings.TrimSpace(out)
	} else {
		d.ClaudeInstalled = false
		d.ClaudeVersion = ""
	}
}

// AutoInstallClaude installs claude CLI via npm if not present
func (d *DepsStatus) AutoInstallClaude() {
	d.mu.RLock()
	if d.ClaudeInstalled || d.Installing || !d.NpmInstalled {
		d.mu.RUnlock()
		return
	}
	d.mu.RUnlock()

	d.mu.Lock()
	d.Installing = true
	d.InstallError = ""
	d.InstallHint = ""
	d.mu.Unlock()

	go func() {
		log.Println("[deps] Installing @anthropic-ai/claude-code ...")

		// Step 1: Clean up stale directory that causes ENOTEMPTY
		d.mu.RLock()
		globalDir := d.NpmGlobalDir
		d.mu.RUnlock()

		if globalDir != "" {
			staleDir := filepath.Join(globalDir, "@anthropic-ai", "claude-code")
			// Try removing the old package dir first to avoid ENOTEMPTY
			rmCmd := exec.Command("rm", "-rf", staleDir)
			if rmOut, rmErr := rmCmd.CombinedOutput(); rmErr != nil {
				log.Printf("[deps] Cleanup hint: could not remove %s: %s %s", staleDir, rmErr, strings.TrimSpace(string(rmOut)))
			} else {
				log.Printf("[deps] Cleaned stale directory: %s", staleDir)
			}
		}

		// Step 2: Install with --force to handle any remaining conflicts
		cmd := exec.Command("npm", "install", "-g", "--force", "@anthropic-ai/claude-code")
		output, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(output))

		d.mu.Lock()
		defer d.mu.Unlock()
		d.Installing = false

		if err != nil {
			d.InstallError = parseInstallError(outStr)
			d.InstallHint = generateHint(outStr, globalDir)
			log.Printf("[deps] Install failed: %s", d.InstallError)
			if d.InstallHint != "" {
				log.Printf("[deps] Hint: %s", d.InstallHint)
			}
			return
		}

		log.Println("[deps] Claude Code CLI installed successfully")
		d.checkClaude()
		d.logStatus()
	}()
}

// GetStatus returns a snapshot of current dependency status
func (d *DepsStatus) GetStatus() DepsStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return DepsStatus{
		NodeInstalled:   d.NodeInstalled,
		NodeVersion:     d.NodeVersion,
		NpmInstalled:    d.NpmInstalled,
		NpmVersion:      d.NpmVersion,
		ClaudeInstalled: d.ClaudeInstalled,
		ClaudeVersion:   d.ClaudeVersion,
		Installing:      d.Installing,
		InstallError:    d.InstallError,
		InstallHint:     d.InstallHint,
	}
}

// RetryInstall allows manual retry from frontend
func (d *DepsStatus) RetryInstall() {
	d.mu.Lock()
	d.InstallError = ""
	d.InstallHint = ""
	d.mu.Unlock()
	d.AutoInstallClaude()
}

func (d *DepsStatus) logStatus() {
	log.Printf("[deps] Node: %v (%s) | npm: %v (%s) | Claude: %v (%s)",
		d.NodeInstalled, d.NodeVersion,
		d.NpmInstalled, d.NpmVersion,
		d.ClaudeInstalled, d.ClaudeVersion,
	)
}

// parseInstallError extracts a human-readable error from npm output
func parseInstallError(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for the key error lines
		if strings.Contains(line, "ENOTEMPTY") {
			return "Directory conflict: a previous installation left stale files."
		}
		if strings.Contains(line, "EACCES") || strings.Contains(line, "permission denied") {
			return "Permission denied. Try running with sudo or fix npm permissions."
		}
		if strings.Contains(line, "ENOTFOUND") || strings.Contains(line, "network") {
			return "Network error. Check your internet connection."
		}
		if strings.Contains(line, "ERESOLVE") {
			return "Dependency conflict. Try: npm install -g --force @anthropic-ai/claude-code"
		}
	}
	// Fallback: return last non-empty npm error line
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" && strings.HasPrefix(l, "npm error") {
			return strings.TrimPrefix(l, "npm error ")
		}
	}
	if len(output) > 200 {
		return output[:200] + "..."
	}
	return output
}

// generateHint provides actionable fix suggestions
func generateHint(output string, globalDir string) string {
	if strings.Contains(output, "ENOTEMPTY") {
		dir := ""
		if globalDir != "" {
			dir = filepath.Join(globalDir, "@anthropic-ai")
		}
		if dir != "" {
			return fmt.Sprintf("Run: sudo rm -rf %s && npm install -g @anthropic-ai/claude-code", dir)
		}
		return "Run: sudo npm install -g --force @anthropic-ai/claude-code"
	}
	if strings.Contains(output, "EACCES") || strings.Contains(output, "permission denied") {
		return "Run: sudo npm install -g @anthropic-ai/claude-code"
	}
	if strings.Contains(output, "ENOTFOUND") {
		return "Check your network connection or proxy settings."
	}
	return ""
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

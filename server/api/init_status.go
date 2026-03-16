package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

// InitStatusResponse represents the system initialization status
type InitStatusResponse struct {
	IsFirstRun     bool            `json:"is_first_run"`
	HasProvider    bool            `json:"has_provider"`
	HasSession     bool            `json:"has_session"`
	MissingDeps    []MissingDep    `json:"missing_deps"`
	DepsStatus     core.DepsStatus `json:"deps_status"`
	Platform       string          `json:"platform"`        // darwin, linux, windows
	PackageManager string          `json:"package_manager"` // brew, apt, winget, choco, none
	ClaudeAuth     *ClaudeAuthInfo `json:"claude_auth,omitempty"`
	RunningAsRoot  bool            `json:"running_as_root"`
	CurrentUser    string          `json:"current_user"`
}

// ClaudeAuthInfo represents Claude CLI authentication status
type ClaudeAuthInfo struct {
	LoggedIn   bool   `json:"logged_in"`
	AuthMethod string `json:"auth_method,omitempty"` // claude.ai, api_key
	Email      string `json:"email,omitempty"`
	Error      string `json:"error,omitempty"`
}

// MissingDep represents a missing dependency
type MissingDep struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InstallCmd  string `json:"install_cmd,omitempty"`
	InstallURL  string `json:"install_url,omitempty"`
	NeedsSudo   bool   `json:"needs_sudo,omitempty"`
	CopyCmd     string `json:"copy_cmd,omitempty"` // Command to copy (without sudo prefix for display)
	Required    bool   `json:"required"`
	Hint        string `json:"hint,omitempty"` // Additional hint for user
}

// Package manager detection cache
var detectedPkgMgr string
var pkgMgrChecked bool

// GetInitStatus returns the system initialization status
func GetInitStatus(c *gin.Context) {
	forceFirstRun := c.Query("force_first_run") == "true"

	providers, _ := store.ListProviders()
	hasProvider := len(providers) > 0

	sessions, _ := store.ListSessions()
	hasSession := len(sessions) > 0

	isFirstRun := !hasProvider && !hasSession
	if forceFirstRun {
		isFirstRun = true
	}

	pkgMgr := detectPackageManager()
	missingDeps := checkMissingDeps(pkgMgr)
	depsStatus := core.Deps.GetStatus()

	// Check if running as root
	runningAsRoot := isRunningAsRoot()
	currentUser := getCurrentUsername()

	// Check Claude CLI auth status
	var claudeAuth *ClaudeAuthInfo
	if checkCommand(getClaudeCmd(), "--version") {
		claudeAuth = checkClaudeAuthStatus()
		// If not logged in, add to missing deps with appropriate hint
		if claudeAuth != nil && !claudeAuth.LoggedIn {
			authDep := getClaudeAuthDep(runningAsRoot, currentUser)
			missingDeps = append(missingDeps, authDep)
		}
	}

	c.JSON(http.StatusOK, InitStatusResponse{
		IsFirstRun:     isFirstRun,
		HasProvider:    hasProvider,
		HasSession:     hasSession,
		MissingDeps:    missingDeps,
		DepsStatus:     depsStatus,
		Platform:       runtime.GOOS,
		PackageManager: pkgMgr,
		ClaudeAuth:     claudeAuth,
		RunningAsRoot:  runningAsRoot,
		CurrentUser:    currentUser,
	})
}

// isRunningAsRoot checks if the current process is running as root/admin
func isRunningAsRoot() bool {
	if runtime.GOOS == "windows" {
		// On Windows, check if running as Administrator
		// Simple heuristic: try to read a protected registry key
		cmd := exec.Command("net", "session")
		err := cmd.Run()
		return err == nil
	}
	// On Unix, check UID
	return os.Getuid() == 0
}

// getCurrentUsername returns the current user's username
func getCurrentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// checkClaudeAuthStatus checks Claude CLI authentication status
func checkClaudeAuthStatus() *ClaudeAuthInfo {
	cmd := exec.Command(getClaudeCmd(), "auth", "status")
	out, err := cmd.Output()
	if err != nil {
		// Try to get stderr for error message
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ClaudeAuthInfo{
				LoggedIn: false,
				Error:    strings.TrimSpace(string(exitErr.Stderr)),
			}
		}
		return &ClaudeAuthInfo{
			LoggedIn: false,
			Error:    err.Error(),
		}
	}

	// Parse JSON output
	var status struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
		Email      string `json:"email"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return &ClaudeAuthInfo{
			LoggedIn: false,
			Error:    "无法解析认证状态",
		}
	}

	return &ClaudeAuthInfo{
		LoggedIn:   status.LoggedIn,
		AuthMethod: status.AuthMethod,
		Email:      status.Email,
	}
}

// getClaudeAuthDep returns a MissingDep for Claude CLI authentication
func getClaudeAuthDep(runningAsRoot bool, currentUser string) MissingDep {
	dep := MissingDep{
		Name:        "Claude CLI 登录",
		Description: "Claude CLI 需要登录才能使用",
		InstallCmd:  "claude login",
		Required:    true,
	}

	if runningAsRoot {
		// Running as root but root hasn't logged in
		dep.Hint = "当前以 root 身份运行，但 root 用户未登录 Claude CLI。请执行: sudo su - 然后 claude login"
		dep.CopyCmd = "sudo su - && claude login"
	} else {
		dep.Hint = "请在终端执行 claude login 完成登录"
	}

	return dep
}

// detectPackageManager detects available package manager
func detectPackageManager() string {
	if pkgMgrChecked {
		return detectedPkgMgr
	}
	pkgMgrChecked = true

	switch runtime.GOOS {
	case "darwin":
		if checkCommand("brew", "--version") {
			detectedPkgMgr = "brew"
		} else {
			detectedPkgMgr = "none"
		}
	case "linux":
		if checkCommand("apt-get", "--version") {
			detectedPkgMgr = "apt"
		} else if checkCommand("yum", "--version") {
			detectedPkgMgr = "yum"
		} else if checkCommand("dnf", "--version") {
			detectedPkgMgr = "dnf"
		} else if checkCommand("pacman", "--version") {
			detectedPkgMgr = "pacman"
		} else {
			detectedPkgMgr = "none"
		}
	case "windows":
		if checkCommand("winget", "--version") {
			detectedPkgMgr = "winget"
		} else if checkCommand("choco", "--version") {
			detectedPkgMgr = "choco"
		} else {
			detectedPkgMgr = "none"
		}
	default:
		detectedPkgMgr = "none"
	}
	return detectedPkgMgr
}

// checkMissingDeps detects missing dependencies with smart install commands
func checkMissingDeps(pkgMgr string) []MissingDep {
	var missing []MissingDep

	// Check Node.js (required for Claude CLI)
	if !checkCommand("node", "--version") {
		dep := getNodeDep(pkgMgr)
		missing = append(missing, dep)
	}

	// Check Claude CLI (required)
	if !checkCommand(getClaudeCmd(), "--version") {
		dep := getClaudeCLIDep(pkgMgr)
		missing = append(missing, dep)
	}

	// Check git (optional but recommended)
	if !checkCommand("git", "--version") {
		dep := getGitDep(pkgMgr)
		missing = append(missing, dep)
	}

	return missing
}

// Node.js dependency
func getNodeDep(pkgMgr string) MissingDep {
	dep := MissingDep{
		Name:        "Node.js",
		Description: "JavaScript 运行时，Claude Code CLI 依赖",
		InstallURL:  "https://nodejs.org/",
		Required:    true,
	}

	switch runtime.GOOS {
	case "darwin":
		if pkgMgr == "brew" {
			dep.InstallCmd = "brew install node"
		} else {
			dep.Hint = "请先安装 Homebrew: /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
			dep.InstallURL = "https://nodejs.org/en/download/"
		}
	case "linux":
		switch pkgMgr {
		case "apt":
			dep.InstallCmd = "curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs"
			dep.NeedsSudo = true
			dep.CopyCmd = "curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && apt-get install -y nodejs"
			dep.Hint = "使用 NodeSource 安装最新 LTS 版本"
		case "yum", "dnf":
			dep.InstallCmd = "curl -fsSL https://rpm.nodesource.com/setup_lts.x | sudo bash - && sudo " + pkgMgr + " install -y nodejs"
			dep.NeedsSudo = true
		case "pacman":
			dep.InstallCmd = "sudo pacman -S nodejs npm"
			dep.NeedsSudo = true
		default:
			dep.InstallURL = "https://nodejs.org/en/download/"
			dep.Hint = "请从官网下载安装包"
		}
	case "windows":
		switch pkgMgr {
		case "winget":
			dep.InstallCmd = "winget install OpenJS.NodeJS.LTS"
		case "choco":
			dep.InstallCmd = "choco install nodejs-lts -y"
		default:
			dep.InstallURL = "https://nodejs.org/en/download/"
			dep.Hint = "请下载 Windows 安装包 (.msi)"
		}
	}
	return dep
}

// Claude CLI dependency
func getClaudeCLIDep(pkgMgr string) MissingDep {
	dep := MissingDep{
		Name:        "Claude Code CLI",
		Description: "Anthropic 官方 CLI 工具",
		InstallCmd:  "npm install -g @anthropic-ai/claude-code",
		Required:    true,
	}

	// Check npm global permission issue on Linux/macOS
	if runtime.GOOS != "windows" {
		if checkNpmPermissionIssue() {
			dep.Hint = "如遇权限问题，可尝试: sudo npm install -g @anthropic-ai/claude-code"
			dep.CopyCmd = "npm install -g @anthropic-ai/claude-code"
		}
	}

	// Use npmmirror for China
	if isInChina() {
		dep.InstallCmd = "npm install -g @anthropic-ai/claude-code --registry=https://registry.npmmirror.com"
		dep.Hint = "使用国内镜像加速下载"
	}

	return dep
}

// Git dependency
func getGitDep(pkgMgr string) MissingDep {
	dep := MissingDep{
		Name:        "Git",
		Description: "版本控制工具",
		InstallURL:  "https://git-scm.com/downloads",
		Required:    false,
	}

	switch runtime.GOOS {
	case "darwin":
		if pkgMgr == "brew" {
			dep.InstallCmd = "brew install git"
		} else {
			dep.Hint = "macOS 可通过 Xcode Command Line Tools 安装: xcode-select --install"
		}
	case "linux":
		switch pkgMgr {
		case "apt":
			dep.InstallCmd = "sudo apt-get install -y git"
			dep.NeedsSudo = true
			dep.CopyCmd = "apt-get install -y git"
		case "yum":
			dep.InstallCmd = "sudo yum install -y git"
			dep.NeedsSudo = true
		case "dnf":
			dep.InstallCmd = "sudo dnf install -y git"
			dep.NeedsSudo = true
		case "pacman":
			dep.InstallCmd = "sudo pacman -S git"
			dep.NeedsSudo = true
		}
	case "windows":
		switch pkgMgr {
		case "winget":
			dep.InstallCmd = "winget install Git.Git"
		case "choco":
			dep.InstallCmd = "choco install git -y"
		}
	}
	return dep
}

// checkCommand checks if a command is available
func checkCommand(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	err := cmd.Run()
	return err == nil
}

// getClaudeCmd returns the correct claude command for the current OS
// Windows npm global installs create .cmd files, not bare executables
func getClaudeCmd() string {
	if runtime.GOOS == "windows" {
		return "claude.cmd"
	}
	return "claude"
}

// checkNpmPermissionIssue checks if npm global install might have permission issues
func checkNpmPermissionIssue() bool {
	// Check if npm global prefix is in a system directory
	cmd := exec.Command("npm", "config", "get", "prefix")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	prefix := strings.TrimSpace(string(out))
	// System directories that typically need sudo
	systemDirs := []string{"/usr/local", "/usr/lib", "/opt"}
	for _, dir := range systemDirs {
		if strings.HasPrefix(prefix, dir) {
			return true
		}
	}
	return false
}

// isInChina tries to detect if user is in China (simple heuristic)
func isInChina() bool {
	// Check common China-specific environment indicators
	// This is a simple heuristic, not 100% accurate
	cmd := exec.Command("curl", "-s", "-m", "2", "https://registry.npmmirror.com", "-o", "/dev/null", "-w", "%{http_code}")
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) == "200" {
		return true
	}
	return false
}

// InstallDepRequest for POST /api/v1/system/install-dep
type InstallDepRequest struct {
	Name       string `json:"name"`
	InstallCmd string `json:"install_cmd"`
}

type InstallDepResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
	Hint    string `json:"hint,omitempty"` // Suggestion for next steps
}

func InstallDep(c *gin.Context) {
	var req InstallDepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.InstallCmd == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "install_cmd is required"})
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/C", req.InstallCmd)
	default:
		cmd = exec.Command("sh", "-c", req.InstallCmd)
	}

	output, err := cmd.CombinedOutput()

	resp := InstallDepResponse{
		Success: err == nil,
		Output:  string(output),
	}

	if err != nil {
		resp.Error = err.Error()
		// Provide helpful hints based on error
		outStr := string(output)
		if strings.Contains(outStr, "permission denied") || strings.Contains(outStr, "EACCES") {
			resp.Hint = "权限不足，请尝试使用 sudo 运行命令"
		} else if strings.Contains(outStr, "not found") || strings.Contains(outStr, "command not found") {
			resp.Hint = "命令未找到，请检查是否已安装相关工具"
		} else if strings.Contains(outStr, "network") || strings.Contains(outStr, "timeout") {
			resp.Hint = "网络错误，请检查网络连接或尝试使用代理"
		}
	}

	c.JSON(http.StatusOK, resp)
}

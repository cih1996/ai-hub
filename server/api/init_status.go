package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/gin-gonic/gin"
)

// InitStatusResponse represents the system initialization status
type InitStatusResponse struct {
	IsFirstRun  bool            `json:"is_first_run"`
	HasProvider bool            `json:"has_provider"`
	HasSession  bool            `json:"has_session"`
	MissingDeps []MissingDep    `json:"missing_deps"`
	DepsStatus  core.DepsStatus `json:"deps_status"`
	Platform    string          `json:"platform"` // darwin, linux, windows
}

// MissingDep represents a missing dependency
type MissingDep struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InstallCmd  string `json:"install_cmd,omitempty"`
	InstallURL  string `json:"install_url,omitempty"` // For manual download
	Required    bool   `json:"required"`
}

// GetInitStatus returns the system initialization status
// GET /api/v1/system/init-status
// Query params:
//   - force_first_run=true: force return is_first_run=true for testing
func GetInitStatus(c *gin.Context) {
	forceFirstRun := c.Query("force_first_run") == "true"

	// Check providers
	providers, _ := store.ListProviders()
	hasProvider := len(providers) > 0

	// Check sessions
	sessions, _ := store.ListSessions()
	hasSession := len(sessions) > 0

	// Determine if first run
	isFirstRun := !hasProvider && !hasSession
	if forceFirstRun {
		isFirstRun = true
	}

	// Check dependencies
	missingDeps := checkMissingDeps()

	// Get deps status from core
	depsStatus := core.Deps.GetStatus()

	c.JSON(http.StatusOK, InitStatusResponse{
		IsFirstRun:  isFirstRun,
		HasProvider: hasProvider,
		HasSession:  hasSession,
		MissingDeps: missingDeps,
		DepsStatus:  depsStatus,
		Platform:    runtime.GOOS,
	})
}

// checkMissingDeps detects missing dependencies
func checkMissingDeps() []MissingDep {
	var missing []MissingDep

	// Check Node.js
	if !checkCommand("node", "--version") {
		missing = append(missing, MissingDep{
			Name:        "Node.js",
			Description: "JavaScript 运行时，Claude Code CLI 依赖",
			InstallCmd:  getNodeInstallCmd(),
			InstallURL:  "https://nodejs.org/",
			Required:    true,
		})
	}

	// Check Python
	pythonOK := checkCommand("python3", "--version") || checkCommand("python", "--version")
	if !pythonOK {
		missing = append(missing, MissingDep{
			Name:        "Python",
			Description: "向量引擎依赖，用于语义搜索",
			InstallCmd:  getPythonInstallCmd(),
			InstallURL:  "https://www.python.org/downloads/",
			Required:    false,
		})
	}

	// Check pip
	pipOK := checkCommand("pip3", "--version") || checkCommand("pip", "--version")
	if pythonOK && !pipOK {
		missing = append(missing, MissingDep{
			Name:        "pip",
			Description: "Python 包管理器",
			InstallCmd:  getPipInstallCmd(),
			Required:    false,
		})
	}

	// Check vector engine status
	if core.Vector == nil || !core.Vector.IsReady() {
		// Check if sentence-transformers is installed
		if !checkPythonPackage("sentence_transformers") {
			missing = append(missing, MissingDep{
				Name:        "sentence-transformers",
				Description: "向量引擎核心库，用于文本嵌入",
				InstallCmd:  getSentenceTransformersCmd(),
				Required:    false,
			})
		}
	}

	// Check Claude CLI
	if !checkCommand("claude", "--version") {
		missing = append(missing, MissingDep{
			Name:        "Claude Code CLI",
			Description: "Anthropic 官方 CLI 工具",
			InstallCmd:  "npm install -g @anthropic-ai/claude-code",
			Required:    true,
		})
	}

	// Check git
	if !checkCommand("git", "--version") {
		missing = append(missing, MissingDep{
			Name:        "Git",
			Description: "版本控制工具",
			InstallCmd:  getGitInstallCmd(),
			InstallURL:  "https://git-scm.com/downloads",
			Required:    false,
		})
	}

	return missing
}

// Platform-specific install commands

func getNodeInstallCmd() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install node"
	case "linux":
		// Use NodeSource for latest LTS
		return "curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs"
	case "windows":
		// winget is available on Windows 10 1709+ and Windows 11
		return "winget install OpenJS.NodeJS.LTS"
	default:
		return ""
	}
}

func getPythonInstallCmd() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install python3"
	case "linux":
		return "sudo apt-get install -y python3 python3-pip"
	case "windows":
		return "winget install Python.Python.3.12"
	default:
		return ""
	}
}

func getPipInstallCmd() string {
	switch runtime.GOOS {
	case "windows":
		return "python -m ensurepip --upgrade"
	default:
		return "python3 -m ensurepip --upgrade"
	}
}

func getSentenceTransformersCmd() string {
	// Use Tsinghua mirror for faster download in China
	switch runtime.GOOS {
	case "windows":
		return "pip install sentence-transformers -i https://pypi.tuna.tsinghua.edu.cn/simple"
	default:
		return "pip3 install sentence-transformers -i https://pypi.tuna.tsinghua.edu.cn/simple"
	}
}

func getGitInstallCmd() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install git"
	case "linux":
		return "sudo apt-get install -y git"
	case "windows":
		return "winget install Git.Git"
	default:
		return ""
	}
}

// checkCommand checks if a command is available
func checkCommand(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	err := cmd.Run()
	return err == nil
}

// checkPythonPackage checks if a Python package is installed
func checkPythonPackage(pkg string) bool {
	// Try python3 first, then python
	for _, py := range []string{"python3", "python"} {
		cmd := exec.Command(py, "-c", "import "+pkg)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

// InstallDep handles dependency installation request
// POST /api/v1/system/install-dep
type InstallDepRequest struct {
	Name       string `json:"name"`
	InstallCmd string `json:"install_cmd"`
}

type InstallDepResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
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

	// Execute command via shell to support pipes and complex commands
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
	}

	c.JSON(http.StatusOK, resp)
}

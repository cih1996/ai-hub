package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

// InitStatusResponse represents the system initialization status
type InitStatusResponse struct {
	IsFirstRun  bool           `json:"is_first_run"`
	HasProvider bool           `json:"has_provider"`
	HasSession  bool           `json:"has_session"`
	MissingDeps []MissingDep   `json:"missing_deps"`
	DepsStatus  core.DepsStatus `json:"deps_status"`
}

// MissingDep represents a missing dependency
type MissingDep struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InstallCmd  string `json:"install_cmd,omitempty"`
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
			InstallCmd:  "brew install node",
			Required:    true,
		})
	}

	// Check Python
	pythonOK := checkCommand("python3", "--version") || checkCommand("python", "--version")
	if !pythonOK {
		missing = append(missing, MissingDep{
			Name:        "Python",
			Description: "向量引擎依赖，用于语义搜索",
			InstallCmd:  "brew install python3",
			Required:    false,
		})
	}

	// Check pip
	pipOK := checkCommand("pip3", "--version") || checkCommand("pip", "--version")
	if pythonOK && !pipOK {
		missing = append(missing, MissingDep{
			Name:        "pip",
			Description: "Python 包管理器",
			InstallCmd:  "python3 -m ensurepip --upgrade",
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
				InstallCmd:  "pip3 install sentence-transformers -i https://pypi.tuna.tsinghua.edu.cn/simple",
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
			InstallCmd:  "brew install git",
			Required:    false,
		})
	}

	return missing
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

	// Execute install command
	// Split command for shell execution
	parts := strings.Fields(req.InstallCmd)
	if len(parts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid install_cmd"})
		return
	}

	cmd := exec.Command(parts[0], parts[1:]...)
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

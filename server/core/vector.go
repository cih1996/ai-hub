package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// venvBinPath returns the correct path for a binary inside a Python venv.
// Windows uses venv/Scripts/<name>.exe, Unix uses venv/bin/<name>.
func venvBinPath(venvDir, name string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", name+".exe")
	}
	return filepath.Join(venvDir, "bin", name)
}

// VectorEngine manages the Python vector engine subprocess
type VectorEngine struct {
	mu       sync.RWMutex
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	port     int
	baseURL  string
	ready    bool
	disabled bool
	err      string

	// paths
	baseDir    string // ~/.ai-hub/vector-engine
	venvDir    string
	pythonPath string // venv python
	scriptDir  string // ai-hub/vector-engine/
}

var Vector *VectorEngine

// InitVectorEngine initializes and starts the vector engine in background.
// Never blocks main startup — failures degrade gracefully.
func InitVectorEngine(scriptDir string) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".ai-hub", "vector-engine")
	port := 8090

	Vector = &VectorEngine{
		port:      port,
		baseURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
		baseDir:   baseDir,
		venvDir:   filepath.Join(baseDir, "venv"),
		scriptDir: scriptDir,
	}

	go Vector.bootstrap()
}

func (v *VectorEngine) bootstrap() {
	log.Println("[vector] starting bootstrap...")

	// Step 1: check python3 >= 3.10
	pyPath, err := v.detectPython()
	if err != nil {
		v.setDisabled("python not found: " + err.Error())
		return
	}
	log.Printf("[vector] python: %s", pyPath)

	// Step 2: create/check venv
	if err := v.ensureVenv(pyPath); err != nil {
		v.setDisabled("venv setup failed: " + err.Error())
		return
	}
	log.Printf("[vector] venv ready: %s", v.venvDir)

	// Step 3: install pip dependencies
	if err := v.installDeps(); err != nil {
		v.setDisabled("pip install failed: " + err.Error())
		return
	}
	log.Println("[vector] pip dependencies ready")

	// Step 4: download embedding model (may take minutes on first run)
	if err := v.downloadModel(); err != nil {
		v.setDisabled("model download failed: " + err.Error())
		return
	}
	log.Println("[vector] embedding model ready")

	// Step 5: start FastAPI service
	if err := v.startProcess(); err != nil {
		if err.Error() == "reuse_existing" {
			// Healthy process already running on the port — reuse it
			v.mu.Lock()
			v.ready = true
			v.mu.Unlock()
			log.Println("[vector] engine ready (reused existing process)")
			return
		}
		v.setDisabled("process start failed: " + err.Error())
		return
	}

	// Step 6: wait for health check (model already downloaded, only wait for server startup)
	if err := v.waitHealthy(30 * time.Second); err != nil {
		v.setDisabled("health check failed: " + err.Error())
		v.Stop()
		return
	}

	v.mu.Lock()
	v.ready = true
	v.mu.Unlock()
	log.Println("[vector] engine ready")
}

func (v *VectorEngine) detectPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		out, err := runCmd(name, "--version")
		if err != nil {
			continue
		}
		// parse "Python 3.x.y"
		ver := strings.TrimSpace(out)
		ver = strings.TrimPrefix(ver, "Python ")
		parts := strings.Split(ver, ".")
		if len(parts) < 2 {
			continue
		}
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		if major == 3 && minor >= 10 {
			path, _ := exec.LookPath(name)
			return path, nil
		}
	}
	return "", fmt.Errorf("python >= 3.10 required")
}

func (v *VectorEngine) ensureVenv(pyPath string) error {
	venvPython := venvBinPath(v.venvDir, "python")
	if _, err := os.Stat(venvPython); err == nil {
		v.pythonPath = venvPython
		return nil
	}
	os.MkdirAll(v.baseDir, 0755)
	cmd := exec.Command(pyPath, "-m", "venv", v.venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	v.pythonPath = venvPython
	return nil
}

func (v *VectorEngine) installDeps() error {
	reqFile := filepath.Join(v.scriptDir, "requirements.txt")
	pip := venvBinPath(v.venvDir, "pip")
	cmd := exec.Command(pip, "install", "-q", "-r", reqFile)
	cmd.Env = append(os.Environ(), "VIRTUAL_ENV="+v.venvDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (v *VectorEngine) downloadModel() error {
	script := filepath.Join(v.scriptDir, "download_model.py")
	cmd := exec.Command(v.pythonPath, script)
	cmd.Env = append(os.Environ(),
		"VIRTUAL_ENV="+v.venvDir,
		"EMBEDDING_MODEL_PATH="+filepath.Join(v.baseDir, "models"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download_model.py failed: %s", err)
	}
	return nil
}

// handlePortConflict checks if the vector engine port is already occupied.
// If a healthy process is found, it sets ready=true and returns a sentinel error to skip startup.
// If an unhealthy process is found, it kills it before returning nil.
func (v *VectorEngine) handlePortConflict() error {
	// Quick health check on the port
	url := v.baseURL + "/health"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// Port not responding — might still be occupied by a dead process
		v.killPortProcess()
		return nil
	}
	resp.Body.Close()

	if resp.StatusCode == 200 {
		// Healthy process already running — reuse it
		log.Printf("[vector] port %d already has a healthy process, reusing", v.port)
		return fmt.Errorf("reuse_existing")
	}

	// Port responds but not healthy — kill and restart
	log.Printf("[vector] port %d responds but unhealthy (status %d), killing", v.port, resp.StatusCode)
	v.killPortProcess()
	time.Sleep(1 * time.Second)
	return nil
}

// killPortProcess finds and kills any process occupying the vector engine port.
func (v *VectorEngine) killPortProcess() {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", v.port)).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return
	}
	pids := strings.Fields(strings.TrimSpace(string(out)))
	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			continue
		}
		log.Printf("[vector] killing residual process pid=%d on port %d", pid, v.port)
		killProcessGroup(pid, true)
	}
	time.Sleep(500 * time.Millisecond)
}

func (v *VectorEngine) startProcess() error {
	// Check if port is already occupied by a residual process
	if err := v.handlePortConflict(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	mainPy := filepath.Join(v.scriptDir, "main.py")
	cmd := exec.CommandContext(ctx, v.pythonPath, mainPy)
	cmd.Env = append(os.Environ(),
		"VIRTUAL_ENV="+v.venvDir,
		fmt.Sprintf("VECTOR_ENGINE_PORT=%d", v.port),
		"VECTOR_DB_PATH="+filepath.Join(v.baseDir, "data"),
		"EMBEDDING_MODEL_PATH="+filepath.Join(v.baseDir, "models"),
	)
	cmd.SysProcAttr = newProcessGroupAttr()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	v.mu.Lock()
	v.cmd = cmd
	v.cancel = cancel
	v.mu.Unlock()

	// monitor process exit
	go func() {
		err := cmd.Wait()
		v.mu.Lock()
		v.ready = false
		if err != nil {
			v.err = "process exited: " + err.Error()
		}
		v.mu.Unlock()
		log.Printf("[vector] process exited: %v", err)
	}()

	return nil
}

func (v *VectorEngine) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := v.baseURL + "/health"
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

// Stop shuts down the vector engine subprocess gracefully
func (v *VectorEngine) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ready = false

	if v.cmd != nil && v.cmd.Process != nil {
		pid := v.cmd.Process.Pid
		// Send SIGTERM to process group (Unix) or kill process (Windows)
		killProcessGroup(pid, false)
		log.Printf("[vector] sent terminate signal to pid %d", pid)

		// Wait up to 5 seconds for graceful exit
		done := make(chan struct{})
		go func() {
			v.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
			log.Println("[vector] process exited gracefully")
		case <-time.After(5 * time.Second):
			// Force kill
			killProcessGroup(pid, true)
			log.Printf("[vector] force killed pid %d", pid)
			<-done
		}
	}

	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	log.Println("[vector] stopped")
}

// Restart stops the current engine (if any) and re-bootstraps from scratch.
// Safe to call even when the engine is disabled or not ready.
func (v *VectorEngine) Restart() {
	log.Println("[vector] restart requested")
	v.Stop()

	// Reset state
	v.mu.Lock()
	v.disabled = false
	v.err = ""
	v.cmd = nil
	v.cancel = nil
	v.mu.Unlock()

	v.bootstrap()
}

// IsReady returns whether the vector engine is available
func (v *VectorEngine) IsReady() bool {
	if v == nil {
		return false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.ready
}

// Status returns current engine status for API
func (v *VectorEngine) Status() map[string]interface{} {
	if v == nil {
		return map[string]interface{}{"ready": false, "disabled": true, "error": "not initialized"}
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	return map[string]interface{}{
		"ready":    v.ready,
		"disabled": v.disabled,
		"error":    v.err,
		"port":     v.port,
	}
}

func (v *VectorEngine) setDisabled(reason string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.disabled = true
	v.err = reason
	log.Printf("[vector] disabled: %s", reason)
}

// Embed sends text to the vector engine for embedding
func (v *VectorEngine) Embed(scope, docID, text string, metadata map[string]interface{}) error {
	if !v.IsReady() {
		return fmt.Errorf("vector engine not ready")
	}
	body := map[string]interface{}{
		"scope":  scope,
		"doc_id": docID,
		"text":   text,
	}
	if metadata != nil {
		body["metadata"] = metadata
	}
	_, err := v.post("/embed", body)
	return err
}

// Search performs semantic search with automatic retry on transient errors.
func (v *VectorEngine) Search(scope, query string, topK int) ([]map[string]interface{}, error) {
	if !v.IsReady() {
		return nil, fmt.Errorf("vector engine not ready")
	}
	body := map[string]interface{}{
		"scope": scope,
		"query": query,
		"top_k": topK,
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			log.Printf("[vector] search retry %d/3 for scope=%s: %v", attempt+1, scope, lastErr)
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			// If engine became not-ready (process crashed), try restart
			if !v.IsReady() {
				log.Println("[vector] engine not ready, attempting restart")
				go v.Restart()
				return nil, fmt.Errorf("vector engine restarting after failure: %w", lastErr)
			}
		}
		resp, err := v.post("/search", body)
		if err != nil {
			lastErr = err
			continue
		}
		results, _ := resp["results"].([]interface{})
		items := make([]map[string]interface{}, 0, len(results))
		for _, r := range results {
			if m, ok := r.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
		return items, nil
	}
	return nil, fmt.Errorf("search failed after 3 attempts: %w", lastErr)
}

// Delete removes a vector record
func (v *VectorEngine) Delete(scope, docID string) error {
	if !v.IsReady() {
		return fmt.Errorf("vector engine not ready")
	}
	body := map[string]interface{}{
		"scope":  scope,
		"doc_id": docID,
	}
	_, err := v.post("/delete", body)
	return err
}

// Stats returns hit statistics
func (v *VectorEngine) Stats(scope string) (map[string]interface{}, error) {
	if !v.IsReady() {
		return nil, fmt.Errorf("vector engine not ready")
	}
	resp, err := http.Get(fmt.Sprintf("%s/stats?scope=%s", v.baseURL, scope))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (v *VectorEngine) post(path string, body map[string]interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	resp, err := http.Post(v.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vector engine %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

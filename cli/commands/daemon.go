package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RunDaemon handles daemon management commands: start, stop, restart, install, uninstall
func RunDaemon(c *client.Client, args []string) int {
	if len(args) == 0 {
		printDaemonHelp()
		return 0
	}

	cmd := args[0]
	switch cmd {
	case "start":
		return daemonStart(c)
	case "stop":
		return daemonStop(c)
	case "restart":
		return daemonRestart(c)
	case "install":
		return daemonInstall("")
	case "uninstall":
		return daemonUninstall()
	case "status":
		return daemonStatus(c)
	case "--help", "-h":
		printDaemonHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown daemon command: %s\n", cmd)
		printDaemonHelp()
		return 1
	}
}

func printDaemonHelp() {
	fmt.Println(`AI Hub Daemon Management

Usage:
  ai-hub daemon <command>

Commands:
  start      Start AI Hub service
  stop       Stop AI Hub service (graceful shutdown)
  restart    Restart AI Hub service
  install    Install AI Hub as system service
  uninstall  Uninstall AI Hub system service
  status     Show service status

Examples:
  ai-hub daemon start
  ai-hub daemon stop
  ai-hub daemon restart`)
}

// daemonStart starts the service via platform-specific method
func daemonStart(c *client.Client) int {
	// Check if already running
	if _, err := c.GET("/api/v1/version"); err == nil {
		fmt.Println("AI Hub is already running")
		return 0
	}

	switch runtime.GOOS {
	case "darwin":
		return startLaunchd()
	case "linux":
		return startSystemd()
	case "windows":
		return startWindows()
	default:
		fmt.Fprintf(os.Stderr, "Unsupported platform: %s\n", runtime.GOOS)
		return 1
	}
}

// daemonStop gracefully stops the service
func daemonStop(c *client.Client) int {
	// Try graceful shutdown via API first
	resp, err := c.POST("/api/v1/shutdown", nil)
	if err == nil && resp != nil {
		fmt.Println("AI Hub is shutting down...")
		// Wait for shutdown
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			if _, err := c.GET("/api/v1/version"); err != nil {
				fmt.Println("AI Hub stopped")
				return 0
			}
		}
		fmt.Println("AI Hub stopped (timeout waiting for confirmation)")
		return 0
	}

	// Fallback to platform-specific stop
	switch runtime.GOOS {
	case "darwin":
		return stopLaunchd()
	case "linux":
		return stopSystemd()
	case "windows":
		return stopWindows()
	default:
		fmt.Println("AI Hub is not running")
		return 0
	}
}

// daemonRestart restarts the service
func daemonRestart(c *client.Client) int {
	daemonStop(c)
	time.Sleep(1 * time.Second)
	return daemonStart(c)
}

// daemonStatus shows service status
func daemonStatus(c *client.Client) int {
	resp, err := c.GET("/api/v1/version")
	if err != nil {
		fmt.Println("Status: stopped")
		return 0
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err == nil {
		if version, ok := result["version"].(string); ok {
			fmt.Printf("Status: running (version %s)\n", version)
			return 0
		}
	}
	fmt.Println("Status: running")
	return 0
}

// ============ Installation ============

// daemonInstall installs AI Hub as a system service
func daemonInstall(binaryPath string) int {
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
			return 1
		}
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(binaryPath)
	case "linux":
		return installSystemd(binaryPath)
	case "windows":
		return installWindows(binaryPath)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported platform: %s\n", runtime.GOOS)
		return 1
	}
}

// daemonUninstall removes AI Hub system service
func daemonUninstall() int {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	case "windows":
		return uninstallWindows()
	default:
		fmt.Fprintf(os.Stderr, "Unsupported platform: %s\n", runtime.GOOS)
		return 1
	}
}

// ============ macOS (launchd) ============

const launchdLabel = "com.ai-hub.server"

func getLaunchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func getInstallPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/usr/local/bin/ai-hub"
	case "linux":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "bin", "ai-hub")
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "ai-hub", "ai-hub.exe")
	default:
		return ""
	}
}

func installLaunchd(binaryPath string) int {
	installPath := getInstallPath()
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ai-hub")
	logPath := filepath.Join(dataDir, "logs", "ai-hub.log")

	// Ensure directories exist
	os.MkdirAll(filepath.Dir(installPath), 0755)
	os.MkdirAll(filepath.Dir(logPath), 0755)
	os.MkdirAll(filepath.Join(home, "Library", "LaunchAgents"), 0755)

	// Copy binary to install path
	if binaryPath != installPath {
		if err := copyFile(binaryPath, installPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to copy binary: %v\n", err)
			return 1
		}
		os.Chmod(installPath, 0755)
		fmt.Printf("Installed binary to %s\n", installPath)
	}

	// Create plist
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>-port</string>
        <string>8080</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>`, launchdLabel, installPath, logPath, logPath, dataDir)

	plistPath := getLaunchdPlistPath()
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write plist: %v\n", err)
		return 1
	}
	fmt.Printf("Created service config: %s\n", plistPath)

	// Load the service
	exec.Command("launchctl", "unload", plistPath).Run() // Ignore error if not loaded
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load service: %v\n", err)
		return 1
	}

	fmt.Println("AI Hub service installed and started")
	fmt.Println("Service will auto-start on login")
	return 0
}

func uninstallLaunchd() int {
	plistPath := getLaunchdPlistPath()

	// Unload service
	exec.Command("launchctl", "unload", plistPath).Run()

	// Remove plist
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Failed to remove plist: %v\n", err)
	}

	// Remove binary
	installPath := getInstallPath()
	if err := os.Remove(installPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Failed to remove binary: %v\n", err)
	}

	fmt.Println("AI Hub service uninstalled")
	return 0
}

func startLaunchd() int {
	plistPath := getLaunchdPlistPath()
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Service not installed. Run 'ai-hub daemon install' first.\n")
		return 1
	}

	if err := exec.Command("launchctl", "start", launchdLabel).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start service: %v\n", err)
		return 1
	}

	fmt.Println("AI Hub service started")
	return 0
}

func stopLaunchd() int {
	if err := exec.Command("launchctl", "stop", launchdLabel).Run(); err != nil {
		// Service might not be running
		return 0
	}
	fmt.Println("AI Hub service stopped")
	return 0
}

// ============ Linux (systemd) ============

const systemdServiceName = "ai-hub"

func getSystemdServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", systemdServiceName+".service")
}

func installSystemd(binaryPath string) int {
	installPath := getInstallPath()
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ai-hub")

	// Ensure directories exist
	os.MkdirAll(filepath.Dir(installPath), 0755)
	os.MkdirAll(filepath.Join(home, ".config", "systemd", "user"), 0755)

	// Copy binary
	if binaryPath != installPath {
		if err := copyFile(binaryPath, installPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to copy binary: %v\n", err)
			return 1
		}
		os.Chmod(installPath, 0755)
		fmt.Printf("Installed binary to %s\n", installPath)
	}

	// Create systemd service file
	service := fmt.Sprintf(`[Unit]
Description=AI Hub Server
After=network.target

[Service]
Type=simple
ExecStart=%s -port 8080
WorkingDirectory=%s
Restart=always
RestartSec=5
Environment=PATH=/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=default.target
`, installPath, dataDir)

	servicePath := getSystemdServicePath()
	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write service file: %v\n", err)
		return 1
	}
	fmt.Printf("Created service config: %s\n", servicePath)

	// Reload and enable
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	exec.Command("systemctl", "--user", "enable", systemdServiceName).Run()
	exec.Command("systemctl", "--user", "start", systemdServiceName).Run()

	// Enable lingering for user services to run without login
	user := os.Getenv("USER")
	if user != "" {
		exec.Command("loginctl", "enable-linger", user).Run()
	}

	fmt.Println("AI Hub service installed and started")
	fmt.Println("Service will auto-start on boot (user linger enabled)")
	return 0
}

func uninstallSystemd() int {
	servicePath := getSystemdServicePath()

	// Stop and disable
	exec.Command("systemctl", "--user", "stop", systemdServiceName).Run()
	exec.Command("systemctl", "--user", "disable", systemdServiceName).Run()

	// Remove service file
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Failed to remove service file: %v\n", err)
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run()

	// Remove binary
	installPath := getInstallPath()
	if err := os.Remove(installPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Failed to remove binary: %v\n", err)
	}

	fmt.Println("AI Hub service uninstalled")
	return 0
}

func startSystemd() int {
	servicePath := getSystemdServicePath()
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Service not installed. Run 'ai-hub daemon install' first.\n")
		return 1
	}

	if err := exec.Command("systemctl", "--user", "start", systemdServiceName).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start service: %v\n", err)
		return 1
	}

	fmt.Println("AI Hub service started")
	return 0
}

func stopSystemd() int {
	exec.Command("systemctl", "--user", "stop", systemdServiceName).Run()
	fmt.Println("AI Hub service stopped")
	return 0
}

// ============ Windows (Task Scheduler) ============

const windowsTaskName = "AIHub"

func installWindows(binaryPath string) int {
	installPath := getInstallPath()
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ai-hub")

	// Ensure directories exist
	os.MkdirAll(filepath.Dir(installPath), 0755)
	os.MkdirAll(dataDir, 0755)

	// Copy binary
	if binaryPath != installPath {
		if err := copyFile(binaryPath, installPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to copy binary: %v\n", err)
			return 1
		}
		fmt.Printf("Installed binary to %s\n", installPath)
	}

	// Delete existing task if any
	exec.Command("schtasks", "/Delete", "/TN", windowsTaskName, "/F").Run()

	// Create scheduled task that runs at logon
	// Using XML for more control
	xmlPath := filepath.Join(dataDir, "task.xml")
	taskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>-port 8080</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>`, strings.ReplaceAll(installPath, `\`, `\\`), strings.ReplaceAll(dataDir, `\`, `\\`))

	if err := os.WriteFile(xmlPath, []byte(taskXML), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write task XML: %v\n", err)
		return 1
	}

	// Create task from XML
	cmd := exec.Command("schtasks", "/Create", "/TN", windowsTaskName, "/XML", xmlPath, "/F")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create scheduled task: %v\n%s\n", err, output)
		return 1
	}

	// Start the task immediately
	exec.Command("schtasks", "/Run", "/TN", windowsTaskName).Run()

	fmt.Println("AI Hub service installed and started")
	fmt.Println("Service will auto-start on login")
	return 0
}

func uninstallWindows() int {
	// Stop the task
	exec.Command("schtasks", "/End", "/TN", windowsTaskName).Run()

	// Delete the task
	exec.Command("schtasks", "/Delete", "/TN", windowsTaskName, "/F").Run()

	// Remove binary
	installPath := getInstallPath()
	if err := os.Remove(installPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Failed to remove binary: %v\n", err)
	}

	fmt.Println("AI Hub service uninstalled")
	return 0
}

func startWindows() int {
	if err := exec.Command("schtasks", "/Run", "/TN", windowsTaskName).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start service. Run 'ai-hub daemon install' first.\n")
		return 1
	}
	fmt.Println("AI Hub service started")
	return 0
}

func stopWindows() int {
	exec.Command("schtasks", "/End", "/TN", windowsTaskName).Run()
	fmt.Println("AI Hub service stopped")
	return 0
}

// ============ Utilities ============

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0755)
}

package core

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ServiceMgr is the global service manager instance.
var ServiceMgr *ServiceManager

// ServiceStatusCallback is called when a service status changes.
type ServiceStatusCallback func(svc *model.Service)

// ServiceManager manages hosted service processes and health checks.
type ServiceManager struct {
	mu       sync.RWMutex
	stopCh   chan struct{}
	callback ServiceStatusCallback
}

// InitServiceManager creates and starts the global service manager.
func InitServiceManager(cb ServiceStatusCallback) {
	ServiceMgr = &ServiceManager{
		stopCh:   make(chan struct{}),
		callback: cb,
	}
	go ServiceMgr.healthLoop()
}

// StopServiceManager stops the health check loop.
func StopServiceManager() {
	if ServiceMgr != nil {
		close(ServiceMgr.stopCh)
	}
}

// Start launches a service process.
func (m *ServiceManager) Start(svc *model.Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already running?
	if svc.PID > 0 && processAlive(svc.PID) {
		return fmt.Errorf("service %q already running (PID %d)", svc.Name, svc.PID)
	}

	// Ensure log directory exists
	os.MkdirAll(logDir(), 0755)

	logFile, err := os.OpenFile(svc.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command("sh", "-c", svc.Command)
	if svc.WorkDir != "" {
		cmd.Dir = svc.WorkDir
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Detach process group so it survives parent exit
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start command: %w", err)
	}

	// Don't wait — let it run in background
	go func() {
		cmd.Wait()
		logFile.Close()
	}()

	svc.PID = cmd.Process.Pid
	svc.Status = "running"
	store.UpdateServiceStatus(svc.ID, svc.Status, svc.PID)
	log.Printf("[service] started %q PID=%d", svc.Name, svc.PID)

	if m.callback != nil {
		m.callback(svc)
	}
	return nil
}

// Stop kills a service process.
func (m *ServiceManager) Stop(svc *model.Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if svc.PID > 0 && processAlive(svc.PID) {
		// Kill process group
		syscall.Kill(-svc.PID, syscall.SIGTERM)
		// Wait briefly then force kill
		time.Sleep(2 * time.Second)
		if processAlive(svc.PID) {
			syscall.Kill(-svc.PID, syscall.SIGKILL)
		}
	}

	svc.Status = "stopped"
	svc.PID = 0
	store.UpdateServiceStatus(svc.ID, svc.Status, svc.PID)
	log.Printf("[service] stopped %q", svc.Name)

	if m.callback != nil {
		m.callback(svc)
	}
	return nil
}

// Restart stops then starts a service.
func (m *ServiceManager) Restart(svc *model.Service) error {
	// Stop without lock (Stop acquires its own lock)
	m.Stop(svc)
	time.Sleep(1 * time.Second)
	return m.Start(svc)
}

// CheckAlive returns the current status of a service.
func (m *ServiceManager) CheckAlive(svc *model.Service) string {
	if svc.PID <= 0 {
		return "stopped"
	}
	if processAlive(svc.PID) {
		// If port configured, also check port
		if svc.Port > 0 && !portReachable(svc.Port) {
			return "running" // process alive but port not ready yet
		}
		return "running"
	}
	return "dead"
}

// healthLoop periodically checks all services.
func (m *ServiceManager) healthLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAll()
		}
	}
}

func (m *ServiceManager) checkAll() {
	services, err := store.ListServices()
	if err != nil {
		return
	}
	for i := range services {
		svc := &services[i]
		newStatus := m.CheckAlive(svc)
		if newStatus != svc.Status {
			oldStatus := svc.Status
			svc.Status = newStatus
			if newStatus == "dead" {
				svc.PID = 0
			}
			store.UpdateServiceStatus(svc.ID, svc.Status, svc.PID)
			log.Printf("[service] %q status changed: %s -> %s", svc.Name, oldStatus, newStatus)
			if m.callback != nil {
				m.callback(svc)
			}
		}
	}
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func portReachable(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func logDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.ai-hub/logs"
}

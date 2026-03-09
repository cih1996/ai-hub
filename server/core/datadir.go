package core

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	dataDir     string
	dataDirOnce sync.Once
)

// InitDataDir initializes the global data directory.
// Priority: explicit value > AI_HUB_DATA_DIR env > ~/.ai-hub
// Must be called once at startup before any GetDataDir() calls.
func InitDataDir(dir string) {
	dataDirOnce.Do(func() {
		if dir != "" {
			dataDir = dir
			return
		}
		if envDir := os.Getenv("AI_HUB_DATA_DIR"); envDir != "" {
			dataDir = envDir
			return
		}
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".ai-hub")
	})
}

// GetDataDir returns the global data directory.
// Falls back to ~/.ai-hub if InitDataDir was never called.
func GetDataDir() string {
	if dataDir == "" {
		// Fallback for cases where InitDataDir wasn't called
		if envDir := os.Getenv("AI_HUB_DATA_DIR"); envDir != "" {
			return envDir
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".ai-hub")
	}
	return dataDir
}

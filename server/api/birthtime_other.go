//go:build !darwin

package api

import (
	"os"
	"time"
)

// fileBirthTime returns the file's modification time on non-macOS platforms.
// Linux does not reliably expose birth time via syscall.Stat_t.
func fileBirthTime(info os.FileInfo) time.Time {
	return info.ModTime()
}

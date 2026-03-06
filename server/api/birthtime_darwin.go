//go:build darwin

package api

import (
	"os"
	"syscall"
	"time"
)

// fileBirthTime returns the file's birth time on macOS.
// Falls back to modification time if unavailable.
func fileBirthTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		bt := time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
		if !bt.IsZero() {
			return bt
		}
	}
	return info.ModTime()
}

//go:build !windows

package core

// decodeStderr on non-Windows platforms just returns the string as-is
// since Unix systems typically use UTF-8
func decodeStderr(data []byte) string {
	return string(data)
}

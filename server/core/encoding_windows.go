//go:build windows

package core

import (
	"bytes"
	"io"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// decodeGBK converts GBK encoded bytes to UTF-8 string
// This is needed on Windows where console output may be GBK encoded
func decodeGBK(data []byte) string {
	// Try to decode as GBK first
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		// If GBK decode fails, return as-is (might already be UTF-8)
		return string(data)
	}
	return string(decoded)
}

// decodeStderr decodes stderr output, handling Windows GBK encoding
func decodeStderr(data []byte) string {
	return decodeGBK(data)
}

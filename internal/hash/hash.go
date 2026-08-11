package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// File computes SHA-256 of a file and returns hex string.
func File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Save writes the SHA-256 hash of filePath to hashFilePath.
func Save(filePath, hashFilePath string) error {
	h, err := File(filePath)
	if err != nil {
		return err
	}
	return os.WriteFile(hashFilePath, []byte(h), 0600)
}

// Verify checks SHA-256 of filePath against the hash stored in hashFilePath.
func Verify(filePath, hashFilePath string) (bool, error) {
	stored, err := os.ReadFile(hashFilePath)
	if err != nil {
		return false, err
	}
	actual, err := File(filePath)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(stored)) == actual, nil
}

// Bytes computes SHA-256 of a byte slice.
func Bytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// FormatShort returns an abbreviated hash display string.
func FormatShort(h string) string {
	if len(h) > 16 {
		return fmt.Sprintf("%s...%s", h[:8], h[len(h)-8:])
	}
	return h
}

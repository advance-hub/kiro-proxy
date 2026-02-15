package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed sidecar/*
var sidecarFS embed.FS

// sidecarName returns the expected binary name for the current platform.
func sidecarName() string {
	if runtime.GOOS == "windows" {
		return "kiro-rs.exe"
	}
	return "kiro-rs"
}

// extractSidecar extracts the embedded kiro-rs binary to a cache directory
// and returns the path. It uses content-hash based caching so the binary
// is only written when it changes.
func extractSidecar() (string, error) {
	name := sidecarName()
	data, err := fs.ReadFile(sidecarFS, "sidecar/"+name)
	if err != nil {
		return "", fmt.Errorf("embedded kiro-rs not found: %v", err)
	}

	// Cache dir: <dataDir>/bin/
	dataDir, err := getDataDir()
	if err != nil {
		return "", err
	}
	binDir := filepath.Join(dataDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("创建 bin 目录失败: %v", err)
	}

	outPath := filepath.Join(binDir, name)

	// Check if existing binary matches (by size + hash prefix)
	if info, err := os.Stat(outPath); err == nil && info.Size() == int64(len(data)) {
		// Size matches, check hash
		existing, err := os.ReadFile(outPath)
		if err == nil {
			newHash := sha256.Sum256(data)
			oldHash := sha256.Sum256(existing)
			if newHash == oldHash {
				// Already up to date
				return outPath, nil
			}
		}
	}

	// Write new binary
	if err := os.WriteFile(outPath, data, 0755); err != nil {
		return "", fmt.Errorf("释放 kiro-rs 失败: %v", err)
	}

	return outPath, nil
}

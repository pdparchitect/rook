package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultConfigPath returns the resolved config file path using a fallback
// chain:
//
//  1. $ROOK_CONFIG environment variable (if set and non-empty)
//  2. $XDG_CONFIG_HOME/rook/config.yaml (if XDG_CONFIG_HOME is set)
//  3. ~/.config/rook/config.yaml
func DefaultConfigPath() string {
	if envPath := strings.TrimSpace(os.Getenv("ROOK_CONFIG")); envPath != "" {
		return envPath
	}

	return filepath.Join(xdgConfigHome(), "rook", "config.yaml")
}

// DefaultRunDir returns the base directory for per-run artifacts:
//
//  1. $XDG_STATE_HOME/rook/runs (if XDG_STATE_HOME is set)
//  2. ~/.local/state/rook/runs
func DefaultRunDir() string {
	return filepath.Join(xdgStateHome(), "rook", "runs")
}

func xdgConfigHome() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return dir
	}
	return filepath.Join(homeDir(), ".config")
}

func xdgStateHome() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); dir != "" {
		return dir
	}
	return filepath.Join(homeDir(), ".local", "state")
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}

	// Fallback for unusual environments.
	return "/tmp/rook-" + strconv.Itoa(os.Getuid())
}

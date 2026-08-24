package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdparchitect/rook/internal/objective"
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

// SkillsDir returns the directory where on-disk skills live, layered over the
// embedded set: $XDG_CONFIG_HOME/rook/skills or ~/.config/rook/skills. The
// runner rescans it every iteration, so a skill placed here - by the operator,
// or by the agent itself cloning a collection mid-run - surfaces on the
// agent's next turn without a restart.
func SkillsDir() string {
	return filepath.Join(xdgConfigHome(), "rook", "skills")
}

// DefaultRunDir returns the base directory for per-run artifacts:
//
//  1. $XDG_STATE_HOME/rook/runs (if XDG_STATE_HOME is set)
//  2. ~/.local/state/rook/runs
func DefaultRunDir() string {
	return filepath.Join(xdgStateHome(), "rook", "runs")
}

// DefaultSessionDir returns the directory where session logs are written, so a
// run can be inspected afterwards and resumed. Mirrors zot's session directory:
// project-local under .rook/sessions by default, so a project's sessions stay
// with its objectives and records. An absolute --session-dir overrides it.
func DefaultSessionDir() string {
	return filepath.Join(objectiveDossierDir(), "sessions")
}

// objectiveDossierDir returns the .rook directory under the current working
// directory, the per-project home for objectives, records, and sessions.
func objectiveDossierDir() string {
	return filepath.Join(".", objective.DossierDir)
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

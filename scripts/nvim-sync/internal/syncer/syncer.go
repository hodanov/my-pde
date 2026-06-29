// Package syncer copies changed Neovim config files from the host into a
// running nvim container via `docker cp`, preserving each file's path relative
// to the configured source root.
package syncer

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Runner executes an external command. It is injected so tests can stub out
// `docker` without depending on a real Docker daemon.
type Runner func(name string, args ...string) error

// Syncer copies files into a target container.
type Syncer struct {
	// Container is the target container name (e.g. "nvim-dev").
	Container string
	// SrcRoot is the host directory whose tree is mirrored into the container.
	SrcRoot string
	// DestRoot is the directory inside the container that mirrors SrcRoot.
	DestRoot string
	// Run executes commands (defaults to a real exec runner in main).
	Run Runner
}

// Dest returns the container-side `docker cp` destination for a changed file,
// e.g. "nvim-dev:/root/.config/nvim/lua/ai_bridge.lua". The returned bool is
// false if changed lies outside SrcRoot.
func (s *Syncer) Dest(changed string) (string, bool) {
	rel, relErr := filepath.Rel(s.SrcRoot, changed)
	if relErr != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return fmt.Sprintf("%s:%s", s.Container, path.Join(s.DestRoot, rel)), true
}

// Copy copies a single changed file into the container with `docker cp`.
func (s *Syncer) Copy(changed string) error {
	dest, ok := s.Dest(changed)
	if !ok {
		return fmt.Errorf("file %q is outside src root %q", changed, s.SrcRoot)
	}
	if runErr := s.Run("docker", "cp", changed, dest); runErr != nil {
		return fmt.Errorf("docker cp %s -> %s: %w", changed, dest, runErr)
	}
	return nil
}

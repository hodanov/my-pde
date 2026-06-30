// Package fsrepo implements the filesystem-backed ports of ai-bridge: the
// request repository, the launch-script store, and directory checks.
package fsrepo

import (
	"encoding/json"
	"fmt"
	"os"

	"ai-bridge/internal/domain"
)

// RequestRepository reads and removes request files. It implements the
// usecase.RequestRepository port.
type RequestRepository struct{}

// requestDTO is the on-disk JSON shape of a request. It is kept separate from
// domain.Request so that serialization concerns stay in the infrastructure
// layer; validation happens via domain.NewRequest.
type requestDTO struct {
	Prompt    string `json:"prompt"`
	CWD       string `json:"cwd"`
	Timestamp int64  `json:"timestamp"`
}

// Load reads, deserializes and validates the request at path.
func (RequestRepository) Load(path string) (*domain.Request, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("read request: %w", readErr)
	}
	var dto requestDTO
	if unmarshalErr := json.Unmarshal(data, &dto); unmarshalErr != nil {
		return nil, fmt.Errorf("parse request JSON: %w", unmarshalErr)
	}
	return domain.NewRequest(dto.Prompt, dto.CWD, dto.Timestamp)
}

// Remove deletes the consumed request file, ignoring errors.
func (RequestRepository) Remove(path string) {
	_ = os.Remove(path)
}

// ScriptStore writes self-deleting launch scripts to temporary files. It
// implements the usecase.ScriptStore port.
type ScriptStore struct{}

// Save creates a temporary script file, builds its contents with the final path
// (needed for the self-deleting line), writes them and makes it executable.
func (ScriptStore) Save(build func(scriptPath string) string) (scriptPath string, retErr error) {
	f, createErr := os.CreateTemp("", "ai-bridge-*.sh")
	if createErr != nil {
		return "", fmt.Errorf("create temp script: %w", createErr)
	}
	defer func() {
		_ = f.Close()
		if retErr != nil {
			_ = os.Remove(f.Name())
		}
	}()

	if _, writeErr := f.WriteString(build(f.Name())); writeErr != nil {
		return "", fmt.Errorf("write temp script: %w", writeErr)
	}
	if chmodErr := f.Chmod(0o755); chmodErr != nil {
		return "", fmt.Errorf("chmod temp script: %w", chmodErr)
	}
	return f.Name(), nil
}

// Remove deletes a previously saved script, ignoring errors.
func (ScriptStore) Remove(path string) {
	_ = os.Remove(path)
}

// Dir implements the directory-related ports: usecase.DirVerifier (cwd
// validation), usecase.BridgeDirEnsurer (bridge dir creation) and
// usecase.DirInspector (diagnostics).
type Dir struct{}

// IsDir reports whether path is an existing directory.
func (Dir) IsDir(path string) bool {
	info, statErr := os.Stat(path)
	return statErr == nil && info.IsDir()
}

// Ensure creates dir (and parents) if it does not already exist.
func (Dir) Ensure(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// StatDir reports whether path is a directory; err is non-nil when the path
// cannot be stat'd (e.g. it does not exist).
func (Dir) StatDir(path string) (bool, error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		return false, statErr
	}
	return info.IsDir(), nil
}

// EnsureWritable returns an error if a file cannot be created in dir.
func (Dir) EnsureWritable(dir string) error {
	f, createErr := os.CreateTemp(dir, ".doctor-write-*")
	if createErr != nil {
		return createErr
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

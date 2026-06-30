// Package fsrepo implements the filesystem-backed ports of ai-bridge: the
// request repository, the launch-script store, and directory checks.
package fsrepo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ai-bridge/internal/domain"
)

// Canonical file names inside the bridge directory.
const (
	requestFileName = "request.json"
	historyFileName = "history.jsonl"
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

// toDTO projects a validated request onto its on-disk JSON shape.
func toDTO(req *domain.Request) requestDTO {
	return requestDTO{Prompt: req.Prompt, CWD: req.CWD, Timestamp: req.Timestamp}
}

// RequestWriter writes request.json atomically. It implements the
// usecase.RequestWriter port.
type RequestWriter struct{}

// Save marshals req and writes it to <bridgeDir>/request.json via a temp file in
// the same directory followed by a rename, so the daemon never observes a
// partially written request.
func (RequestWriter) Save(bridgeDir string, req *domain.Request) (retErr error) {
	data, marshalErr := json.Marshal(toDTO(req))
	if marshalErr != nil {
		return fmt.Errorf("marshal request: %w", marshalErr)
	}

	tmp, createErr := os.CreateTemp(bridgeDir, ".request-*.tmp")
	if createErr != nil {
		return fmt.Errorf("create temp request: %w", createErr)
	}
	tmpName := tmp.Name()
	defer func() {
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp request: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("close temp request: %w", closeErr)
	}
	if renameErr := os.Rename(tmpName, filepath.Join(bridgeDir, requestFileName)); renameErr != nil {
		return fmt.Errorf("rename temp request: %w", renameErr)
	}
	return nil
}

// HistoryRepository persists requests to an append-only JSONL file. It
// implements the usecase.HistoryRepository port.
type HistoryRepository struct{}

// Append writes req as one JSON line to <bridgeDir>/history.jsonl, creating the
// file if needed. A single O_APPEND write keeps concurrent lines from interleaving.
func (HistoryRepository) Append(bridgeDir string, req *domain.Request) error {
	f, openErr := os.OpenFile(filepath.Join(bridgeDir, historyFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		return fmt.Errorf("open history: %w", openErr)
	}
	defer func() { _ = f.Close() }()

	data, marshalErr := json.Marshal(toDTO(req))
	if marshalErr != nil {
		return fmt.Errorf("marshal record: %w", marshalErr)
	}
	if _, writeErr := f.Write(append(data, '\n')); writeErr != nil {
		return fmt.Errorf("write history: %w", writeErr)
	}
	return nil
}

// Load reads all history entries newest-first. A missing file yields an empty
// slice and no error. Blank and corrupt (unparseable or invalid) lines are
// skipped so one bad row cannot block the whole history.
func (HistoryRepository) Load(bridgeDir string) ([]*domain.Request, error) {
	f, openErr := os.Open(filepath.Join(bridgeDir, historyFileName))
	if openErr != nil {
		if os.IsNotExist(openErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history: %w", openErr)
	}
	defer func() { _ = f.Close() }()

	var records []*domain.Request
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var dto requestDTO
		if unmarshalErr := json.Unmarshal(line, &dto); unmarshalErr != nil {
			continue // skip corrupt line
		}
		req, newErr := domain.NewRequest(dto.Prompt, dto.CWD, dto.Timestamp)
		if newErr != nil {
			continue // skip invalid line
		}
		records = append(records, req)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("read history: %w", scanErr)
	}

	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
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

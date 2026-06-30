package fsrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestRepositoryLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		content       string
		noFile        bool
		wantPrompt    string
		wantCWD       string
		wantTimestamp int64
		wantErr       bool
	}{
		{
			name:          "valid request returns parsed fields",
			content:       `{"prompt":"hello","cwd":"/tmp","timestamp":1234}`,
			wantPrompt:    "hello",
			wantCWD:       "/tmp",
			wantTimestamp: 1234,
		},
		{
			name:    "missing prompt returns error",
			content: `{"cwd":"/tmp","timestamp":1234}`,
			wantErr: true,
		},
		{
			name:    "missing cwd returns error",
			content: `{"prompt":"hello","timestamp":1234}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON returns error",
			content: `not json`,
			wantErr: true,
		},
		{
			name:    "file not found returns error",
			noFile:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reqFile string
			if tt.noFile {
				reqFile = "/nonexistent/request.json"
			} else {
				reqFile = filepath.Join(t.TempDir(), "request.json")
				if writeErr := os.WriteFile(reqFile, []byte(tt.content), 0o644); writeErr != nil {
					t.Fatal(writeErr)
				}
			}

			req, err := RequestRepository{}.Load(reqFile)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if req.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", req.Prompt, tt.wantPrompt)
			}
			if req.CWD != tt.wantCWD {
				t.Errorf("CWD = %q, want %q", req.CWD, tt.wantCWD)
			}
			if req.Timestamp != tt.wantTimestamp {
				t.Errorf("Timestamp = %d, want %d", req.Timestamp, tt.wantTimestamp)
			}
		})
	}
}

func TestScriptStoreSave(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		build        func(scriptPath string) string
		wantContains []string
	}{
		{
			name:         "writes content from builder and is executable",
			build:        func(p string) string { return "#!/bin/bash\nclaude 'hi'\nrm -f " + p + "\n" },
			wantContains: []string{"#!/bin/bash\n", "claude", "rm -f"},
		},
		{
			name:         "builder receives the final script path",
			build:        func(p string) string { return "path=" + p },
			wantContains: []string{"path=/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, saveErr := ScriptStore{}.Save(tt.build)
			if saveErr != nil {
				t.Fatal(saveErr)
			}
			defer func() { _ = os.Remove(path) }()

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			content := string(data)
			for _, want := range tt.wantContains {
				if !strings.Contains(content, want) {
					t.Errorf("script missing %q\ncontent:\n%s", want, content)
				}
			}

			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if info.Mode().Perm()&0o100 == 0 {
				t.Error("script should be executable")
			}
		})
	}
}

func TestDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "afile")
	if writeErr := os.WriteFile(filePath, []byte("x"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	t.Run("IsDir", func(t *testing.T) {
		t.Parallel()
		if !(Dir{}).IsDir(dir) {
			t.Errorf("IsDir(%q) = false, want true", dir)
		}
		if (Dir{}).IsDir(filePath) {
			t.Errorf("IsDir(file) = true, want false")
		}
		if (Dir{}).IsDir(filepath.Join(dir, "missing")) {
			t.Errorf("IsDir(missing) = true, want false")
		}
	})

	t.Run("Ensure creates nested dir", func(t *testing.T) {
		t.Parallel()
		target := filepath.Join(t.TempDir(), "a", "b")
		if err := (Dir{}).Ensure(target); err != nil {
			t.Fatal(err)
		}
		if !(Dir{}).IsDir(target) {
			t.Error("Ensure did not create directory")
		}
	})

	t.Run("StatDir distinguishes dir, file and missing", func(t *testing.T) {
		t.Parallel()
		isDir, err := (Dir{}).StatDir(dir)
		if err != nil || !isDir {
			t.Errorf("StatDir(dir) = (%v, %v), want (true, nil)", isDir, err)
		}
		isDir, err = (Dir{}).StatDir(filePath)
		if err != nil || isDir {
			t.Errorf("StatDir(file) = (%v, %v), want (false, nil)", isDir, err)
		}
		if _, statErr := (Dir{}).StatDir(filepath.Join(dir, "missing")); statErr == nil {
			t.Error("StatDir(missing) error = nil, want non-nil")
		}
	})

	t.Run("EnsureWritable", func(t *testing.T) {
		t.Parallel()
		if err := (Dir{}).EnsureWritable(dir); err != nil {
			t.Errorf("EnsureWritable(dir) = %v, want nil", err)
		}
		if err := (Dir{}).EnsureWritable(filepath.Join(dir, "missing")); err == nil {
			t.Error("EnsureWritable(missing) = nil, want error")
		}
	})
}

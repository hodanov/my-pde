package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"agent-stats/internal/permission"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunJoinsLedgerTranscriptsAndLocalRules(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	worktreeCwd := filepath.Join(repo, "sub")
	writeFile(t, filepath.Join(worktreeCwd, ".keep"), "")
	writeFile(t, filepath.Join(repo, ".claude", "settings.local.json"), `{"permissions":{"allow":["Bash(uv run:*)","Bash(git *)"]}}`)

	settings := filepath.Join(root, "settings.json")
	writeFile(t, settings, `{"permissions":{"allow":["Bash(git *)"]}}`)
	transcripts := filepath.Join(root, "projects")
	transcript := filepath.Join(transcripts, "p", "s1.jsonl")
	ledger := filepath.Join(root, "ledger.jsonl")
	writeFile(t, ledger, `{"ts":"2026-09-12T10:00:00Z","session_id":"s1","tool_name":"Bash","cwd":"`+worktreeCwd+`","transcript_path":"`+transcript+`","tool_input":{"command":"go env GOPATH"},"suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"go env *"}],"behavior":"allow"}]}`+"\n")
	writeFile(t, transcript, `{"type":"assistant","timestamp":"2026-09-12T09:59:59.500Z","cwd":"`+worktreeCwd+`","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go env GOPATH"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"/go"}]}}
`)
	args := []string{
		"--settings", settings, "--ledger", ledger, "--declined", filepath.Join(root, "absent.txt"),
		"--dir", transcripts, "--since", "0",
	}

	var jsonOut bytes.Buffer
	if err := run(append(args, "--json"), &jsonOut); err != nil {
		t.Fatalf("run --json: %v", err)
	}
	var got permission.Report
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, jsonOut.String())
	}
	want := permission.Report{
		Candidates: []permission.Candidate{
			{
				Rule: "Bash(go env *)", Approved: 1, Sessions: 1, Repos: []string{repo},
				LastSeen: time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC), Examples: []string{"go env GOPATH"},
			},
			{Rule: "Bash(uv run *)", LocalRepos: []string{repo}},
		},
		Unsuggested: []permission.Candidate{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("report =\n%+v\nwant\n%+v", got, want)
	}

	var table bytes.Buffer
	if err := run(args, &table); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, s := range []string{"Suggested rules: 2", "Bash(go env *)", "go env GOPATH", "Drafted rules"} {
		if !strings.Contains(table.String(), s) {
			t.Errorf("table lacks %q:\n%s", s, table.String())
		}
	}
}

func TestRunArguments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    func(root string) []string
		wantErr bool
	}{
		{
			name:    "settings is required",
			args:    func(string) []string { return nil },
			wantErr: true,
		},
		{
			name: "missing ledger, declined list and transcripts are an empty audit",
			args: func(root string) []string {
				settings := filepath.Join(root, "settings.json")
				writeFile(t, settings, `{}`)
				return []string{
					"--settings", settings, "--ledger", filepath.Join(root, "none.jsonl"),
					"--declined", filepath.Join(root, "none.txt"), "--dir", filepath.Join(root, "none"), "--json",
				}
			},
		},
		{
			name: "unreadable settings is an error",
			args: func(root string) []string {
				return []string{"--settings", filepath.Join(root, "none.json"), "--dir", root}
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := run(tt.args(t.TempDir()), &out)
			if (err != nil) != tt.wantErr {
				t.Errorf("run err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

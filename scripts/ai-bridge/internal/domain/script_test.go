package domain

import (
	"strings"
	"testing"
)

func TestBuildScript(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cli          string
		prompt       string
		scriptPath   string
		wantContains []string
	}{
		{
			name:       "basic script has shebang, cli, prompt and self-delete",
			cli:        "claude",
			prompt:     "hello world",
			scriptPath: "/tmp/ai-bridge-123.sh",
			wantContains: []string{
				"#!/bin/bash\n",
				"claude",
				"hello world",
				"rm -f",
				"/tmp/ai-bridge-123.sh",
			},
		},
		{
			name:       "special chars in prompt are safely quoted",
			cli:        "claude",
			prompt:     `it's a "test" with $vars`,
			scriptPath: "/tmp/x.sh",
			wantContains: []string{
				"it",
				"$vars",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			content := BuildScript(tt.cli, tt.prompt, tt.scriptPath)
			for _, want := range tt.wantContains {
				if !strings.Contains(content, want) {
					t.Errorf("script missing %q\ncontent:\n%s", want, content)
				}
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"it's", "'it'\"'\"'s'"},
		{"hello world", "'hello world'"},
		{"$var", "'$var'"},
		{"", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := ShellQuote(tt.input)
			if got != tt.want {
				t.Errorf("ShellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

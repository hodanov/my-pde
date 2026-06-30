package domain

import "testing"

func TestNewRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		prompt        string
		cwd           string
		timestamp     int64
		wantPrompt    string
		wantCWD       string
		wantTimestamp int64
		wantErr       bool
	}{
		{
			name:          "valid request returns value object",
			prompt:        "hello",
			cwd:           "/tmp",
			timestamp:     1234,
			wantPrompt:    "hello",
			wantCWD:       "/tmp",
			wantTimestamp: 1234,
		},
		{
			name:    "empty prompt returns error",
			prompt:  "",
			cwd:     "/tmp",
			wantErr: true,
		},
		{
			name:    "empty cwd returns error",
			prompt:  "hello",
			cwd:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req, err := NewRequest(tt.prompt, tt.cwd, tt.timestamp)

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

package domain

import "fmt"

// Request is a validated bridge request: a prompt to run in a working directory.
type Request struct {
	Prompt    string
	CWD       string
	Timestamp int64
}

// NewRequest validates the required invariants of a request (non-empty prompt
// and working directory) and returns the value object. Serialization (JSON) is
// an infrastructure concern handled by the request repository adapter.
func NewRequest(prompt, cwd string, timestamp int64) (*Request, error) {
	if cwd == "" {
		return nil, fmt.Errorf("cwd is null or empty")
	}
	if prompt == "" {
		return nil, fmt.Errorf("prompt is null or empty")
	}
	return &Request{Prompt: prompt, CWD: cwd, Timestamp: timestamp}, nil
}

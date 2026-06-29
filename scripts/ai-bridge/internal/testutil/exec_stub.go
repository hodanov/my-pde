// Package testutil provides shared helpers for ai-bridge tests.
package testutil

import "sync"

// ExecRecord records a single command execution.
type ExecRecord struct {
	Name string
	Args []string
}

// ExecStub records command executions for test verification.
type ExecStub struct {
	mu      sync.Mutex
	records []ExecRecord
	err     error
}

// NewExecStub creates a stub that returns the given error (nil for success).
func NewExecStub(err error) *ExecStub {
	return &ExecStub{err: err}
}

// Run records the command and returns the configured error.
func (s *ExecStub) Run(name string, args ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]string, len(args))
	copy(cp, args)
	s.records = append(s.records, ExecRecord{Name: name, Args: cp})
	return s.err
}

// Records returns all recorded command executions.
func (s *ExecStub) Records() []ExecRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	dst := make([]ExecRecord, len(s.records))
	copy(dst, s.records)
	return dst
}

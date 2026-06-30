package usecase

import (
	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase/port"
)

// ListHistory reads the persisted request history newest-first for display.
type ListHistory struct {
	history   port.HistoryRepository
	bridgeDir string
}

// NewListHistory wires the dependencies of the history-listing use case.
func NewListHistory(history port.HistoryRepository, bridgeDir string) *ListHistory {
	return &ListHistory{history: history, bridgeDir: bridgeDir}
}

// Run returns the history entries, newest first.
func (uc *ListHistory) Run() ([]*domain.Request, error) {
	return uc.history.Load(uc.bridgeDir)
}

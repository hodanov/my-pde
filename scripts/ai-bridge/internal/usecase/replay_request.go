package usecase

import (
	"fmt"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase/port"
)

// ReplayRequest re-injects the most recent history entry as a new request so the
// running daemon picks it up through its existing watcher/launcher path.
type ReplayRequest struct {
	history   port.HistoryRepository
	writer    port.RequestWriter
	bridgeDir string
	now       func() int64
}

// NewReplayRequest wires the dependencies of the replay use case. now supplies
// the replay timestamp (injected for deterministic tests).
func NewReplayRequest(history port.HistoryRepository, writer port.RequestWriter, bridgeDir string, now func() int64) *ReplayRequest {
	return &ReplayRequest{history: history, writer: writer, bridgeDir: bridgeDir, now: now}
}

// Run loads the history, selects the most recent entry, stamps it with the
// replay time and writes it back as request.json. It errors when there is
// nothing to replay.
func (uc *ReplayRequest) Run() error {
	records, loadErr := uc.history.Load(uc.bridgeDir)
	if loadErr != nil {
		return fmt.Errorf("load history: %w", loadErr)
	}
	if len(records) == 0 {
		return fmt.Errorf("no history to replay")
	}

	latest := records[0]
	replayed, newErr := domain.NewRequest(latest.Prompt, latest.CWD, uc.now())
	if newErr != nil {
		return fmt.Errorf("build replay request: %w", newErr)
	}
	if saveErr := uc.writer.Save(uc.bridgeDir, replayed); saveErr != nil {
		return fmt.Errorf("write request: %w", saveErr)
	}
	return nil
}

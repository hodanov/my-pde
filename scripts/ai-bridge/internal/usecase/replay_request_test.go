package usecase_test

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase"
	"ai-bridge/internal/usecase/port/mock"
)

func TestReplayRequestRun(t *testing.T) {
	t.Parallel()
	const replayTime int64 = 999
	// History is newest-first; the most recent entry is replayed with a fresh timestamp.
	loaded := []*domain.Request{
		{Prompt: "latest", CWD: "/work", Timestamp: 5},
		{Prompt: "older", CWD: "/old", Timestamp: 1},
	}
	wantSaved := &domain.Request{Prompt: "latest", CWD: "/work", Timestamp: replayTime}

	tests := []struct {
		name    string
		setup   func(history *mock.MockHistoryRepository, writer *mock.MockRequestWriter)
		wantErr bool
	}{
		{
			name: "replays newest entry with refreshed timestamp",
			setup: func(history *mock.MockHistoryRepository, writer *mock.MockRequestWriter) {
				history.EXPECT().Load("/bridge").Return(loaded, nil)
				writer.EXPECT().Save("/bridge", wantSaved).Return(nil)
			},
		},
		{
			name: "empty history returns error and never writes",
			setup: func(history *mock.MockHistoryRepository, _ *mock.MockRequestWriter) {
				history.EXPECT().Load("/bridge").Return(nil, nil)
			},
			wantErr: true,
		},
		{
			name: "load error is propagated",
			setup: func(history *mock.MockHistoryRepository, _ *mock.MockRequestWriter) {
				history.EXPECT().Load("/bridge").Return(nil, errors.New("read failed"))
			},
			wantErr: true,
		},
		{
			name: "write error is propagated",
			setup: func(history *mock.MockHistoryRepository, writer *mock.MockRequestWriter) {
				history.EXPECT().Load("/bridge").Return(loaded, nil)
				writer.EXPECT().Save("/bridge", wantSaved).Return(errors.New("disk full"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			history := mock.NewMockHistoryRepository(ctrl)
			writer := mock.NewMockRequestWriter(ctrl)
			tt.setup(history, writer)

			now := func() int64 { return replayTime }
			runErr := usecase.NewReplayRequest(history, writer, "/bridge", now).Run()
			if gotErr := runErr != nil; gotErr != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", runErr, tt.wantErr)
			}
		})
	}
}

package usecase_test

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase"
	"ai-bridge/internal/usecase/port/mock"
)

func TestListHistoryRun(t *testing.T) {
	t.Parallel()
	records := []*domain.Request{
		{Prompt: "second", CWD: "/b", Timestamp: 2},
		{Prompt: "first", CWD: "/a", Timestamp: 1},
	}

	tests := []struct {
		name    string
		setup   func(history *mock.MockHistoryRepository)
		wantLen int
		wantErr bool
	}{
		{
			name: "passes through loaded records",
			setup: func(history *mock.MockHistoryRepository) {
				history.EXPECT().Load("/bridge").Return(records, nil)
			},
			wantLen: 2,
		},
		{
			name: "load error is propagated",
			setup: func(history *mock.MockHistoryRepository) {
				history.EXPECT().Load("/bridge").Return(nil, errors.New("read failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			history := mock.NewMockHistoryRepository(ctrl)
			tt.setup(history)

			got, runErr := usecase.NewListHistory(history, "/bridge").Run()
			if gotErr := runErr != nil; gotErr != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", runErr, tt.wantErr)
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("got %d records, want %d", len(got), tt.wantLen)
			}
		})
	}
}

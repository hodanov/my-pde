package usecase_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase"
	"ai-bridge/internal/usecase/port/mock"
)

func TestRunDaemonRun(t *testing.T) {
	t.Parallel()
	cfg := &domain.Config{BridgeDir: "/bridge", CLI: "claude", Launcher: "wezterm"}
	validReq := &domain.Request{Prompt: "hi", CWD: "/work", Timestamp: 1}

	tests := []struct {
		name    string
		setup   func(ensurer *mock.MockBridgeDirEnsurer, watcher *mock.MockWatcher, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, scripts *mock.MockScriptStore, launcher *mock.MockLauncher)
		wantErr bool
	}{
		{
			name: "ensure error returns immediately",
			setup: func(ensurer *mock.MockBridgeDirEnsurer, _ *mock.MockWatcher, _ *mock.MockRequestRepository, _ *mock.MockDirVerifier, _ *mock.MockScriptStore, _ *mock.MockLauncher) {
				ensurer.EXPECT().Ensure("/bridge").Return(errors.New("permission denied"))
			},
			wantErr: true,
		},
		{
			name: "watch error returns error",
			setup: func(ensurer *mock.MockBridgeDirEnsurer, watcher *mock.MockWatcher, _ *mock.MockRequestRepository, _ *mock.MockDirVerifier, _ *mock.MockScriptStore, _ *mock.MockLauncher) {
				ensurer.EXPECT().Ensure("/bridge").Return(nil)
				watcher.EXPECT().Watch(gomock.Any()).Return(nil, errors.New("fsnotify failed"))
			},
			wantErr: true,
		},
		{
			name: "consumed request is dispatched then channel closes",
			setup: func(ensurer *mock.MockBridgeDirEnsurer, watcher *mock.MockWatcher, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, scripts *mock.MockScriptStore, launcher *mock.MockLauncher) {
				ensurer.EXPECT().Ensure("/bridge").Return(nil)
				ch := make(chan string, 1)
				ch <- "/bridge/request.json.consumed"
				close(ch)
				var recv <-chan string = ch
				watcher.EXPECT().Watch(gomock.Any()).Return(recv, nil)
				reqs.EXPECT().Load("/bridge/request.json.consumed").Return(validReq, nil)
				reqs.EXPECT().Remove("/bridge/request.json.consumed")
				dirs.EXPECT().IsDir("/work").Return(true)
				scripts.EXPECT().Save(gomock.Any()).Return("/tmp/s.sh", nil)
				launcher.EXPECT().Launch("/work", "/tmp/s.sh").Return(nil)
			},
		},
		{
			name: "failing request is logged but loop continues to close",
			setup: func(ensurer *mock.MockBridgeDirEnsurer, watcher *mock.MockWatcher, reqs *mock.MockRequestRepository, _ *mock.MockDirVerifier, _ *mock.MockScriptStore, _ *mock.MockLauncher) {
				ensurer.EXPECT().Ensure("/bridge").Return(nil)
				ch := make(chan string, 1)
				ch <- "/bridge/request.json.consumed"
				close(ch)
				var recv <-chan string = ch
				watcher.EXPECT().Watch(gomock.Any()).Return(recv, nil)
				reqs.EXPECT().Load("/bridge/request.json.consumed").Return(nil, errors.New("bad json"))
				reqs.EXPECT().Remove("/bridge/request.json.consumed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			ensurer := mock.NewMockBridgeDirEnsurer(ctrl)
			watcher := mock.NewMockWatcher(ctrl)
			reqs := mock.NewMockRequestRepository(ctrl)
			dirs := mock.NewMockDirVerifier(ctrl)
			scripts := mock.NewMockScriptStore(ctrl)
			launcher := mock.NewMockLauncher(ctrl)
			tt.setup(ensurer, watcher, reqs, dirs, scripts, launcher)

			process := usecase.NewProcessRequest(reqs, dirs, scripts, launcher, cfg.CLI)
			uc := usecase.NewRunDaemon(ensurer, watcher, process, cfg)
			err := uc.Run(context.Background())

			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

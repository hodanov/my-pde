package usecase_test

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase"
	"ai-bridge/internal/usecase/mock"
)

func TestInstallAgentRun(t *testing.T) {
	t.Parallel()
	cfg := &domain.Config{BridgeDir: "/bridge", CLI: "claude", Launcher: "wezterm"}

	tests := []struct {
		name    string
		setup   func(exe *mock.MockExecutable, installer *mock.MockAgentInstaller)
		wantErr bool
	}{
		{
			name: "resolves binary and installs",
			setup: func(exe *mock.MockExecutable, installer *mock.MockAgentInstaller) {
				exe.EXPECT().Path().Return("/usr/local/bin/ai-bridge", nil)
				installer.EXPECT().Install("/usr/local/bin/ai-bridge", "claude", "wezterm").Return(nil)
			},
		},
		{
			name: "executable resolution error short-circuits",
			setup: func(exe *mock.MockExecutable, _ *mock.MockAgentInstaller) {
				exe.EXPECT().Path().Return("", errors.New("cannot resolve"))
			},
			wantErr: true,
		},
		{
			name: "install error is propagated",
			setup: func(exe *mock.MockExecutable, installer *mock.MockAgentInstaller) {
				exe.EXPECT().Path().Return("/usr/local/bin/ai-bridge", nil)
				installer.EXPECT().Install("/usr/local/bin/ai-bridge", "claude", "wezterm").Return(errors.New("launchctl failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			exe := mock.NewMockExecutable(ctrl)
			installer := mock.NewMockAgentInstaller(ctrl)
			tt.setup(exe, installer)

			err := usecase.NewInstallAgent(exe, installer, cfg).Run()

			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

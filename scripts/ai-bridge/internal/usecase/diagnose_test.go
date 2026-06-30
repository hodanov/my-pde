package usecase_test

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase"
	"ai-bridge/internal/usecase/mock"
)

func TestDiagnoseRun(t *testing.T) {
	t.Parallel()
	cfg := &domain.Config{BridgeDir: "/bridge", CLI: "claude", Launcher: "wezterm"}

	tests := []struct {
		name        string
		setup       func(dirs *mock.MockDirInspector, locator *mock.MockCommandLocator)
		wantStatus  []domain.Status
		wantFailure bool
	}{
		{
			name: "all ok",
			setup: func(dirs *mock.MockDirInspector, locator *mock.MockCommandLocator) {
				dirs.EXPECT().StatDir("/bridge").Return(true, nil)
				dirs.EXPECT().EnsureWritable("/bridge").Return(nil)
				locator.EXPECT().LookPath("claude").Return("/usr/bin/claude", nil)
				locator.EXPECT().LookPath("wezterm").Return("/usr/bin/wezterm", nil)
			},
			wantStatus:  []domain.Status{domain.StatusOK, domain.StatusOK, domain.StatusOK},
			wantFailure: false,
		},
		{
			name: "missing executables fail",
			setup: func(dirs *mock.MockDirInspector, locator *mock.MockCommandLocator) {
				dirs.EXPECT().StatDir("/bridge").Return(true, nil)
				dirs.EXPECT().EnsureWritable("/bridge").Return(nil)
				locator.EXPECT().LookPath("claude").Return("", errors.New("nope"))
				locator.EXPECT().LookPath("wezterm").Return("", errors.New("nope"))
			},
			wantStatus:  []domain.Status{domain.StatusOK, domain.StatusFail, domain.StatusFail},
			wantFailure: true,
		},
		{
			name: "missing bridge dir fails",
			setup: func(dirs *mock.MockDirInspector, locator *mock.MockCommandLocator) {
				dirs.EXPECT().StatDir("/bridge").Return(false, errors.New("no such file"))
				locator.EXPECT().LookPath("claude").Return("/usr/bin/claude", nil)
				locator.EXPECT().LookPath("wezterm").Return("/usr/bin/wezterm", nil)
			},
			wantStatus:  []domain.Status{domain.StatusFail, domain.StatusOK, domain.StatusOK},
			wantFailure: true,
		},
		{
			name: "bridge path is not a directory",
			setup: func(dirs *mock.MockDirInspector, locator *mock.MockCommandLocator) {
				dirs.EXPECT().StatDir("/bridge").Return(false, nil)
				locator.EXPECT().LookPath("claude").Return("/usr/bin/claude", nil)
				locator.EXPECT().LookPath("wezterm").Return("/usr/bin/wezterm", nil)
			},
			wantStatus:  []domain.Status{domain.StatusFail, domain.StatusOK, domain.StatusOK},
			wantFailure: true,
		},
		{
			name: "bridge dir not writable",
			setup: func(dirs *mock.MockDirInspector, locator *mock.MockCommandLocator) {
				dirs.EXPECT().StatDir("/bridge").Return(true, nil)
				dirs.EXPECT().EnsureWritable("/bridge").Return(errors.New("read-only"))
				locator.EXPECT().LookPath("claude").Return("/usr/bin/claude", nil)
				locator.EXPECT().LookPath("wezterm").Return("/usr/bin/wezterm", nil)
			},
			wantStatus:  []domain.Status{domain.StatusFail, domain.StatusOK, domain.StatusOK},
			wantFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			dirs := mock.NewMockDirInspector(ctrl)
			locator := mock.NewMockCommandLocator(ctrl)
			tt.setup(dirs, locator)

			checks := usecase.NewDiagnose(dirs, locator, cfg).Run()

			if len(checks) != len(tt.wantStatus) {
				t.Fatalf("got %d checks, want %d", len(checks), len(tt.wantStatus))
			}
			for i, want := range tt.wantStatus {
				if checks[i].Status != want {
					t.Errorf("check %d (%s) status = %v, want %v", i, checks[i].Name, checks[i].Status, want)
				}
			}
			if got := domain.HasFailure(checks); got != tt.wantFailure {
				t.Errorf("HasFailure() = %v, want %v", got, tt.wantFailure)
			}
		})
	}
}

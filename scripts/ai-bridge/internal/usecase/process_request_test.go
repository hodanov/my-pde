package usecase_test

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"ai-bridge/internal/domain"
	"ai-bridge/internal/usecase"
	"ai-bridge/internal/usecase/port/mock"
)

func TestProcessRequestHandle(t *testing.T) {
	t.Parallel()
	const consumed = "/bridge/request.json.consumed"
	validReq := &domain.Request{Prompt: "hi", CWD: "/work", Timestamp: 1}

	tests := []struct {
		name    string
		setup   func(t *testing.T, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, scripts *mock.MockScriptStore, launcher *mock.MockLauncher)
		wantErr bool
	}{
		{
			name: "valid request builds script and launches",
			setup: func(t *testing.T, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, scripts *mock.MockScriptStore, launcher *mock.MockLauncher) {
				t.Helper()
				reqs.EXPECT().Load(consumed).Return(validReq, nil)
				reqs.EXPECT().Remove(consumed)
				dirs.EXPECT().IsDir("/work").Return(true)
				scripts.EXPECT().Save(gomock.Any()).DoAndReturn(func(build func(string) string) (string, error) {
					content := build("/tmp/ai-bridge-x.sh")
					if !strings.Contains(content, "claude") || !strings.Contains(content, "hi") {
						t.Errorf("built script missing cli/prompt: %q", content)
					}
					return "/tmp/ai-bridge-x.sh", nil
				})
				launcher.EXPECT().Launch("/work", "/tmp/ai-bridge-x.sh").Return(nil)
			},
		},
		{
			name: "load error removes file and returns error",
			setup: func(t *testing.T, reqs *mock.MockRequestRepository, _ *mock.MockDirVerifier, _ *mock.MockScriptStore, _ *mock.MockLauncher) {
				t.Helper()
				reqs.EXPECT().Load(consumed).Return(nil, errors.New("bad json"))
				reqs.EXPECT().Remove(consumed)
			},
			wantErr: true,
		},
		{
			name: "invalid cwd returns error before script generation",
			setup: func(t *testing.T, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, _ *mock.MockScriptStore, _ *mock.MockLauncher) {
				t.Helper()
				reqs.EXPECT().Load(consumed).Return(validReq, nil)
				reqs.EXPECT().Remove(consumed)
				dirs.EXPECT().IsDir("/work").Return(false)
			},
			wantErr: true,
		},
		{
			name: "save error returns error before launch",
			setup: func(t *testing.T, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, scripts *mock.MockScriptStore, _ *mock.MockLauncher) {
				t.Helper()
				reqs.EXPECT().Load(consumed).Return(validReq, nil)
				reqs.EXPECT().Remove(consumed)
				dirs.EXPECT().IsDir("/work").Return(true)
				scripts.EXPECT().Save(gomock.Any()).Return("", errors.New("disk full"))
			},
			wantErr: true,
		},
		{
			name: "launch error removes script and returns error",
			setup: func(t *testing.T, reqs *mock.MockRequestRepository, dirs *mock.MockDirVerifier, scripts *mock.MockScriptStore, launcher *mock.MockLauncher) {
				t.Helper()
				reqs.EXPECT().Load(consumed).Return(validReq, nil)
				reqs.EXPECT().Remove(consumed)
				dirs.EXPECT().IsDir("/work").Return(true)
				scripts.EXPECT().Save(gomock.Any()).Return("/tmp/s.sh", nil)
				launcher.EXPECT().Launch("/work", "/tmp/s.sh").Return(errors.New("no terminal"))
				scripts.EXPECT().Remove("/tmp/s.sh")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			reqs := mock.NewMockRequestRepository(ctrl)
			dirs := mock.NewMockDirVerifier(ctrl)
			scripts := mock.NewMockScriptStore(ctrl)
			launcher := mock.NewMockLauncher(ctrl)
			tt.setup(t, reqs, dirs, scripts, launcher)

			uc := usecase.NewProcessRequest(reqs, dirs, scripts, launcher, "claude")
			err := uc.Handle(consumed)

			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("Handle() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

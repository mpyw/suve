package diff

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/mpyw/suve/internal/diff"
	"github.com/mpyw/suve/internal/testutil"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSpec1  *wantSpec
		wantSpec2  *wantSpec
		wantErrMsg string
	}{
		// === 1 argument: full spec vs latest ===
		{
			name: "1 arg: version specified",
			args: []string{"/app/config#3"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(3)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},
		{
			name: "1 arg: shift specified",
			args: []string{"/app/config~1"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   1,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},
		{
			name: "1 arg: version and shift",
			args: []string{"/app/config#5~2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(5)),
				shift:   2,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},
		{
			name: "1 arg: no version (latest vs latest)",
			args: []string{"/app/config"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},

		// === 2 arguments: second starts with # or ~ (use name from first) ===
		{
			name: "2 args: name + version spec (partial spec)",
			args: []string{"/app/config", "#3"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(3)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},
		{
			name: "2 args: full spec + version spec (mixed)",
			args: []string{"/app/config#1", "#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: full spec with shift + version spec",
			args: []string{"/app/config~1", "#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   1,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: name + shift spec",
			args: []string{"/app/config", "~"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   1,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},
		{
			name: "2 args: name + shift spec ~2",
			args: []string{"/app/config", "~2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   2,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},

		// === 2 arguments: fullpath x2 ===
		{
			name: "2 args: full spec x2 same key",
			args: []string{"/app/config#1", "/app/config#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: full spec x2 different keys",
			args: []string{"/app/config#1", "/other/key#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/other/key",
				version: testutil.Ptr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: first latest, second versioned",
			args: []string{"/app/config", "/app/config#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: first versioned, second latest",
			args: []string{"/app/config#1", "/app/config"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   0,
			},
		},

		// === 3 arguments: partial spec format ===
		{
			name: "3 args: partial spec format",
			args: []string{"/app/config", "#1", "#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "3 args: partial spec with shifts",
			args: []string{"/app/config", "~1", "~2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   1,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   2,
			},
		},
		{
			name: "3 args: partial spec mixed version and shift",
			args: []string{"/app/config", "#3", "~"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: testutil.Ptr(int64(3)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: nil,
				shift:   1,
			},
		},

		// === Error cases ===
		{
			name:       "0 args: error",
			args:       []string{},
			wantErrMsg: "usage:",
		},
		{
			name:       "4+ args: error",
			args:       []string{"/app/config", "#1", "#2", "#3"},
			wantErrMsg: "usage:",
		},
		{
			name:       "invalid shift spec in 1 arg",
			args:       []string{"/app/config#3~abc"},
			wantErrMsg: "invalid",
		},
		{
			name:       "invalid shift spec in 2nd arg",
			args:       []string{"/app/config", "#3~abc"},
			wantErrMsg: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec1, spec2, err := diff.ParseArgs(
				tt.args,
				ssmversion.Parse,
				func(abs ssmversion.AbsoluteSpec) bool { return abs.Version != nil },
				"#~",
				"usage: suve ssm diff <spec1> [spec2] | <name> <version1> [version2]",
			)

			if tt.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErrMsg)) {
					t.Errorf("expected error containing %q, got %q", tt.wantErrMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertSpec(t, "spec1", spec1, tt.wantSpec1)
			assertSpec(t, "spec2", spec2, tt.wantSpec2)
		})
	}
}

type wantSpec struct {
	name    string
	version *int64
	shift   int
}

func assertSpec(t *testing.T, label string, got *ssmversion.Spec, want *wantSpec) {
	t.Helper()
	if got.Name != want.name {
		t.Errorf("%s.Name = %q, want %q", label, got.Name, want.name)
	}
	if !testutil.PtrEqual(got.Absolute.Version, want.version) {
		t.Errorf("%s.Absolute.Version = %v, want %v", label, got.Absolute.Version, want.version)
	}
	if got.Shift != want.shift {
		t.Errorf("%s.Shift = %d, want %d", label, got.Shift, want.shift)
	}
}

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func TestRun(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		opts    Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "diff between two versions",
			opts: Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					name := aws.ToString(params.Name)
					if name == "/app/param:1" {
						return &ssm.GetParameterOutput{
							Parameter: &types.Parameter{
								Name:             aws.String("/app/param"),
								Value:            aws.String("old-value"),
								Version:          1,
								Type:             types.ParameterTypeString,
								LastModifiedDate: aws.Time(now.Add(-time.Hour)),
							},
						}, nil
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             aws.String("/app/param"),
							Value:            aws.String("new-value"),
							Version:          2,
							Type:             types.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("-old-value")) {
					t.Error("expected '-old-value' in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-value")) {
					t.Error("expected '+new-value' in diff output")
				}
			},
		},
		{
			name: "no diff when same content",
			opts: Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/app/param"),
							Value:   aws.String("same-value"),
							Version: 1,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// No diff lines expected for identical content
				if bytes.Contains([]byte(output), []byte("-same-value")) {
					t.Error("expected no diff for identical content")
				}
			},
		},
		{
			name: "error getting first version",
			opts: Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if aws.ToString(params.Name) == "/app/param:1" {
						return nil, fmt.Errorf("version not found")
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/app/param"),
							Value:   aws.String("value"),
							Version: 2,
						},
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "error getting second version",
			opts: Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: testutil.Ptr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if aws.ToString(params.Name) == "/app/param:2" {
						return nil, fmt.Errorf("version not found")
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/app/param"),
							Value:   aws.String("value"),
							Version: 1,
						},
					}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf, errBuf bytes.Buffer
			r := &Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

func TestRun_IdenticalWarning(t *testing.T) {
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    aws.String("/app/param"),
					Value:   aws.String("same-value"),
					Version: 1,
					Type:    types.ParameterTypeString,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &Runner{
		Client: mock,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	opts := Options{
		Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{}},
		Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{}},
	}

	err := r.Run(t.Context(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stdout should be empty (no diff output)
	if stdout.String() != "" {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}

	// stderr should contain warning and hint
	stderrStr := stderr.String()
	if !bytes.Contains([]byte(stderrStr), []byte("Warning:")) {
		t.Error("expected warning message in stderr")
	}
	if !bytes.Contains([]byte(stderrStr), []byte("comparing identical versions")) {
		t.Error("expected 'comparing identical versions' in stderr")
	}
	if !bytes.Contains([]byte(stderrStr), []byte("Hint:")) {
		t.Error("expected hint message in stderr")
	}
	if !bytes.Contains([]byte(stderrStr), []byte("/app/param~1")) {
		t.Error("expected hint with '~1' in stderr")
	}
}

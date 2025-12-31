package diff_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	ssmdiff "github.com/mpyw/suve/internal/cli/ssm/diff"
	"github.com/mpyw/suve/internal/diff"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "diff"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "diff", "/app/param#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})

	t.Run("too many arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "diff", "/a", "#1", "#2", "#3"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestParseArgs(t *testing.T) {
	t.Parallel()
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
				version: lo.ToPtr(int64(3)),
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
				version: lo.ToPtr(int64(5)),
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
				version: lo.ToPtr(int64(3)),
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
				version: lo.ToPtr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: lo.ToPtr(int64(2)),
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
				version: lo.ToPtr(int64(2)),
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
				version: lo.ToPtr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: lo.ToPtr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: full spec x2 different keys",
			args: []string{"/app/config#1", "/other/key#2"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: lo.ToPtr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/other/key",
				version: lo.ToPtr(int64(2)),
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
				version: lo.ToPtr(int64(2)),
				shift:   0,
			},
		},
		{
			name: "2 args: first versioned, second latest",
			args: []string{"/app/config#1", "/app/config"},
			wantSpec1: &wantSpec{
				name:    "/app/config",
				version: lo.ToPtr(int64(1)),
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
				version: lo.ToPtr(int64(1)),
				shift:   0,
			},
			wantSpec2: &wantSpec{
				name:    "/app/config",
				version: lo.ToPtr(int64(2)),
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
				version: lo.ToPtr(int64(3)),
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
		{
			name:       "2 args: invalid first arg",
			args:       []string{"#", "/app/config#2"},
			wantErrMsg: "invalid first argument",
		},
		{
			name:       "2 args: invalid second arg (full spec x2)",
			args:       []string{"/app/config#1", "/app/config#"},
			wantErrMsg: "invalid second argument",
		},
		{
			name:       "3 args: invalid version1",
			args:       []string{"/app/config", "#", "#2"},
			wantErrMsg: "invalid version1",
		},
		{
			name:       "3 args: invalid version2",
			args:       []string{"/app/config", "#1", "#"},
			wantErrMsg: "invalid version2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec1, spec2, err := diff.ParseArgs(
				tt.args,
				ssmversion.Parse,
				func(abs ssmversion.AbsoluteSpec) bool { return abs.Version != nil },
				"#~",
				"usage: suve ssm diff <spec1> [spec2] | <name> <version1> [version2]",
			)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
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
	assert.Equal(t, want.name, got.Name, "%s.Name", label)
	assert.Equal(t, want.version, got.Absolute.Version, "%s.Absolute.Version", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
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
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		opts    ssmdiff.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "diff between two versions",
			opts: ssmdiff.Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					name := lo.FromPtr(params.Name)
					if name == "/app/param:1" {
						return &ssm.GetParameterOutput{
							Parameter: &types.Parameter{
								Name:             lo.ToPtr("/app/param"),
								Value:            lo.ToPtr("old-value"),
								Version:          1,
								Type:             types.ParameterTypeString,
								LastModifiedDate: lo.ToPtr(now.Add(-time.Hour)),
							},
						}, nil
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             lo.ToPtr("/app/param"),
							Value:            lo.ToPtr("new-value"),
							Version:          2,
							Type:             types.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
			},
		},
		{
			name: "no diff when same content",
			opts: ssmdiff.Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("same-value"),
							Version: 1,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// No diff lines expected for identical content
				assert.NotContains(t, output, "-same-value")
			},
		},
		{
			name: "error getting first version",
			opts: ssmdiff.Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if lo.FromPtr(params.Name) == "/app/param:1" {
						return nil, fmt.Errorf("version not found")
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("value"),
							Version: 2,
						},
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "error getting second version",
			opts: ssmdiff.Options{
				Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if lo.FromPtr(params.Name) == "/app/param:2" {
						return nil, fmt.Errorf("version not found")
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("value"),
							Version: 1,
						},
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "json format with valid JSON values",
			opts: ssmdiff.Options{
				Spec1:      &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2:      &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					name := lo.FromPtr(params.Name)
					if name == "/app/param:1" {
						return &ssm.GetParameterOutput{
							Parameter: &types.Parameter{
								Name:    lo.ToPtr("/app/param"),
								Value:   lo.ToPtr(`{"key":"old"}`),
								Version: 1,
								Type:    types.ParameterTypeString,
							},
						}, nil
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr(`{"key":"new"}`),
							Version: 2,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-")
				assert.Contains(t, output, "+")
			},
		},
		{
			name: "json format with non-JSON values warns",
			opts: ssmdiff.Options{
				Spec1:      &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2:      &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
				JSONFormat: true,
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					name := lo.FromPtr(params.Name)
					if name == "/app/param:1" {
						return &ssm.GetParameterOutput{
							Parameter: &types.Parameter{
								Name:    lo.ToPtr("/app/param"),
								Value:   lo.ToPtr("not json"),
								Version: 1,
								Type:    types.ParameterTypeString,
							},
						}, nil
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("also not json"),
							Version: 2,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &ssmdiff.Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

func TestRun_IdenticalWarning(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/app/param"),
					Value:   lo.ToPtr("same-value"),
					Version: 1,
					Type:    types.ParameterTypeString,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &ssmdiff.Runner{
		Client: mock,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	opts := ssmdiff.Options{
		Spec1: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{}},
		Spec2: &ssmversion.Spec{Name: "/app/param", Absolute: ssmversion.AbsoluteSpec{}},
	}

	err := r.Run(t.Context(), opts)
	require.NoError(t, err)

	// stdout should be empty (no diff output)
	assert.Empty(t, stdout.String())

	// stderr should contain warning and hint
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Warning:")
	assert.Contains(t, stderrStr, "comparing identical versions")
	assert.Contains(t, stderrStr, "Hint:")
	assert.Contains(t, stderrStr, "/app/param~1")
}

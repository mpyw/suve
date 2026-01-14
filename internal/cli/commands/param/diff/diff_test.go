package diff_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	paramdiff "github.com/mpyw/suve/internal/cli/commands/param/diff"
	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

const testParamVersion1 = "/app/param:1"

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "diff"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "diff", "/app/param#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})

	t.Run("too many arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "diff", "/a", "#1", "#2", "#3"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

//nolint:funlen // Table-driven test with many cases
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

			spec1, spec2, err := diffargs.ParseArgs(
				tt.args,
				paramversion.Parse,
				func(abs paramversion.AbsoluteSpec) bool { return abs.Version != nil },
				"#~",
				"usage: suve param diff <spec1> [spec2] | <name> <version1> [version2]",
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

func assertSpec(t *testing.T, label string, got *paramversion.Spec, want *wantSpec) {
	t.Helper()
	assert.Equal(t, want.name, got.Name, "%s.Name", label)
	assert.Equal(t, want.version, got.Absolute.Version, "%s.Absolute.Version", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

//nolint:lll // mock struct fields match AWS SDK interface signatures
type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error)
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("GetParameter not mocked")
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockClient) GetParameterHistory(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

//nolint:funlen // Table-driven test with many cases
func TestRun(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		opts    paramdiff.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "diff between two versions",
			opts: paramdiff.Options{
				Spec1: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				//nolint:lll // inline mock function in test table
				getParameterFunc: func(_ context.Context, params *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					name := lo.FromPtr(params.Name)
					if name == testParamVersion1 {
						return &paramapi.GetParameterOutput{
							Parameter: &paramapi.Parameter{
								Name:             lo.ToPtr("/app/param"),
								Value:            lo.ToPtr("old-value"),
								Version:          1,
								Type:             paramapi.ParameterTypeString,
								LastModifiedDate: lo.ToPtr(now.Add(-time.Hour)),
							},
						}, nil
					}

					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:             lo.ToPtr("/app/param"),
							Value:            lo.ToPtr("new-value"),
							Version:          2,
							Type:             paramapi.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
			},
		},
		{
			name: "no diff when same content",
			opts: paramdiff.Options{
				Spec1: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("same-value"),
							Version: 1,
							Type:    paramapi.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// No diff lines expected for identical content
				assert.NotContains(t, output, "-same-value")
			},
		},
		{
			name: "error getting first version",
			opts: paramdiff.Options{
				Spec1: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				//nolint:lll // inline mock function in test table
				getParameterFunc: func(_ context.Context, params *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					if lo.FromPtr(params.Name) == testParamVersion1 {
						return nil, fmt.Errorf("version not found")
					}

					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
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
			opts: paramdiff.Options{
				Spec1: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
			},
			mock: &mockClient{
				//nolint:lll // inline mock function in test table
				getParameterFunc: func(_ context.Context, params *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					if lo.FromPtr(params.Name) == "/app/param:2" {
						return nil, fmt.Errorf("version not found")
					}

					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
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
			opts: paramdiff.Options{
				Spec1:     &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2:     &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
				ParseJSON: true,
			},
			mock: &mockClient{
				//nolint:lll // mock function signature
				getParameterFunc: func(_ context.Context, params *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					name := lo.FromPtr(params.Name)
					if name == testParamVersion1 {
						return &paramapi.GetParameterOutput{
							Parameter: &paramapi.Parameter{
								Name:    lo.ToPtr("/app/param"),
								Value:   lo.ToPtr(`{"key":"old"}`),
								Version: 1,
								Type:    paramapi.ParameterTypeString,
							},
						}, nil
					}

					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr(`{"key":"new"}`),
							Version: 2,
							Type:    paramapi.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-")
				assert.Contains(t, output, "+")
			},
		},
		{
			name: "json format with non-JSON values warns",
			opts: paramdiff.Options{
				Spec1:     &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}},
				Spec2:     &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))}},
				ParseJSON: true,
			},
			mock: &mockClient{
				//nolint:lll // mock function signature
				getParameterFunc: func(_ context.Context, params *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					name := lo.FromPtr(params.Name)
					if name == testParamVersion1 {
						return &paramapi.GetParameterOutput{
							Parameter: &paramapi.Parameter{
								Name:    lo.ToPtr("/app/param"),
								Value:   lo.ToPtr("not json"),
								Version: 1,
								Type:    paramapi.ParameterTypeString,
							},
						}, nil
					}

					return &paramapi.GetParameterOutput{
						Parameter: &paramapi.Parameter{
							Name:    lo.ToPtr("/app/param"),
							Value:   lo.ToPtr("also not json"),
							Version: 2,
							Type:    paramapi.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &paramdiff.Runner{
				UseCase: &param.DiffUseCase{Client: tt.mock},
				Stdout:  &buf,
				Stderr:  &errBuf,
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
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/param"),
					Value:   lo.ToPtr("same-value"),
					Version: 1,
					Type:    paramapi.ParameterTypeString,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer

	r := &paramdiff.Runner{
		UseCase: &param.DiffUseCase{Client: mock},
		Stdout:  &stdout,
		Stderr:  &stderr,
	}
	opts := paramdiff.Options{
		Spec1: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{}},
		Spec2: &paramversion.Spec{Name: "/app/param", Absolute: paramversion.AbsoluteSpec{}},
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

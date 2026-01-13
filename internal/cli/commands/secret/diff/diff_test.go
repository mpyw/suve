package diff_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	secretdiff "github.com/mpyw/suve/internal/cli/commands/secret/diff"
	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

const testStageLabelPrevious = "AWSPREVIOUS"

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "diff"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "diff", "my-secret#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})

	t.Run("invalid label spec", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "diff", "my-secret:"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})

	t.Run("too many arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "diff", "my-secret", ":AWSPREVIOUS", ":AWSCURRENT", ":extra"})
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
		// === 1 argument: full spec vs AWSCURRENT ===
		{
			name: "1 arg: label specified",
			args: []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "1 arg: version ID specified",
			args: []string{"my-secret#abc123"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "1 arg: shift specified",
			args: []string{"my-secret~1"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      1,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "1 arg: no specifier (AWSCURRENT vs AWSCURRENT)",
			args: []string{"my-secret"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},

		// === 2 arguments: second starts with #/:/ ~ (use name from first) ===
		{
			name: "2 args: name + label spec (partial spec)",
			args: []string{"my-secret", ":AWSPREVIOUS"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "2 args: name + version ID spec",
			args: []string{"my-secret", "#abc123"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "2 args: full spec + label spec (mixed)",
			args: []string{"my-secret:AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSCURRENT"),
				shift:      0,
			},
		},
		{
			name: "2 args: full spec#id + #id spec (mixed)",
			args: []string{"my-secret#abc123", "#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("def456"),
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "2 args: name + shift spec",
			args: []string{"my-secret", "~"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      1,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
		},

		// === 2 arguments: fullpath x2 ===
		{
			name: "2 args: full spec x2 same key with labels",
			args: []string{"my-secret:AWSPREVIOUS", "my-secret:AWSCURRENT"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSCURRENT"),
				shift:      0,
			},
		},
		{
			name: "2 args: full spec x2 same key with IDs",
			args: []string{"my-secret#abc123", "my-secret#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("def456"),
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "2 args: full spec x2 different keys",
			args: []string{"my-secret#abc123", "other-secret#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "other-secret",
				id:         lo.ToPtr("def456"),
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "2 args: first latest, second versioned",
			args: []string{"my-secret", "my-secret#abc123"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
		},

		// === 3 arguments: partial spec format ===
		{
			name: "3 args: partial spec format with labels",
			args: []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSCURRENT"),
				shift:      0,
			},
		},
		{
			name: "3 args: partial spec format with IDs",
			args: []string{"my-secret", "#abc123", "#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         lo.ToPtr("def456"),
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "3 args: partial spec mixed label and shift",
			args: []string{"my-secret", ":AWSPREVIOUS", "~"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      lo.ToPtr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      nil,
				shift:      1,
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
			args:       []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT", ":extra"},
			wantErrMsg: "usage:",
		},
		{
			name:       "invalid label in 1 arg",
			args:       []string{"my-secret:"},
			wantErrMsg: "invalid",
		},
		{
			name:       "invalid version ID in 2nd arg",
			args:       []string{"my-secret", "#"},
			wantErrMsg: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec1, spec2, err := diffargs.ParseArgs(
				tt.args,
				secretversion.Parse,
				func(abs secretversion.AbsoluteSpec) bool { return abs.ID != nil || abs.Label != nil },
				"#:~",
				"usage: suve secret diff <spec1> [spec2] | <name> <version1> [version2]",
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
	secretName string
	id         *string
	label      *string
	shift      int
}

func assertSpec(t *testing.T, label string, got *secretversion.Spec, want *wantSpec) {
	t.Helper()
	assert.Equal(t, want.secretName, got.Name, "%s.Name", label)
	assert.Equal(t, want.id, got.Absolute.ID, "%s.Absolute.ID", label)
	assert.Equal(t, want.label, got.Absolute.Label, "%s.Absolute.Label", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretapi.ListSecretVersionIdsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretapi.ListSecretVersionIdsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestRunnerRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    secretdiff.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "diff between two versions",
			opts: secretdiff.Options{
				Spec1: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
				Spec2: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}},
			},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					stage := lo.FromPtr(params.VersionStage)
					if stage == testStageLabelPrevious {
						return &secretapi.GetSecretValueOutput{
							Name:         lo.ToPtr("my-secret"),
							VersionId:    lo.ToPtr("prev-version-id-long"),
							SecretString: lo.ToPtr("old-secret"),
						}, nil
					}
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("curr-version-id-long"),
						SecretString: lo.ToPtr("new-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old-secret")
				assert.Contains(t, output, "+new-secret")
			},
		},
		{
			name: "short version IDs not truncated",
			opts: secretdiff.Options{
				Spec1: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
				Spec2: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}},
			},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					stage := lo.FromPtr(params.VersionStage)
					if stage == testStageLabelPrevious {
						return &secretapi.GetSecretValueOutput{
							Name:         lo.ToPtr("my-secret"),
							VersionId:    lo.ToPtr("v1"),
							SecretString: lo.ToPtr("old"),
						}, nil
					}
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("v2"),
						SecretString: lo.ToPtr("new"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old")
				assert.Contains(t, output, "+new")
			},
		},
		{
			name: "error getting first version",
			opts: secretdiff.Options{
				Spec1: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
				Spec2: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}},
			},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					if lo.FromPtr(params.VersionStage) == testStageLabelPrevious {
						return nil, fmt.Errorf("version not found")
					}
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("v1"),
						SecretString: lo.ToPtr("secret"),
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "error getting second version",
			opts: secretdiff.Options{
				Spec1: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
				Spec2: &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}},
			},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					if lo.FromPtr(params.VersionStage) == "AWSCURRENT" {
						return nil, fmt.Errorf("version not found")
					}
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("v1"),
						SecretString: lo.ToPtr("secret"),
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "json format with valid JSON values",
			opts: secretdiff.Options{
				Spec1:     &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
				Spec2:     &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}},
				ParseJSON: true,
			},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					stage := lo.FromPtr(params.VersionStage)
					if stage == testStageLabelPrevious {
						return &secretapi.GetSecretValueOutput{
							Name:         lo.ToPtr("my-secret"),
							VersionId:    lo.ToPtr("v1-longer-id"),
							SecretString: lo.ToPtr(`{"key":"old"}`),
						}, nil
					}
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("v2-longer-id"),
						SecretString: lo.ToPtr(`{"key":"new"}`),
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
			opts: secretdiff.Options{
				Spec1:     &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
				Spec2:     &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}},
				ParseJSON: true,
			},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					stage := lo.FromPtr(params.VersionStage)
					if stage == testStageLabelPrevious {
						return &secretapi.GetSecretValueOutput{
							Name:         lo.ToPtr("my-secret"),
							VersionId:    lo.ToPtr("v1-longer-id"),
							SecretString: lo.ToPtr("not json"),
						}, nil
					}
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("v2-longer-id"),
						SecretString: lo.ToPtr("also not json"),
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
			var stdout, stderr bytes.Buffer
			r := &secretdiff.Runner{
				UseCase: &secret.DiffUseCase{Client: tt.mock},
				Stdout:  &stdout,
				Stderr:  &stderr,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, stdout.String())
			}
		})
	}
}

func TestRun_IdenticalWarning(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				VersionId:    lo.ToPtr("version-id"),
				SecretString: lo.ToPtr("same-content"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &secretdiff.Runner{
		UseCase: &secret.DiffUseCase{Client: mock},
		Stdout:  &stdout,
		Stderr:  &stderr,
	}
	opts := secretdiff.Options{
		Spec1:     &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{}},
		Spec2:     &secretversion.Spec{Name: "my-secret", Absolute: secretversion.AbsoluteSpec{}},
		ParseJSON: false,
	}

	err := r.Run(t.Context(), opts)
	require.NoError(t, err)

	// stdout should be empty (no diff output)
	assert.Empty(t, stdout.String())

	// stderr should contain warning and hints
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Warning:")
	assert.Contains(t, stderrStr, "comparing identical versions")
	assert.Contains(t, stderrStr, "Hint:")
	assert.Contains(t, stderrStr, "my-secret~1")
	assert.Contains(t, stderrStr, "my-secret:AWSPREVIOUS")
}

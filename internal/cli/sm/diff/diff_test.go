package diff

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/diff"
	"github.com/mpyw/suve/internal/version/smversion"
)

func TestParseArgs(t *testing.T) {
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
			spec1, spec2, err := diff.ParseArgs(
				tt.args,
				smversion.Parse,
				func(abs smversion.AbsoluteSpec) bool { return abs.ID != nil || abs.Label != nil },
				"#:~",
				"usage: suve sm diff <spec1> [spec2] | <name> <version1> [version2]",
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

func assertSpec(t *testing.T, label string, got *smversion.Spec, want *wantSpec) {
	t.Helper()
	assert.Equal(t, want.secretName, got.Name, "%s.Name", label)
	assert.Equal(t, want.id, got.Absolute.ID, "%s.Absolute.ID", label)
	assert.Equal(t, want.label, got.Absolute.Label, "%s.Absolute.Label", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		secretName string
		version1   string
		version2   string
		mock       *mockClient
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name:       "diff between two versions",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					stage := aws.ToString(params.VersionStage)
					if stage == "AWSPREVIOUS" {
						return &secretsmanager.GetSecretValueOutput{
							Name:         aws.String("my-secret"),
							VersionId:    aws.String("prev-version-id"),
							SecretString: aws.String("old-secret"),
						}, nil
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("curr-version-id"),
						SecretString: aws.String("new-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old-secret")
				assert.Contains(t, output, "+new-secret")
			},
		},
		{
			name:       "no diff when same content",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("version-id"),
						SecretString: aws.String("same-content"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.NotContains(t, output, "-same-content")
			},
		},
		{
			name:       "short version id",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					stage := aws.ToString(params.VersionStage)
					if stage == "AWSPREVIOUS" {
						return &secretsmanager.GetSecretValueOutput{
							Name:         aws.String("my-secret"),
							VersionId:    aws.String("v1"), // Short version ID
							SecretString: aws.String("old"),
						}, nil
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("v2"), // Short version ID
						SecretString: aws.String("new"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Should not panic with short version IDs
				assert.Contains(t, output, "-old")
				assert.Contains(t, output, "+new")
			},
		},
		{
			name:       "error getting first version",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					if aws.ToString(params.VersionStage) == "AWSPREVIOUS" {
						return nil, fmt.Errorf("version not found")
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("version-id"),
						SecretString: aws.String("secret"),
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name:       "error getting second version",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					if aws.ToString(params.VersionStage) == "AWSCURRENT" {
						return nil, fmt.Errorf("version not found")
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("version-id"),
						SecretString: aws.String("secret"),
					}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, tt.secretName, tt.version1, tt.version2)

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
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				VersionId:    aws.String("version-id"),
				SecretString: aws.String("same-content"),
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
		Spec1:      &smversion.Spec{Name: "my-secret", Absolute: smversion.AbsoluteSpec{}},
		Spec2:      &smversion.Spec{Name: "my-secret", Absolute: smversion.AbsoluteSpec{}},
		JSONFormat: false,
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

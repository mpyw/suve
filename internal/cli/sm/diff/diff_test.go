package diff

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/mpyw/suve/internal/testutil"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSpec1  *wantSpec
		wantSpec2  *wantSpec
		wantErrMsg string
	}{
		// === 1 argument: fullspec vs AWSCURRENT ===
		{
			name: "1 arg: label specified",
			args: []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSPREVIOUS"),
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
				id:         testutil.Ptr("abc123"),
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
			name: "2 args: name + label spec (legacy with omission)",
			args: []string{"my-secret", ":AWSPREVIOUS"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSPREVIOUS"),
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
				id:         testutil.Ptr("abc123"),
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
			name: "2 args: fullspec + label spec (mixed)",
			args: []string{"my-secret:AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSCURRENT"),
				shift:      0,
			},
		},
		{
			name: "2 args: fullspec#id + #id spec (mixed)",
			args: []string{"my-secret#abc123", "#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("def456"),
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
			name: "2 args: fullspec x2 same key with labels",
			args: []string{"my-secret:AWSPREVIOUS", "my-secret:AWSCURRENT"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSCURRENT"),
				shift:      0,
			},
		},
		{
			name: "2 args: fullspec x2 same key with IDs",
			args: []string{"my-secret#abc123", "my-secret#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("def456"),
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "2 args: fullspec x2 different keys",
			args: []string{"my-secret#abc123", "other-secret#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "other-secret",
				id:         testutil.Ptr("def456"),
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
				id:         testutil.Ptr("abc123"),
				label:      nil,
				shift:      0,
			},
		},

		// === 3 arguments: legacy format ===
		{
			name: "3 args: legacy format with labels",
			args: []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSPREVIOUS"),
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSCURRENT"),
				shift:      0,
			},
		},
		{
			name: "3 args: legacy format with IDs",
			args: []string{"my-secret", "#abc123", "#def456"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("abc123"),
				label:      nil,
				shift:      0,
			},
			wantSpec2: &wantSpec{
				secretName: "my-secret",
				id:         testutil.Ptr("def456"),
				label:      nil,
				shift:      0,
			},
		},
		{
			name: "3 args: legacy mixed label and shift",
			args: []string{"my-secret", ":AWSPREVIOUS", "~"},
			wantSpec1: &wantSpec{
				secretName: "my-secret",
				id:         nil,
				label:      testutil.Ptr("AWSPREVIOUS"),
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
			spec1, spec2, err := ParseArgs(tt.args)

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
	secretName string
	id         *string
	label      *string
	shift      int
}

func assertSpec(t *testing.T, name string, got *ParsedSpec, want *wantSpec) {
	t.Helper()
	if got.Name != want.secretName {
		t.Errorf("%s.Name = %q, want %q", name, got.Name, want.secretName)
	}
	if !testutil.PtrEqual(got.ID, want.id) {
		t.Errorf("%s.ID = %v, want %v", name, got.ID, want.id)
	}
	if !testutil.PtrEqual(got.Label, want.label) {
		t.Errorf("%s.Label = %v, want %v", name, got.Label, want.label)
	}
	if got.Shift != want.shift {
		t.Errorf("%s.Shift = %d, want %d", name, got.Shift, want.shift)
	}
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
				if !bytes.Contains([]byte(output), []byte("-old-secret")) {
					t.Error("expected '-old-secret' in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-secret")) {
					t.Error("expected '+new-secret' in diff output")
				}
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
				if bytes.Contains([]byte(output), []byte("-same-content")) {
					t.Error("expected no diff for identical content")
				}
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
				if !bytes.Contains([]byte(output), []byte("-old")) {
					t.Error("expected '-old' in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new")) {
					t.Error("expected '+new' in diff output")
				}
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

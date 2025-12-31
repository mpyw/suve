package log

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type mockClient struct {
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func TestRun(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		secretName string
		opts       Options
		mock       *mockClient
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name:       "show version history",
			secretName: "my-secret",
			opts:       Options{MaxResults: 10},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					if aws.ToString(params.SecretId) != "my-secret" {
						t.Errorf("expected SecretId my-secret, got %s", aws.ToString(params.SecretId))
					}
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: aws.String("v2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Version")) {
					t.Error("expected 'Version' in output")
				}
				if !bytes.Contains([]byte(output), []byte("AWSCURRENT")) {
					t.Error("expected AWSCURRENT stage in output")
				}
			},
		},
		{
			name:       "show patch between versions",
			secretName: "my-secret",
			opts:       Options{MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("version-id-1"), CreatedDate: aws.Time(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: aws.String("version-id-2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					versionID := aws.ToString(params.VersionId)
					switch versionID {
					case "version-id-1":
						return &secretsmanager.GetSecretValueOutput{
							SecretString: aws.String("old-value"),
							VersionId:    aws.String("version-id-1"),
						}, nil
					case "version-id-2":
						return &secretsmanager.GetSecretValueOutput{
							SecretString: aws.String("new-value"),
							VersionId:    aws.String("version-id-2"),
						}, nil
					}
					return nil, fmt.Errorf("unknown version")
				},
			},
			check: func(t *testing.T, output string) {
				// Should contain diff markers
				if !bytes.Contains([]byte(output), []byte("-old-value")) {
					t.Error("expected -old-value in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-value")) {
					t.Error("expected +new-value in diff output")
				}
				// Should contain truncated version IDs in headers
				if !bytes.Contains([]byte(output), []byte("my-secret#version-")) {
					t.Error("expected version ID in diff output")
				}
			},
		},
		{
			name:       "patch with single version shows no diff",
			secretName: "my-secret",
			opts:       Options{MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("only-version"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						SecretString: aws.String("only-value"),
						VersionId:    aws.String("only-version"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Should contain version info but no diff markers
				if !bytes.Contains([]byte(output), []byte("Version")) {
					t.Error("expected Version in output")
				}
				// No diff markers for single version
				if bytes.Contains([]byte(output), []byte("---")) {
					t.Error("expected no diff markers for single version")
				}
			},
		},
		{
			name:       "error from AWS",
			secretName: "my-secret",
			opts:       Options{MaxResults: 10},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name:       "reverse order shows oldest first",
			secretName: "my-secret",
			opts:       Options{MaxResults: 10, Reverse: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("version-1"), CreatedDate: aws.Time(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: aws.String("version-2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// In reverse mode, AWSPREVIOUS should appear before AWSCURRENT
				prevPos := bytes.Index([]byte(output), []byte("AWSPREVIOUS"))
				currPos := bytes.Index([]byte(output), []byte("AWSCURRENT"))
				if prevPos < 0 || currPos < 0 {
					t.Error("expected both version stages in output")
				}
				if prevPos > currPos {
					t.Error("expected AWSPREVIOUS before AWSCURRENT in reverse mode")
				}
			},
		},
		{
			name:       "empty version list",
			secretName: "my-secret",
			opts:       Options{MaxResults: 10},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if output != "" {
					t.Errorf("expected empty output for empty version list, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf, errBuf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, &errBuf, tt.secretName, tt.opts)

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

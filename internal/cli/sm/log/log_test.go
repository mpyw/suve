package log_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/sm/log"
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
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		opts    log.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show version history",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("v2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version")
				assert.Contains(t, output, "AWSCURRENT")
			},
		},
		{
			name: "show patch between versions",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("version-id-1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("version-id-2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					versionID := lo.FromPtr(params.VersionId)
					switch versionID {
					case "version-id-1":
						return &secretsmanager.GetSecretValueOutput{
							SecretString: lo.ToPtr("old-value"),
							VersionId:    lo.ToPtr("version-id-1"),
						}, nil
					case "version-id-2":
						return &secretsmanager.GetSecretValueOutput{
							SecretString: lo.ToPtr("new-value"),
							VersionId:    lo.ToPtr("version-id-2"),
						}, nil
					}
					return nil, fmt.Errorf("unknown version")
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
				assert.Contains(t, output, "my-secret#version-")
			},
		},
		{
			name: "patch with single version shows no diff",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("only-version"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						SecretString: lo.ToPtr("only-value"),
						VersionId:    lo.ToPtr("only-version"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "error from AWS",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("version-1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("version-2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				prevPos := bytes.Index([]byte(output), []byte("AWSPREVIOUS"))
				currPos := bytes.Index([]byte(output), []byte("AWSCURRENT"))
				require.NotEqual(t, -1, prevPos, "expected AWSPREVIOUS in output")
				require.NotEqual(t, -1, currPos, "expected AWSCURRENT in output")
				assert.Less(t, prevPos, currPos, "expected AWSPREVIOUS before AWSCURRENT in reverse mode")
			},
		},
		{
			name: "empty version list",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Empty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &log.Runner{
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

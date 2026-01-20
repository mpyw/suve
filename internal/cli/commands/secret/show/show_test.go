package show_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/show"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "show"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve secret show")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "show", "my-secret#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})
}

type mockClient struct {
	getSecretResult *model.Secret
	getSecretErr    error
	versionsResult  []*model.SecretVersion
	versionsErr     error
	describeResult  *model.SecretListItem
	describeErr     error
	tagsResult      map[string]string
	tagsErr         error
}

func (m *mockClient) GetSecret(_ context.Context, _, _, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		return nil, m.getSecretErr
	}

	return m.getSecretResult, nil
}

func (m *mockClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	if m.versionsErr != nil {
		return nil, m.versionsErr
	}

	return m.versionsResult, nil
}

func (m *mockClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	return nil, errors.New("not implemented")
}

func (m *mockClient) DescribeSecret(_ context.Context, _ string) (*model.SecretListItem, error) {
	if m.describeErr != nil {
		return nil, m.describeErr
	}

	return m.describeResult, nil
}

func (m *mockClient) GetTags(_ context.Context, _ string) (map[string]string, error) {
	if m.tagsErr != nil {
		return nil, m.tagsErr
	}

	return m.tagsResult, nil
}

func (m *mockClient) AddTags(_ context.Context, _ string, _ map[string]string) error {
	return errors.New("not implemented")
}

func (m *mockClient) RemoveTags(_ context.Context, _ string, _ []string) error {
	return errors.New("not implemented")
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

//nolint:funlen // Table-driven test with many cases
func TestRun(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		opts    show.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:      "my-secret",
					Value:     "secret-value",
					Version:   "abc123",
					CreatedAt: ptrTime(now),
					Metadata: model.AWSSecretMeta{
						ARN:           "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
						VersionStages: []string{"AWSCURRENT"},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "my-secret")
				assert.Contains(t, output, "secret-value")
			},
		},
		{
			name: "show with shift",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret", Shift: 1}},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: ptrTime(now.Add(-2 * time.Hour))},
					{Version: "v2", CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: "v3", CreatedAt: ptrTime(now)},
				},
				getSecretResult: &model.Secret{
					Name:    "my-secret",
					Value:   "previous-value",
					Version: "v2",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "previous-value")
			},
		},
		{
			name: "show JSON formatted with sorted keys",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:      "my-secret",
					Value:     `{"zebra":"last","apple":"first"}`,
					Version:   "abc123",
					CreatedAt: ptrTime(now),
					Metadata: model.AWSSecretMeta{
						ARN:           "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
						VersionStages: []string{"AWSCURRENT"},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

				appleIdx := strings.Index(output, "apple")
				zebraIdx := strings.Index(output, "zebra")

				require.NotEqual(t, -1, appleIdx, "expected apple in output")
				require.NotEqual(t, -1, zebraIdx, "expected zebra in output")
				assert.Less(t, appleIdx, zebraIdx, "expected keys to be sorted (apple before zebra)")
			},
		},
		{
			name: "error from AWS",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretErr: errors.New("AWS error"),
			},
			wantErr: true,
		},
		{
			name: "show without optional fields",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:  "my-secret",
					Value: "secret-value",
					Metadata: model.AWSSecretMeta{
						ARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "my-secret")
				assert.NotContains(t, output, "VersionId")
				assert.NotContains(t, output, "Stages")
				assert.NotContains(t, output, "Created")
			},
		},
		{
			name: "json flag with non-JSON value warns",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:  "my-secret",
					Value: "not json",
					Metadata: model.AWSSecretMeta{
						ARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "not json")
			},
		},
		{
			name: "raw mode outputs only value",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Raw: true},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:  "my-secret",
					Value: "raw-secret-value",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Equal(t, "raw-secret-value", output)
			},
		},
		{
			name: "raw mode with shift",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret", Shift: 1}, Raw: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: "v2", CreatedAt: ptrTime(now)},
				},
				getSecretResult: &model.Secret{
					Name:  "my-secret",
					Value: "previous-value",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Equal(t, "previous-value", output)
			},
		},
		{
			name: "raw mode with JSON formatting",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true, Raw: true},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:  "my-secret",
					Value: `{"zebra":"last","apple":"first"}`,
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

				appleIdx := strings.Index(output, "apple")
				zebraIdx := strings.Index(output, "zebra")

				require.NotEqual(t, -1, appleIdx, "expected apple in output")
				require.NotEqual(t, -1, zebraIdx, "expected zebra in output")
				assert.Less(t, appleIdx, zebraIdx, "expected keys to be sorted (apple before zebra)")
			},
		},
		{
			name: "show with tags",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:      "my-secret",
					Value:     "secret-value",
					Version:   "abc123",
					CreatedAt: ptrTime(now),
					Metadata: model.AWSSecretMeta{
						ARN:           "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
						VersionStages: []string{"AWSCURRENT"},
					},
				},
				tagsResult: map[string]string{
					"Environment": "production",
					"Team":        "backend",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tags")
				assert.Contains(t, output, "2 tag(s)")
				assert.Contains(t, output, "Environment")
				assert.Contains(t, output, "production")
				assert.Contains(t, output, "Team")
				assert.Contains(t, output, "backend")
			},
		},
		{
			name: "show with tags in JSON output",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Output: output.FormatJSON},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:      "my-secret",
					Value:     "secret-value",
					Version:   "abc123",
					CreatedAt: ptrTime(now),
					Metadata: model.AWSSecretMeta{
						ARN:           "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
						VersionStages: []string{"AWSCURRENT"},
					},
				},
				tagsResult: map[string]string{
					"Environment": "production",
					"Team":        "backend",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"tags"`)
				assert.Contains(t, output, `"Environment"`)
				assert.Contains(t, output, `"production"`)
				assert.Contains(t, output, `"Team"`)
				assert.Contains(t, output, `"backend"`)
			},
		},
		{
			name: "JSON output with empty tags shows empty object",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Output: output.FormatJSON},
			mock: &mockClient{
				getSecretResult: &model.Secret{
					Name:  "my-secret",
					Value: "secret-value",
					Metadata: model.AWSSecretMeta{
						ARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf",
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"tags": {}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &show.Runner{
				UseCase: &secret.ShowUseCase{Client: tt.mock},
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

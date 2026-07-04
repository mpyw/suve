package secret_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/secret"
)

// mockClient is a configurable mock of the narrow Secrets Manager interface.
type mockClient struct {
	getValue    func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error)
	listVersion func(*secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error)
	describe    func(*secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error)
	create      func(*secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error)
	putValue    func(*secretsmanager.PutSecretValueInput) (*secretsmanager.PutSecretValueOutput, error)
	deleteSec   func(*secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error)
	restore     func(*secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error)
	tag         func(*secretsmanager.TagResourceInput) (*secretsmanager.TagResourceOutput, error)
	untag       func(*secretsmanager.UntagResourceInput) (*secretsmanager.UntagResourceOutput, error)
	listSecrets func(*secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error)
}

func (m *mockClient) GetSecretValue(
	_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.GetSecretValueOutput, error) {
	return m.getValue(in)
}

//nolint:revive // Method name matches AWS SDK interface naming convention
func (m *mockClient) ListSecretVersionIds(
	_ context.Context, in *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	return m.listVersion(in)
}

func (m *mockClient) DescribeSecret(
	_ context.Context, in *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.DescribeSecretOutput, error) {
	return m.describe(in)
}

func (m *mockClient) CreateSecret(
	_ context.Context, in *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.CreateSecretOutput, error) {
	return m.create(in)
}

func (m *mockClient) PutSecretValue(
	_ context.Context, in *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.PutSecretValueOutput, error) {
	return m.putValue(in)
}

func (m *mockClient) DeleteSecret(
	_ context.Context, in *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.DeleteSecretOutput, error) {
	return m.deleteSec(in)
}

func (m *mockClient) RestoreSecret(
	_ context.Context, in *secretsmanager.RestoreSecretInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.RestoreSecretOutput, error) {
	return m.restore(in)
}

func (m *mockClient) TagResource(
	_ context.Context, in *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.TagResourceOutput, error) {
	return m.tag(in)
}

func (m *mockClient) UntagResource(
	_ context.Context, in *secretsmanager.UntagResourceInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.UntagResourceOutput, error) {
	return m.untag(in)
}

func (m *mockClient) ListSecrets(
	_ context.Context, in *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.ListSecretsOutput, error) {
	return m.listSecrets(in)
}

// versionsNewestFirst returns three versions; v3 is AWSCURRENT, v2 AWSPREVIOUS.
func versionsList() []types.SecretVersionsListEntry {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	return []types.SecretVersionsListEntry{
		{VersionId: aws.String("id-1"), CreatedDate: aws.Time(base), VersionStages: []string{}},
		{VersionId: aws.String("id-2"), CreatedDate: aws.Time(base.Add(time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
		{VersionId: aws.String("id-3"), CreatedDate: aws.Time(base.Add(2 * time.Hour)), VersionStages: []string{"AWSCURRENT"}},
	}
}

func TestResolve_Latest(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{})

	ref, err := store.Resolve(t.Context(), "my-secret", "")
	require.NoError(t, err)
	assert.True(t, ref.IsLatest())
}

func TestResolve_VersionID(t *testing.T) {
	t.Parallel()

	// Explicit id, no shift => no listing needed.
	store := secret.New(&mockClient{})

	ref, err := store.Resolve(t.Context(), "my-secret", "#abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", ref.ID())
}

func TestResolve_Label(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	// :AWSCURRENT resolves to the concrete version id (label confined here).
	ref, err := store.Resolve(t.Context(), "my-secret", ":AWSCURRENT")
	require.NoError(t, err)
	assert.Equal(t, "id-3", ref.ID())
}

func TestResolve_LabelThenShift(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	// :AWSCURRENT~1 => one before current (id-3) => id-2.
	ref, err := store.Resolve(t.Context(), "my-secret", ":AWSCURRENT~1")
	require.NoError(t, err)
	assert.Equal(t, "id-2", ref.ID())
}

func TestResolve_ShiftOutOfRange(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	_, err := store.Resolve(t.Context(), "my-secret", "~9")
	require.Error(t, err)
}

func TestGet_MapsEntryWithDescriptionAndTags(t *testing.T) {
	t.Parallel()

	var gotVersionID string

	store := secret.New(&mockClient{
		getValue: func(in *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			gotVersionID = aws.ToString(in.VersionId)

			return &secretsmanager.GetSecretValueOutput{
				Name:          aws.String("my-secret"),
				SecretString:  aws.String("s3cr3t"),
				VersionId:     aws.String("id-3"),
				VersionStages: []string{"AWSCURRENT"},
				CreatedDate:   aws.Time(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)),
			}, nil
		},
		describe: func(_ *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return &secretsmanager.DescribeSecretOutput{
				Description: aws.String("my desc"),
				Tags:        []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
			}, nil
		},
	})

	entry, err := store.Get(t.Context(), "my-secret", provider.NewVersionRef("id-3"))
	require.NoError(t, err)
	assert.Equal(t, "id-3", gotVersionID)
	assert.Equal(t, "s3cr3t", entry.Value)
	assert.Equal(t, domain.ValueTypeSecret, entry.Type)
	assert.Equal(t, "id-3", entry.Version.ID)
	assert.Equal(t, "AWSCURRENT", entry.Version.Label)
	assert.Equal(t, "my desc", entry.Description)
	require.Len(t, entry.Tags, 1)
	assert.Equal(t, "env", entry.Tags[0].Key)
}

func TestGet_LatestOmitsVersionID(t *testing.T) {
	t.Parallel()

	var hadVersionID bool

	store := secret.New(&mockClient{
		getValue: func(in *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			hadVersionID = in.VersionId != nil

			return &secretsmanager.GetSecretValueOutput{Name: aws.String("my-secret"), SecretString: aws.String("v")}, nil
		},
		describe: func(_ *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return &secretsmanager.DescribeSecretOutput{}, nil
		},
	})

	_, err := store.Get(t.Context(), "my-secret", provider.NewVersionRef(""))
	require.NoError(t, err)
	assert.False(t, hadVersionID)
}

func TestHistory_NewestFirst(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 3)
	assert.Equal(t, "id-3", versions[0].ID)
	assert.Equal(t, "AWSCURRENT", versions[0].Label)
	assert.Equal(t, "id-1", versions[2].ID)
}

func TestList_Paginated(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		listSecrets: func(in *secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error) {
			if aws.ToString(in.NextToken) == "" {
				return &secretsmanager.ListSecretsOutput{
					SecretList: []types.SecretListEntry{{Name: aws.String("a")}},
					NextToken:  aws.String("tok"),
				}, nil
			}

			return &secretsmanager.ListSecretsOutput{SecretList: []types.SecretListEntry{{Name: aws.String("b")}}}, nil
		},
	})

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestPut_CreateWhenNew(t *testing.T) {
	t.Parallel()

	var createIn *secretsmanager.CreateSecretInput

	store := secret.New(&mockClient{
		create: func(in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
			createIn = in

			return &secretsmanager.CreateSecretOutput{VersionId: aws.String("new-id")}, nil
		},
	})

	v, err := store.Put(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "desc")
	require.NoError(t, err)
	assert.Equal(t, "new-id", v.ID)
	require.NotNil(t, createIn)
	assert.Equal(t, "desc", aws.ToString(createIn.Description))
}

func TestPut_PutsNewVersionWhenExists(t *testing.T) {
	t.Parallel()

	var putCalled bool

	store := secret.New(&mockClient{
		create: func(_ *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
			return nil, &types.ResourceExistsException{Message: aws.String("exists")}
		},
		putValue: func(_ *secretsmanager.PutSecretValueInput) (*secretsmanager.PutSecretValueOutput, error) {
			putCalled = true

			return &secretsmanager.PutSecretValueOutput{VersionId: aws.String("ver-2")}, nil
		},
	})

	v, err := store.Put(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "")
	require.NoError(t, err)
	assert.True(t, putCalled)
	assert.Equal(t, "ver-2", v.ID)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	var gotID string

	store := secret.New(&mockClient{
		deleteSec: func(in *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
			gotID = aws.ToString(in.SecretId)

			return &secretsmanager.DeleteSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "my-secret"))
	assert.Equal(t, "my-secret", gotID)
}

func TestRestore(t *testing.T) {
	t.Parallel()

	var gotID string

	store := secret.New(&mockClient{
		restore: func(in *secretsmanager.RestoreSecretInput) (*secretsmanager.RestoreSecretOutput, error) {
			gotID = aws.ToString(in.SecretId)

			return &secretsmanager.RestoreSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Restore(t.Context(), "my-secret"))
	assert.Equal(t, "my-secret", gotID)
}

func TestDescribe(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		describe: func(_ *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return &secretsmanager.DescribeSecretOutput{
				Name:               aws.String("my-secret"),
				Description:        aws.String("the desc"),
				Tags:               []types.Tag{{Key: aws.String("team"), Value: aws.String("sec")}},
				LastChangedDate:    aws.Time(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)),
				VersionIdsToStages: map[string][]string{"id-3": {"AWSCURRENT"}},
			}, nil
		},
	})

	entry, err := store.Describe(t.Context(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "my-secret", entry.Name)
	assert.Empty(t, entry.Value) // Describe never fetches the value
	assert.Equal(t, domain.ValueTypeSecret, entry.Type)
	assert.Equal(t, "the desc", entry.Description)
	assert.Equal(t, "id-3", entry.Version.ID)
	require.Len(t, entry.Tags, 1)
	assert.Equal(t, "team", entry.Tags[0].Key)
}

func TestTagAndUntag(t *testing.T) {
	t.Parallel()

	var (
		tagIn   *secretsmanager.TagResourceInput
		untagIn *secretsmanager.UntagResourceInput
	)

	store := secret.New(&mockClient{
		tag: func(in *secretsmanager.TagResourceInput) (*secretsmanager.TagResourceOutput, error) {
			tagIn = in

			return &secretsmanager.TagResourceOutput{}, nil
		},
		untag: func(in *secretsmanager.UntagResourceInput) (*secretsmanager.UntagResourceOutput, error) {
			untagIn = in

			return &secretsmanager.UntagResourceOutput{}, nil
		},
	})

	require.NoError(t, store.Tag(t.Context(), "my-secret", map[string]string{"env": "prod"}))
	require.NotNil(t, tagIn)
	require.Len(t, tagIn.Tags, 1)

	require.NoError(t, store.Untag(t.Context(), "my-secret", []string{"env"}))
	require.NotNil(t, untagIn)
	assert.Equal(t, []string{"env"}, untagIn.TagKeys)
}

func TestCreate_NewReturnsVersion(t *testing.T) {
	t.Parallel()

	var createIn *secretsmanager.CreateSecretInput

	store := secret.New(&mockClient{
		create: func(in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
			createIn = in

			return &secretsmanager.CreateSecretOutput{VersionId: aws.String("new-id")}, nil
		},
	})

	v, err := store.Create(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "desc")
	require.NoError(t, err)
	assert.Equal(t, "new-id", v.ID)
	require.NotNil(t, createIn)
	assert.Equal(t, "desc", aws.ToString(createIn.Description))
}

func TestCreate_AlreadyExistsMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		create: func(*secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
			return nil, &types.ResourceExistsException{Message: aws.String("exists")}
		},
	})

	_, err := store.Create(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrAlreadyExists)
}

func TestGet_NotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		getValue: func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("nope")}
		},
	})

	_, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

func TestDescribe_NotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		describe: func(*secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("nope")}
		},
	})

	_, err := store.Describe(t.Context(), "my-secret")
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

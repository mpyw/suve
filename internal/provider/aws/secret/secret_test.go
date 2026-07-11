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
	updateSec   func(*secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error)
	rotate      func(*secretsmanager.RotateSecretInput) (*secretsmanager.RotateSecretOutput, error)
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

func (m *mockClient) UpdateSecret(
	_ context.Context, in *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.UpdateSecretOutput, error) {
	return m.updateSec(in)
}

func (m *mockClient) RotateSecret(
	_ context.Context, in *secretsmanager.RotateSecretInput, _ ...func(*secretsmanager.Options),
) (*secretsmanager.RotateSecretOutput, error) {
	return m.rotate(in)
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
	// StagingLabels carries the full stage set; State is empty (no such concept).
	assert.Equal(t, []string{"AWSCURRENT"}, entry.Version.StagingLabels)
	assert.Empty(t, entry.Version.State)
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

// A version can carry several staging labels in an unspecified order; the
// representative Label must be chosen deterministically by priority, so
// AWSCURRENT wins even when it is not the first stage returned (#317).
func TestGet_LabelPrefersAWSCURRENTRegardlessOfOrder(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		getValue: func(_ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:          aws.String("my-secret"),
				SecretString:  aws.String("v"),
				VersionId:     aws.String("id-3"),
				VersionStages: []string{"my-custom-label", "AWSPREVIOUS", "AWSCURRENT"},
			}, nil
		},
		describe: func(_ *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return &secretsmanager.DescribeSecretOutput{}, nil
		},
	})

	entry, err := store.Get(t.Context(), "my-secret", provider.NewVersionRef("id-3"))
	require.NoError(t, err)
	// StagingLabels keeps every staging label AWS returned, in order.
	assert.Equal(t, []string{"my-custom-label", "AWSPREVIOUS", "AWSCURRENT"}, entry.Version.StagingLabels)
}

// History preserves every staging label AWS returns for a version, in the
// order AWS reported them (no collapsing to a single representative).
func TestHistory_StagingLabels(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := secret.New(&mockClient{
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     aws.String("id-2"),
						CreatedDate:   aws.Time(base.Add(time.Hour)),
						VersionStages: []string{"AWSPREVIOUS", "AWSPENDING"},
					},
					{
						VersionId:     aws.String("id-1"),
						CreatedDate:   aws.Time(base),
						VersionStages: []string{"zeta", "alpha"},
					},
				},
			}, nil
		},
	})

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	// Newest first: id-2.
	assert.Equal(t, "id-2", versions[0].ID)
	assert.Equal(t, []string{"AWSPREVIOUS", "AWSPENDING"}, versions[0].StagingLabels)
	assert.Equal(t, "id-1", versions[1].ID)
	assert.Equal(t, []string{"zeta", "alpha"}, versions[1].StagingLabels)
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
	assert.Equal(t, []string{"AWSCURRENT"}, versions[0].StagingLabels)
	assert.Equal(t, "id-1", versions[2].ID)
}

func TestHistory_IncludeDeprecatedAndPaginates(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	var (
		gotIncludeDeprecated bool
		calls                int
	)

	store := secret.New(&mockClient{
		listVersion: func(in *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			gotIncludeDeprecated = aws.ToBool(in.IncludeDeprecated)
			calls++

			if aws.ToString(in.NextToken) == "" {
				// Page 1: the labeled versions.
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []types.SecretVersionsListEntry{
						{VersionId: aws.String("id-3"), CreatedDate: aws.Time(base.Add(2 * time.Hour)), VersionStages: []string{"AWSCURRENT"}},
						{VersionId: aws.String("id-2"), CreatedDate: aws.Time(base.Add(time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
					},
					NextToken: aws.String("tok"),
				}, nil
			}

			// Page 2: a deprecated (unlabeled) version, retained but invisible
			// without IncludeDeprecated.
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("id-1"), CreatedDate: aws.Time(base), VersionStages: []string{}},
				},
			}, nil
		},
	})

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	assert.True(t, gotIncludeDeprecated, "History must request IncludeDeprecated=true")
	assert.Equal(t, 2, calls, "History must page through all results")
	require.Len(t, versions, 3)
	assert.Equal(t, "id-3", versions[0].ID)
	assert.Equal(t, "id-1", versions[2].ID, "deprecated version must be included as the oldest")
}

func TestResolve_ShiftReachesDeprecatedVersion(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	var gotIncludeDeprecated bool

	store := secret.New(&mockClient{
		listVersion: func(in *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			gotIncludeDeprecated = aws.ToBool(in.IncludeDeprecated)

			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("id-3"), CreatedDate: aws.Time(base.Add(2 * time.Hour)), VersionStages: []string{"AWSCURRENT"}},
					{VersionId: aws.String("id-2"), CreatedDate: aws.Time(base.Add(time.Hour)), VersionStages: []string{}},
					{VersionId: aws.String("id-1"), CreatedDate: aws.Time(base), VersionStages: []string{}},
				},
			}, nil
		},
	})

	// AWSCURRENT~2 walks two versions back into a deprecated (unlabeled) one;
	// this would fail with "version shift out of range" if deprecated versions
	// were excluded from the listing.
	ref, err := store.Resolve(t.Context(), "my-secret", ":AWSCURRENT~2")
	require.NoError(t, err)
	assert.True(t, gotIncludeDeprecated, "Resolve must request IncludeDeprecated=true")
	assert.Equal(t, "id-1", ref.ID())
}

func TestResolve_BareShiftAnchorsAtAWSCURRENT(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// During an in-progress rotation AWSPENDING is the newest-CREATED version.
	store := secret.New(&mockClient{
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("pending"), CreatedDate: aws.Time(base.Add(3 * time.Hour)), VersionStages: []string{"AWSPENDING"}},
					{VersionId: aws.String("current"), CreatedDate: aws.Time(base.Add(2 * time.Hour)), VersionStages: []string{"AWSCURRENT"}},
					{VersionId: aws.String("previous"), CreatedDate: aws.Time(base.Add(time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
				},
			}, nil
		},
	})

	// Bare ~1 counts back from AWSCURRENT (what the bare name resolves to), not
	// from the newest-created version (AWSPENDING), so it reaches AWSPREVIOUS.
	// With the old index-0 anchor it would have returned AWSCURRENT. (#313)
	ref, err := store.Resolve(t.Context(), "my-secret", "~1")
	require.NoError(t, err)
	assert.Equal(t, "previous", ref.ID())
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

func TestPut_UpdatesWhenExists(t *testing.T) {
	t.Parallel()

	var updateIn *secretsmanager.UpdateSecretInput

	store := secret.New(&mockClient{
		create: func(_ *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
			return nil, &types.ResourceExistsException{Message: aws.String("exists")}
		},
		updateSec: func(in *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
			updateIn = in

			return &secretsmanager.UpdateSecretOutput{VersionId: aws.String("ver-2")}, nil
		},
	})

	// Put on an existing secret updates both value and description in one call.
	v, err := store.Put(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "new desc")
	require.NoError(t, err)
	assert.Equal(t, "ver-2", v.ID)
	require.NotNil(t, updateIn)
	assert.Equal(t, "val", aws.ToString(updateIn.SecretString))
	assert.Equal(t, "new desc", aws.ToString(updateIn.Description))
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

func TestDelete_ForceDelete(t *testing.T) {
	t.Parallel()

	var in *secretsmanager.DeleteSecretInput

	store := secret.New(&mockClient{
		deleteSec: func(got *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
			in = got

			return &secretsmanager.DeleteSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "my-secret", provider.ForceDelete{}))
	require.NotNil(t, in)
	assert.True(t, aws.ToBool(in.ForceDeleteWithoutRecovery))
	assert.Nil(t, in.RecoveryWindowInDays)
}

func TestDelete_RecoveryWindow(t *testing.T) {
	t.Parallel()

	var in *secretsmanager.DeleteSecretInput

	store := secret.New(&mockClient{
		deleteSec: func(got *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
			in = got

			return &secretsmanager.DeleteSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "my-secret", secret.RecoveryWindow{Days: 14}))
	require.NotNil(t, in)
	assert.Equal(t, int64(14), aws.ToInt64(in.RecoveryWindowInDays))
	assert.Nil(t, in.ForceDeleteWithoutRecovery)
}

func TestGet_PopulatesExtraARN(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		getValue: func(*secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				ARN:          aws.String("arn:aws:secretsmanager:us-east-1:123:secret:my-secret-AbCdEf"),
				SecretString: aws.String("val"),
				VersionId:    aws.String("id-3"),
			}, nil
		},
		describe: func(*secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return &secretsmanager.DescribeSecretOutput{}, nil
		},
	})

	entry, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.NoError(t, err)
	require.Len(t, entry.Extra, 1)
	assert.Equal(t, "ARN", entry.Extra[0].Label)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:my-secret-AbCdEf", entry.Extra[0].Value)
}

func TestCreate_AppliesKMSKeyAndRotation(t *testing.T) {
	t.Parallel()

	var (
		createIn *secretsmanager.CreateSecretInput
		rotateIn *secretsmanager.RotateSecretInput
	)

	store := secret.New(&mockClient{
		create: func(in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
			createIn = in

			return &secretsmanager.CreateSecretOutput{VersionId: aws.String("new-id")}, nil
		},
		rotate: func(in *secretsmanager.RotateSecretInput) (*secretsmanager.RotateSecretOutput, error) {
			rotateIn = in

			return &secretsmanager.RotateSecretOutput{}, nil
		},
	})

	_, err := store.Create(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "",
		secret.KMSKeyID{Value: "alias/my-key"},
		secret.RotationRules{AutomaticallyAfterDays: 30},
	)
	require.NoError(t, err)
	require.NotNil(t, createIn)
	assert.Equal(t, "alias/my-key", aws.ToString(createIn.KmsKeyId))
	require.NotNil(t, rotateIn)
	require.NotNil(t, rotateIn.RotationRules)
	assert.Equal(t, int64(30), aws.ToInt64(rotateIn.RotationRules.AutomaticallyAfterDays))
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

	// The secret was created long before its current version; Describe must
	// report the VERSION's own CreatedDate, not the secret's (#317).
	secretCreated := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	currentVersionCreated := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)

	store := secret.New(&mockClient{
		describe: func(_ *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
			return &secretsmanager.DescribeSecretOutput{
				Name:               aws.String("my-secret"),
				Description:        aws.String("the desc"),
				Tags:               []types.Tag{{Key: aws.String("team"), Value: aws.String("sec")}},
				CreatedDate:        aws.Time(secretCreated),
				LastChangedDate:    aws.Time(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)),
				VersionIdsToStages: map[string][]string{"id-3": {"AWSCURRENT"}},
			}, nil
		},
		listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{VersionId: aws.String("id-1"), CreatedDate: aws.Time(secretCreated), VersionStages: []string{}},
					{VersionId: aws.String("id-3"), CreatedDate: aws.Time(currentVersionCreated), VersionStages: []string{"AWSCURRENT"}},
				},
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
	assert.Equal(t, []string{"AWSCURRENT"}, entry.Version.StagingLabels)
	// The version's own creation time, not the secret-level CreatedDate.
	require.NotNil(t, entry.Version.Created)
	assert.Equal(t, currentVersionCreated, *entry.Version.Created)
	assert.NotEqual(t, secretCreated, *entry.Version.Created)
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

// TestResolve_ShiftNotFoundMapsSentinel guards #481: a ~shift (or label) spec
// drives resolution through ListSecretVersionIds instead of GetSecretValue. A
// missing secret must still map to provider.ErrNotFound so callers see the same
// sentinel they get on the no-shift path (Get).
func TestResolve_ShiftNotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secret.New(&mockClient{
		listVersion: func(*secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("nope")}
		},
	})

	_, err := store.Resolve(t.Context(), "missing-secret", "~1")
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

// TestHistory_DeterministicOnEqualTimestamps guards #314: versions with equal
// CreatedDate must sort deterministically (version-id descending tie-break),
// independent of the arbitrary API/list order.
func TestHistory_DeterministicOnEqualTimestamps(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	mk := func(order []string) *secret.Store {
		return secret.New(&mockClient{
			listVersion: func(_ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				vs := make([]types.SecretVersionsListEntry, len(order))
				for i, id := range order {
					vs[i] = types.SecretVersionsListEntry{VersionId: aws.String(id), CreatedDate: aws.Time(created)}
				}

				return &secretsmanager.ListSecretVersionIdsOutput{Versions: vs}, nil
			},
		})
	}

	for _, order := range [][]string{{"aaa", "bbb"}, {"bbb", "aaa"}} {
		versions, err := mk(order).History(t.Context(), "my-secret")
		require.NoError(t, err)
		require.Len(t, versions, 2)
		assert.Equal(t, "bbb", versions[0].ID, "input %v", order) // id-desc tie-break
		assert.Equal(t, "aaa", versions[1].ID, "input %v", order)
	}
}

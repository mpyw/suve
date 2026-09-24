package secretsmanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	secretsmanagersdk "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/secretsmanager"
)

// mockClient is a configurable mock of the narrow Secrets Manager interface.
type mockClient struct {
	getValue    func(*secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error)
	listVersion func(*secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error)
	describe    func(*secretsmanagersdk.DescribeSecretInput) (*secretsmanagersdk.DescribeSecretOutput, error)
	create      func(*secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error)
	updateSec   func(*secretsmanagersdk.UpdateSecretInput) (*secretsmanagersdk.UpdateSecretOutput, error)
	rotate      func(*secretsmanagersdk.RotateSecretInput) (*secretsmanagersdk.RotateSecretOutput, error)
	deleteSec   func(*secretsmanagersdk.DeleteSecretInput) (*secretsmanagersdk.DeleteSecretOutput, error)
	restore     func(*secretsmanagersdk.RestoreSecretInput) (*secretsmanagersdk.RestoreSecretOutput, error)
	tag         func(*secretsmanagersdk.TagResourceInput) (*secretsmanagersdk.TagResourceOutput, error)
	untag       func(*secretsmanagersdk.UntagResourceInput) (*secretsmanagersdk.UntagResourceOutput, error)
	listSecrets func(*secretsmanagersdk.ListSecretsInput) (*secretsmanagersdk.ListSecretsOutput, error)
}

func (m *mockClient) GetSecretValue(
	_ context.Context, in *secretsmanagersdk.GetSecretValueInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.GetSecretValueOutput, error) {
	return m.getValue(in)
}

//nolint:revive // Method name matches AWS SDK interface naming convention
func (m *mockClient) ListSecretVersionIds(
	_ context.Context, in *secretsmanagersdk.ListSecretVersionIdsInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
	return m.listVersion(in)
}

func (m *mockClient) DescribeSecret(
	_ context.Context, in *secretsmanagersdk.DescribeSecretInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.DescribeSecretOutput, error) {
	return m.describe(in)
}

func (m *mockClient) CreateSecret(
	_ context.Context, in *secretsmanagersdk.CreateSecretInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.CreateSecretOutput, error) {
	return m.create(in)
}

func (m *mockClient) UpdateSecret(
	_ context.Context, in *secretsmanagersdk.UpdateSecretInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.UpdateSecretOutput, error) {
	return m.updateSec(in)
}

func (m *mockClient) RotateSecret(
	_ context.Context, in *secretsmanagersdk.RotateSecretInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.RotateSecretOutput, error) {
	return m.rotate(in)
}

func (m *mockClient) DeleteSecret(
	_ context.Context, in *secretsmanagersdk.DeleteSecretInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.DeleteSecretOutput, error) {
	return m.deleteSec(in)
}

func (m *mockClient) RestoreSecret(
	_ context.Context, in *secretsmanagersdk.RestoreSecretInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.RestoreSecretOutput, error) {
	return m.restore(in)
}

func (m *mockClient) TagResource(
	_ context.Context, in *secretsmanagersdk.TagResourceInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.TagResourceOutput, error) {
	return m.tag(in)
}

func (m *mockClient) UntagResource(
	_ context.Context, in *secretsmanagersdk.UntagResourceInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.UntagResourceOutput, error) {
	return m.untag(in)
}

func (m *mockClient) ListSecrets(
	_ context.Context, in *secretsmanagersdk.ListSecretsInput, _ ...func(*secretsmanagersdk.Options),
) (*secretsmanagersdk.ListSecretsOutput, error) {
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

	store := secretsmanager.New(&mockClient{})

	ref, err := store.Resolve(t.Context(), "my-secret", "")
	require.NoError(t, err)
	assert.True(t, ref.IsLatest())
}

func TestResolve_VersionID(t *testing.T) {
	t.Parallel()

	// Explicit id, no shift => no listing needed.
	store := secretsmanager.New(&mockClient{})

	ref, err := store.Resolve(t.Context(), "my-secret", "#abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", ref.ID())
}

func TestResolve_Label(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	// :AWSCURRENT resolves to the concrete version id (label confined here).
	ref, err := store.Resolve(t.Context(), "my-secret", ":AWSCURRENT")
	require.NoError(t, err)
	assert.Equal(t, "id-3", ref.ID())
}

func TestResolve_LabelThenShift(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	// :AWSCURRENT~1 => one before current (id-3) => id-2.
	ref, err := store.Resolve(t.Context(), "my-secret", ":AWSCURRENT~1")
	require.NoError(t, err)
	assert.Equal(t, "id-2", ref.ID())
}

func TestResolve_ShiftOutOfRange(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	_, err := store.Resolve(t.Context(), "my-secret", "~9")
	require.Error(t, err)
}

func TestGet_MapsEntryWithDescriptionAndTags(t *testing.T) {
	t.Parallel()

	var gotVersionID string

	store := secretsmanager.New(&mockClient{
		getValue: func(in *secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error) {
			gotVersionID = aws.ToString(in.VersionId)

			return &secretsmanagersdk.GetSecretValueOutput{
				Name:          aws.String("my-secret"),
				SecretString:  aws.String("s3cr3t"),
				VersionId:     aws.String("id-3"),
				VersionStages: []string{"AWSCURRENT"},
				CreatedDate:   aws.Time(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)),
			}, nil
		},
		describe: func(_ *secretsmanagersdk.DescribeSecretInput) (*secretsmanagersdk.DescribeSecretOutput, error) {
			return &secretsmanagersdk.DescribeSecretOutput{
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
	// Labels carries the full stage set; State is empty (no such concept).
	assert.Equal(t, []string{"AWSCURRENT"}, entry.Version.Labels)
	assert.Empty(t, entry.Version.State)
	assert.Equal(t, "my desc", entry.Description)
	require.Len(t, entry.Tags, 1)
	assert.Equal(t, "env", entry.Tags[0].Key)
}

func TestGet_LatestOmitsVersionID(t *testing.T) {
	t.Parallel()

	var hadVersionID bool

	store := secretsmanager.New(&mockClient{
		getValue: func(in *secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error) {
			hadVersionID = in.VersionId != nil

			return &secretsmanagersdk.GetSecretValueOutput{Name: aws.String("my-secret"), SecretString: aws.String("v")}, nil
		},
		describe: func(_ *secretsmanagersdk.DescribeSecretInput) (*secretsmanagersdk.DescribeSecretOutput, error) {
			return &secretsmanagersdk.DescribeSecretOutput{}, nil
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

	store := secretsmanager.New(&mockClient{
		getValue: func(_ *secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error) {
			return &secretsmanagersdk.GetSecretValueOutput{
				Name:          aws.String("my-secret"),
				SecretString:  aws.String("v"),
				VersionId:     aws.String("id-3"),
				VersionStages: []string{"my-custom-label", "AWSPREVIOUS", "AWSCURRENT"},
			}, nil
		},
		describe: func(_ *secretsmanagersdk.DescribeSecretInput) (*secretsmanagersdk.DescribeSecretOutput, error) {
			return &secretsmanagersdk.DescribeSecretOutput{}, nil
		},
	})

	entry, err := store.Get(t.Context(), "my-secret", provider.NewVersionRef("id-3"))
	require.NoError(t, err)
	// Labels keeps every staging label AWS returned, in order.
	assert.Equal(t, []string{"my-custom-label", "AWSPREVIOUS", "AWSCURRENT"}, entry.Version.Labels)
}

// History preserves every staging label AWS returns for a version, in the
// order AWS reported them (no collapsing to a single representative).
func TestHistory_StagingLabels(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{
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
	assert.Equal(t, []string{"AWSPREVIOUS", "AWSPENDING"}, versions[0].Labels)
	assert.Equal(t, "id-1", versions[1].ID)
	assert.Equal(t, []string{"zeta", "alpha"}, versions[1].Labels)
	// No version carries AWSCURRENT, so the newest one is current.
	assert.True(t, versions[0].Current)
	assert.False(t, versions[1].Current)
}

// TestHistory_CurrentIsAWSCURRENT pins that the current version is the one
// carrying the AWSCURRENT staging label (membership, even alongside a custom
// label, #317), not the newest one.
func TestHistory_CurrentIsAWSCURRENT(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     aws.String("pending"),
						CreatedDate:   aws.Time(base.Add(time.Hour)),
						VersionStages: []string{"AWSPENDING"},
					},
					{
						VersionId:     aws.String("current"),
						CreatedDate:   aws.Time(base),
						VersionStages: []string{"custom", "AWSCURRENT"},
					},
				},
			}, nil
		},
	})

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "pending", versions[0].ID)
	assert.False(t, versions[0].Current)
	assert.Equal(t, "current", versions[1].ID)
	assert.True(t, versions[1].Current)
}

func TestHistory_NewestFirst(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{Versions: versionsList()}, nil
		},
	})

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 3)
	assert.Equal(t, "id-3", versions[0].ID)
	assert.Equal(t, []string{"AWSCURRENT"}, versions[0].Labels)
	assert.Equal(t, "id-1", versions[2].ID)
}

func TestHistory_IncludeDeprecatedAndPaginates(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	var (
		gotIncludeDeprecated bool
		calls                int
	)

	store := secretsmanager.New(&mockClient{
		listVersion: func(in *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			gotIncludeDeprecated = aws.ToBool(in.IncludeDeprecated)
			calls++

			if aws.ToString(in.NextToken) == "" {
				// Page 1: the labeled versions.
				return &secretsmanagersdk.ListSecretVersionIdsOutput{
					Versions: []types.SecretVersionsListEntry{
						{VersionId: aws.String("id-3"), CreatedDate: aws.Time(base.Add(2 * time.Hour)), VersionStages: []string{"AWSCURRENT"}},
						{VersionId: aws.String("id-2"), CreatedDate: aws.Time(base.Add(time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
					},
					NextToken: aws.String("tok"),
				}, nil
			}

			// Page 2: a deprecated (unlabeled) version, retained but invisible
			// without IncludeDeprecated.
			return &secretsmanagersdk.ListSecretVersionIdsOutput{
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

	store := secretsmanager.New(&mockClient{
		listVersion: func(in *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			gotIncludeDeprecated = aws.ToBool(in.IncludeDeprecated)

			return &secretsmanagersdk.ListSecretVersionIdsOutput{
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
	store := secretsmanager.New(&mockClient{
		listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
			return &secretsmanagersdk.ListSecretVersionIdsOutput{
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

	store := secretsmanager.New(&mockClient{
		listSecrets: func(in *secretsmanagersdk.ListSecretsInput) (*secretsmanagersdk.ListSecretsOutput, error) {
			if aws.ToString(in.NextToken) == "" {
				return &secretsmanagersdk.ListSecretsOutput{
					SecretList: []types.SecretListEntry{{Name: aws.String("a")}},
					NextToken:  aws.String("tok"),
				}, nil
			}

			return &secretsmanagersdk.ListSecretsOutput{SecretList: []types.SecretListEntry{{Name: aws.String("b")}}}, nil
		},
	})

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestPut_CreateWhenNew(t *testing.T) {
	t.Parallel()

	var createIn *secretsmanagersdk.CreateSecretInput

	store := secretsmanager.New(&mockClient{
		create: func(in *secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error) {
			createIn = in

			return &secretsmanagersdk.CreateSecretOutput{VersionId: aws.String("new-id")}, nil
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

	var updateIn *secretsmanagersdk.UpdateSecretInput

	store := secretsmanager.New(&mockClient{
		create: func(_ *secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error) {
			return nil, &types.ResourceExistsException{Message: aws.String("exists")}
		},
		updateSec: func(in *secretsmanagersdk.UpdateSecretInput) (*secretsmanagersdk.UpdateSecretOutput, error) {
			updateIn = in

			return &secretsmanagersdk.UpdateSecretOutput{VersionId: aws.String("ver-2")}, nil
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

	store := secretsmanager.New(&mockClient{
		deleteSec: func(in *secretsmanagersdk.DeleteSecretInput) (*secretsmanagersdk.DeleteSecretOutput, error) {
			gotID = aws.ToString(in.SecretId)

			return &secretsmanagersdk.DeleteSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "my-secret"))
	assert.Equal(t, "my-secret", gotID)
}

func TestDelete_ForceDelete(t *testing.T) {
	t.Parallel()

	var in *secretsmanagersdk.DeleteSecretInput

	store := secretsmanager.New(&mockClient{
		deleteSec: func(got *secretsmanagersdk.DeleteSecretInput) (*secretsmanagersdk.DeleteSecretOutput, error) {
			in = got

			return &secretsmanagersdk.DeleteSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "my-secret", provider.ForceDelete{}))
	require.NotNil(t, in)
	assert.True(t, aws.ToBool(in.ForceDeleteWithoutRecovery))
	assert.Nil(t, in.RecoveryWindowInDays)
}

func TestDelete_RecoveryWindow(t *testing.T) {
	t.Parallel()

	var in *secretsmanagersdk.DeleteSecretInput

	store := secretsmanager.New(&mockClient{
		deleteSec: func(got *secretsmanagersdk.DeleteSecretInput) (*secretsmanagersdk.DeleteSecretOutput, error) {
			in = got

			return &secretsmanagersdk.DeleteSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Delete(t.Context(), "my-secret", secretsmanager.RecoveryWindow{Days: 14}))
	require.NotNil(t, in)
	assert.Equal(t, int64(14), aws.ToInt64(in.RecoveryWindowInDays))
	assert.Nil(t, in.ForceDeleteWithoutRecovery)
}

// TestDelete_NotFoundMapsSentinel guards the ResourceNotFound→ErrNotFound
// mapping on the delete path so callers can treat a missing secret idempotently.
func TestDelete_NotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		deleteSec: func(*secretsmanagersdk.DeleteSecretInput) (*secretsmanagersdk.DeleteSecretOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("nope")}
		},
	})

	err := store.Delete(t.Context(), "missing-secret")
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

// TestPut_UpdatesWhenExistsAppliesKMSKey covers applyUpdateOptions: a KMSKeyID
// WriteOption must fold onto the UpdateSecretInput when Put updates an existing
// secret (the create path already covers applyCreateOptions).
func TestPut_UpdatesWhenExistsAppliesKMSKey(t *testing.T) {
	t.Parallel()

	var updateIn *secretsmanagersdk.UpdateSecretInput

	store := secretsmanager.New(&mockClient{
		create: func(*secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error) {
			return nil, &types.ResourceExistsException{Message: aws.String("exists")}
		},
		updateSec: func(in *secretsmanagersdk.UpdateSecretInput) (*secretsmanagersdk.UpdateSecretOutput, error) {
			updateIn = in

			return &secretsmanagersdk.UpdateSecretOutput{VersionId: aws.String("ver-2")}, nil
		},
	})

	_, err := store.Put(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "",
		secretsmanager.KMSKeyID{Value: "alias/my-key"},
	)
	require.NoError(t, err)
	require.NotNil(t, updateIn)
	assert.Equal(t, "alias/my-key", aws.ToString(updateIn.KmsKeyId))
}

func TestGet_PopulatesExtraARN(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		getValue: func(*secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error) {
			return &secretsmanagersdk.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				ARN:          aws.String("arn:aws:secretsmanager:us-east-1:123:secret:my-secret-AbCdEf"),
				SecretString: aws.String("val"),
				VersionId:    aws.String("id-3"),
			}, nil
		},
		describe: func(*secretsmanagersdk.DescribeSecretInput) (*secretsmanagersdk.DescribeSecretOutput, error) {
			return &secretsmanagersdk.DescribeSecretOutput{}, nil
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
		createIn *secretsmanagersdk.CreateSecretInput
		rotateIn *secretsmanagersdk.RotateSecretInput
	)

	store := secretsmanager.New(&mockClient{
		create: func(in *secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error) {
			createIn = in

			return &secretsmanagersdk.CreateSecretOutput{VersionId: aws.String("new-id")}, nil
		},
		rotate: func(in *secretsmanagersdk.RotateSecretInput) (*secretsmanagersdk.RotateSecretOutput, error) {
			rotateIn = in

			return &secretsmanagersdk.RotateSecretOutput{}, nil
		},
	})

	_, err := store.Create(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "",
		secretsmanager.KMSKeyID{Value: "alias/my-key"},
		secretsmanager.RotationRules{AutomaticallyAfterDays: 30},
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

	store := secretsmanager.New(&mockClient{
		restore: func(in *secretsmanagersdk.RestoreSecretInput) (*secretsmanagersdk.RestoreSecretOutput, error) {
			gotID = aws.ToString(in.SecretId)

			return &secretsmanagersdk.RestoreSecretOutput{}, nil
		},
	})

	require.NoError(t, store.Restore(t.Context(), "my-secret"))
	assert.Equal(t, "my-secret", gotID)
}

func TestTagAndUntag(t *testing.T) {
	t.Parallel()

	var (
		tagIn   *secretsmanagersdk.TagResourceInput
		untagIn *secretsmanagersdk.UntagResourceInput
	)

	store := secretsmanager.New(&mockClient{
		tag: func(in *secretsmanagersdk.TagResourceInput) (*secretsmanagersdk.TagResourceOutput, error) {
			tagIn = in

			return &secretsmanagersdk.TagResourceOutput{}, nil
		},
		untag: func(in *secretsmanagersdk.UntagResourceInput) (*secretsmanagersdk.UntagResourceOutput, error) {
			untagIn = in

			return &secretsmanagersdk.UntagResourceOutput{}, nil
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

	var createIn *secretsmanagersdk.CreateSecretInput

	store := secretsmanager.New(&mockClient{
		create: func(in *secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error) {
			createIn = in

			return &secretsmanagersdk.CreateSecretOutput{VersionId: aws.String("new-id")}, nil
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

	store := secretsmanager.New(&mockClient{
		create: func(*secretsmanagersdk.CreateSecretInput) (*secretsmanagersdk.CreateSecretOutput, error) {
			return nil, &types.ResourceExistsException{Message: aws.String("exists")}
		},
	})

	_, err := store.Create(t.Context(), "my-secret", "val", domain.ValueTypeSecret, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrAlreadyExists)
}

func TestGet_NotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		getValue: func(*secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("nope")}
		},
	})

	_, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

// TestGet_BinarySecretRejected guards #469: a secret stored via SecretBinary
// (SecretString nil) must NOT be mapped to an empty value with a nil error, as
// that misrepresents a non-empty secret and lets a staged string edit clobber it
// on apply. Get must fail with provider.ErrBinaryValue instead.
func TestGet_BinarySecretRejected(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		getValue: func(*secretsmanagersdk.GetSecretValueInput) (*secretsmanagersdk.GetSecretValueOutput, error) {
			return &secretsmanagersdk.GetSecretValueOutput{
				Name:         aws.String("my-secret"),
				SecretBinary: []byte{0x00, 0x01},
				SecretString: nil,
				VersionId:    aws.String("id-1"),
			}, nil
		},
	})

	entry, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.Error(t, err)
	require.ErrorIs(t, err, provider.ErrBinaryValue)
	assert.Nil(t, entry)
}

// TestResolve_ShiftNotFoundMapsSentinel guards #481: a ~shift (or label) spec
// drives resolution through ListSecretVersionIds instead of GetSecretValue. A
// missing secret must still map to provider.ErrNotFound so callers see the same
// sentinel they get on the no-shift path (Get).
func TestResolve_ShiftNotFoundMapsSentinel(t *testing.T) {
	t.Parallel()

	store := secretsmanager.New(&mockClient{
		listVersion: func(*secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
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

	mk := func(order []string) *secretsmanager.Store {
		return secretsmanager.New(&mockClient{
			listVersion: func(_ *secretsmanagersdk.ListSecretVersionIdsInput) (*secretsmanagersdk.ListSecretVersionIdsOutput, error) {
				vs := make([]types.SecretVersionsListEntry, len(order))
				for i, id := range order {
					vs[i] = types.SecretVersionsListEntry{VersionId: aws.String(id), CreatedDate: aws.Time(created)}
				}

				return &secretsmanagersdk.ListSecretVersionIdsOutput{Versions: vs}, nil
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

package secret

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version/awssecretversion"
)

// baseIndex and sortNewestFirst are pure package-private helpers over the raw
// SDK version-list type. The localstack emulator always returns well-formed,
// timestamped, labeled versions, so the not-found and nil-CreatedDate branches
// never fire in e2e; they are exercised here with crafted lists.

// baseList is a newest-first list: id-3 is AWSCURRENT, id-2 AWSPREVIOUS, id-1
// deprecated (unlabeled).
func baseList() []types.SecretVersionsListEntry {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	return []types.SecretVersionsListEntry{
		{VersionId: aws.String("id-3"), CreatedDate: aws.Time(base.Add(2 * time.Hour)), VersionStages: []string{"AWSCURRENT"}},
		{VersionId: aws.String("id-2"), CreatedDate: aws.Time(base.Add(time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
		{VersionId: aws.String("id-1"), CreatedDate: aws.Time(base), VersionStages: []string{}},
	}
}

func TestBaseIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		abs     awssecretversion.AbsoluteSpec
		wantIdx int
		wantErr string
	}{
		{
			name:    "id found",
			abs:     awssecretversion.AbsoluteSpec{ID: aws.String("id-2")},
			wantIdx: 1,
		},
		{
			name:    "id not found",
			abs:     awssecretversion.AbsoluteSpec{ID: aws.String("nope")},
			wantErr: "version ID not found: nope",
		},
		{
			name:    "label found",
			abs:     awssecretversion.AbsoluteSpec{Label: aws.String("AWSPREVIOUS")},
			wantIdx: 1,
		},
		{
			name:    "label not found",
			abs:     awssecretversion.AbsoluteSpec{Label: aws.String("NOSUCHLABEL")},
			wantErr: "version label not found: NOSUCHLABEL",
		},
		{
			name:    "no spec anchors at AWSCURRENT",
			abs:     awssecretversion.AbsoluteSpec{},
			wantIdx: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idx, err := baseIndex(baseList(), tt.abs)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantIdx, idx)
		})
	}
}

// TestBaseIndex_NoSpecFallsBackToZero covers the default branch when no version
// carries AWSCURRENT: the anchor falls back to index 0.
func TestBaseIndex_NoSpecFallsBackToZero(t *testing.T) {
	t.Parallel()

	list := []types.SecretVersionsListEntry{
		{VersionId: aws.String("id-2"), VersionStages: []string{"AWSPREVIOUS"}},
		{VersionId: aws.String("id-1"), VersionStages: []string{}},
	}

	idx, err := baseIndex(list, awssecretversion.AbsoluteSpec{})
	require.NoError(t, err)
	assert.Equal(t, 0, idx)
}

func TestSortNewestFirst(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input []types.SecretVersionsListEntry
		want  []string // expected VersionId order
	}{
		{
			name: "distinct timestamps newest first",
			input: []types.SecretVersionsListEntry{
				{VersionId: aws.String("old"), CreatedDate: aws.Time(base)},
				{VersionId: aws.String("new"), CreatedDate: aws.Time(base.Add(time.Hour))},
			},
			want: []string{"new", "old"},
		},
		{
			name: "equal timestamps tie-break by version-id descending",
			input: []types.SecretVersionsListEntry{
				{VersionId: aws.String("aaa"), CreatedDate: aws.Time(base)},
				{VersionId: aws.String("bbb"), CreatedDate: aws.Time(base)},
			},
			want: []string{"bbb", "aaa"},
		},
		{
			name: "both nil CreatedDate tie-break by version-id descending",
			input: []types.SecretVersionsListEntry{
				{VersionId: aws.String("aaa")},
				{VersionId: aws.String("bbb")},
			},
			want: []string{"bbb", "aaa"},
		},
		{
			name: "nil CreatedDate sorts after a dated version",
			input: []types.SecretVersionsListEntry{
				{VersionId: aws.String("undated")},
				{VersionId: aws.String("dated"), CreatedDate: aws.Time(base)},
			},
			want: []string{"dated", "undated"},
		},
		{
			name: "dated version sorts before a nil-CreatedDate one",
			input: []types.SecretVersionsListEntry{
				{VersionId: aws.String("dated"), CreatedDate: aws.Time(base)},
				{VersionId: aws.String("undated")},
			},
			want: []string{"dated", "undated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sortNewestFirst(tt.input)
			got := lo.Map(tt.input, func(v types.SecretVersionsListEntry, _ int) string {
				return aws.ToString(v.VersionId)
			})
			assert.Equal(t, tt.want, got)
		})
	}
}

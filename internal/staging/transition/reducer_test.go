package transition //nolint:testpackage // Internal tests sharing test fixtures with executor_test.go

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/maputil"
)

// testExistingValue is declared in executor_test.go

func TestReduceEntry_Add(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		state           EntryState
		action          EntryActionAdd
		wantState       EntryStagedState
		wantDiscardTags bool
		wantError       error
	}{
		{
			name: "NotStaged -> Create",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateNotStaged{},
			},
			action:    EntryActionAdd{Value: "new-value"},
			wantState: EntryStagedStateCreate{DraftValue: "new-value"},
		},
		{
			name: "Create -> Create (update draft)",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateCreate{DraftValue: "old-value"},
			},
			action:    EntryActionAdd{Value: "new-value"},
			wantState: EntryStagedStateCreate{DraftValue: "new-value"},
		},
		{
			name: "Update -> ERROR (inconsistent state: staged for update but resource not on AWS)",
			state: EntryState{
				CurrentValue: nil, // Resource doesn't exist on AWS
				StagedState:  EntryStagedStateUpdate{DraftValue: "updated"},
			},
			action:    EntryActionAdd{Value: "new-value"},
			wantState: EntryStagedStateUpdate{DraftValue: "updated"},
			wantError: ErrCannotAddToUpdate,
		},
		{
			name: "Delete -> ERROR (inconsistent state: staged for delete but resource not on AWS)",
			state: EntryState{
				CurrentValue: nil, // Resource doesn't exist on AWS
				StagedState:  EntryStagedStateDelete{},
			},
			action:    EntryActionAdd{Value: "new-value"},
			wantState: EntryStagedStateDelete{},
			wantError: ErrCannotAddToDelete,
		},
		{
			name: "ERROR when resource already exists on AWS",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateNotStaged{},
			},
			action:    EntryActionAdd{Value: "new-value"},
			wantState: EntryStagedStateNotStaged{},
			wantError: ErrCannotAddToExisting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ReduceEntry(tt.state, tt.action)
			assert.Equal(t, tt.wantState, result.NewState.StagedState)
			assert.Equal(t, tt.wantDiscardTags, result.DiscardTags)
			assert.Equal(t, tt.wantError, result.Error)
		})
	}
}

func TestReduceEntry_Edit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		state           EntryState
		action          EntryActionEdit
		wantState       EntryStagedState
		wantDiscardTags bool
		wantError       error
	}{
		{
			name: "NotStaged -> Update (value != AWS)",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateNotStaged{},
			},
			action:    EntryActionEdit{Value: "new-value"},
			wantState: EntryStagedStateUpdate{DraftValue: "new-value"},
		},
		{
			name: "NotStaged -> NotStaged (value == AWS, auto-skip)",
			state: EntryState{
				CurrentValue: lo.ToPtr("same-value"),
				StagedState:  EntryStagedStateNotStaged{},
			},
			action:    EntryActionEdit{Value: "same-value"},
			wantState: EntryStagedStateNotStaged{},
		},
		{
			name: "NotStaged with nil CurrentValue -> Update",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateNotStaged{},
			},
			action:    EntryActionEdit{Value: "new-value"},
			wantState: EntryStagedStateUpdate{DraftValue: "new-value"},
		},
		{
			name: "Create -> Create (update draft)",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateCreate{DraftValue: "old"},
			},
			action:    EntryActionEdit{Value: "new"},
			wantState: EntryStagedStateCreate{DraftValue: "new"},
		},
		{
			name: "Update -> Update (value != AWS)",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateUpdate{DraftValue: "old"},
			},
			action:    EntryActionEdit{Value: "new"},
			wantState: EntryStagedStateUpdate{DraftValue: "new"},
		},
		{
			name: "Update -> NotStaged (value == AWS, auto-unstage)",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateUpdate{DraftValue: "something"},
			},
			action:    EntryActionEdit{Value: "current"},
			wantState: EntryStagedStateNotStaged{},
		},
		{
			name: "Delete -> ERROR",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateDelete{},
			},
			action:    EntryActionEdit{Value: "new"},
			wantState: EntryStagedStateDelete{},
			wantError: ErrCannotEditDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ReduceEntry(tt.state, tt.action)
			assert.Equal(t, tt.wantState, result.NewState.StagedState)
			assert.Equal(t, tt.wantDiscardTags, result.DiscardTags)
			assert.Equal(t, tt.wantError, result.Error)
		})
	}
}

func TestReduceEntry_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		state           EntryState
		wantState       EntryStagedState
		wantDiscardTags bool
	}{
		{
			name: "NotStaged -> Delete",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateNotStaged{},
			},
			wantState: EntryStagedStateDelete{},
		},
		{
			name: "Create -> NotStaged (unstage, also unstage tags)",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateCreate{DraftValue: "draft"},
			},
			wantState:       EntryStagedStateNotStaged{},
			wantDiscardTags: true,
		},
		{
			name: "Update -> Delete",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateUpdate{DraftValue: "updated"},
			},
			wantState: EntryStagedStateDelete{},
		},
		{
			name: "Delete -> Delete (no-op)",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateDelete{},
			},
			wantState: EntryStagedStateDelete{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ReduceEntry(tt.state, EntryActionDelete{})
			assert.Equal(t, tt.wantState, result.NewState.StagedState)
			assert.Equal(t, tt.wantDiscardTags, result.DiscardTags)
			assert.NoError(t, result.Error)
		})
	}
}

func TestReduceEntry_Reset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     EntryState
		wantState EntryStagedState
	}{
		{
			name: "NotStaged -> NotStaged",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateNotStaged{},
			},
			wantState: EntryStagedStateNotStaged{},
		},
		{
			name: "Create -> NotStaged",
			state: EntryState{
				CurrentValue: nil,
				StagedState:  EntryStagedStateCreate{DraftValue: "draft"},
			},
			wantState: EntryStagedStateNotStaged{},
		},
		{
			name: "Update -> NotStaged",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateUpdate{DraftValue: "updated"},
			},
			wantState: EntryStagedStateNotStaged{},
		},
		{
			name: "Delete -> NotStaged",
			state: EntryState{
				CurrentValue: lo.ToPtr("current"),
				StagedState:  EntryStagedStateDelete{},
			},
			wantState: EntryStagedStateNotStaged{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ReduceEntry(tt.state, EntryActionReset{})
			assert.Equal(t, tt.wantState, result.NewState.StagedState)
			assert.False(t, result.DiscardTags)
			assert.NoError(t, result.Error)
		})
	}
}

func TestReduceTag_Tag(t *testing.T) {
	t.Parallel()

	existingValue := testExistingValue
	tests := []struct {
		name          string
		entryState    EntryState
		stagedTags    StagedTags
		action        TagActionTag
		wantStagedTag StagedTags
		wantError     error
	}{
		{
			name:       "Add tag to empty staged tags",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{},
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Auto-skip tag matching AWS",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{"env": "prod"},
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Update tag with different value than AWS",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{"env": "dev"},
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Tag removes from ToUnset",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet("env"),
			},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{},
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "ERROR when entry is Delete",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateDelete{}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{},
			},
			wantStagedTag: StagedTags{},
			wantError:     ErrCannotTagDelete,
		},
		{
			name:       "Allow tag when entry is Create",
			entryState: EntryState{CurrentValue: nil, StagedState: EntryStagedStateCreate{DraftValue: "new"}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{},
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Allow tag when entry is Update",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateUpdate{DraftValue: "updated"}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{},
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Auto-skip also clears ToUnset (cancel previous untag)",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet("env"), // Previously staged untag
			},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{"env": "prod"}, // Same as AWS
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet[string](), // ToUnset should be cleared
			},
		},
		{
			name:       "Nil CurrentAWSTags disables auto-skip",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: nil, // nil disables auto-skip
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{"env": "prod"}, // Should be staged since we don't know AWS state
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "ERROR when resource not found and not staged",
			entryState: EntryState{CurrentValue: nil, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionTag{
				Tags:           map[string]string{"env": "prod"},
				CurrentAWSTags: map[string]string{},
			},
			wantStagedTag: StagedTags{},
			wantError:     ErrCannotTagNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ReduceTag(tt.entryState, tt.stagedTags, tt.action)
			assert.Equal(t, tt.wantStagedTag.ToSet, result.NewStagedTags.ToSet)
			assert.Equal(t, tt.wantStagedTag.ToUnset.Values(), result.NewStagedTags.ToUnset.Values())
			assert.Equal(t, tt.wantError, result.Error)
		})
	}
}

func TestReduceTag_Untag(t *testing.T) {
	t.Parallel()

	existingValue := testExistingValue
	tests := []struct {
		name          string
		entryState    EntryState
		stagedTags    StagedTags
		action        TagActionUntag
		wantStagedTag StagedTags
		wantError     error
	}{
		{
			name:       "Untag existing AWS tag",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet("env"),
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet("env"),
			},
		},
		{
			name:       "Auto-skip untag for non-existent AWS tag",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet[string](),
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Untag removes from ToSet (AWS has tag)",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet("env"),
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet("env"),
			},
		},
		{
			name:       "Untag removes from ToSet only (AWS has no tag, auto-skip)",
			entryState: EntryState{CurrentValue: nil, StagedState: EntryStagedStateCreate{DraftValue: "new"}}, // Creating new resource
			stagedTags: StagedTags{
				ToSet:   map[string]string{"env": "prod"}, // Previously staged tag
				ToUnset: maputil.NewSet[string](),
			},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet[string](), // AWS has no tags (resource doesn't exist)
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},      // ToSet cleared (tag cancelled)
				ToUnset: maputil.NewSet[string](), // ToUnset empty (nothing to unstage on AWS)
			},
		},
		{
			name:       "ERROR when entry is Delete",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateDelete{}},
			stagedTags: StagedTags{},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet("env"),
			},
			wantStagedTag: StagedTags{},
			wantError:     ErrCannotUntagDelete,
		},
		{
			name:       "Auto-skip also clears ToSet (cancel previous tag)",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{
				ToSet:   map[string]string{"env": "prod"}, // Previously staged tag
				ToUnset: maputil.NewSet[string](),
			},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet[string](), // Not on AWS
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{}, // ToSet should be cleared
				ToUnset: maputil.NewSet[string](),
			},
		},
		{
			name:       "Nil CurrentAWSTagKeys disables auto-skip",
			entryState: EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: nil, // nil disables auto-skip
			},
			wantStagedTag: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet("env"), // Should be staged since we don't know AWS state
			},
		},
		{
			name:       "ERROR when resource not found and not staged",
			entryState: EntryState{CurrentValue: nil, StagedState: EntryStagedStateNotStaged{}},
			stagedTags: StagedTags{},
			action: TagActionUntag{
				Keys:              maputil.NewSet("env"),
				CurrentAWSTagKeys: maputil.NewSet("env"),
			},
			wantStagedTag: StagedTags{},
			wantError:     ErrCannotUntagNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ReduceTag(tt.entryState, tt.stagedTags, tt.action)
			assert.Equal(t, tt.wantStagedTag.ToSet, result.NewStagedTags.ToSet)
			assert.Equal(t, tt.wantStagedTag.ToUnset.Values(), result.NewStagedTags.ToUnset.Values())
			assert.Equal(t, tt.wantError, result.Error)
		})
	}
}

func TestStagedTags_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stagedTags StagedTags
		want       bool
	}{
		{
			name:       "Empty",
			stagedTags: StagedTags{},
			want:       true,
		},
		{
			name: "Empty with initialized maps",
			stagedTags: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet[string](),
			},
			want: true,
		},
		{
			name: "Has ToSet",
			stagedTags: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet[string](),
			},
			want: false,
		},
		{
			name: "Has ToUnset",
			stagedTags: StagedTags{
				ToSet:   map[string]string{},
				ToUnset: maputil.NewSet("env"),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.stagedTags.IsEmpty())
		})
	}
}

func TestStagedTags_Clone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		stagedTags  StagedTags
		wantToSet   map[string]string
		wantToUnset []string
	}{
		{
			name:        "Empty with nil maps",
			stagedTags:  StagedTags{},
			wantToSet:   map[string]string{},
			wantToUnset: []string{},
		},
		{
			name: "With values",
			stagedTags: StagedTags{
				ToSet:   map[string]string{"env": "prod"},
				ToUnset: maputil.NewSet("deprecated"),
			},
			wantToSet:   map[string]string{"env": "prod"},
			wantToUnset: []string{"deprecated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloned := tt.stagedTags.Clone()
			assert.Equal(t, tt.wantToSet, cloned.ToSet)
			assert.ElementsMatch(t, tt.wantToUnset, cloned.ToUnset.Values())

			// Ensure maps are initialized (not nil)
			assert.NotNil(t, cloned.ToSet)
			assert.NotNil(t, cloned.ToUnset)

			// Ensure it's a deep copy by modifying cloned and checking original is unchanged
			cloned.ToSet["new"] = "value"
			cloned.ToUnset.Add("new-key")
			assert.NotContains(t, tt.stagedTags.ToSet, "new")
		})
	}
}

func TestReduceEntry_Delete_NotFound(t *testing.T) {
	t.Parallel()

	// Test case: CurrentValue=nil + NotStaged -> ERROR (resource not found)
	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateNotStaged{},
	}
	result := ReduceEntry(state, EntryActionDelete{})
	assert.Equal(t, ErrCannotDeleteNotFound, result.Error)
	assert.Equal(t, EntryStagedStateNotStaged{}, result.NewState.StagedState)
}

func TestReduceEntry_Delete_InconsistentState(t *testing.T) {
	t.Parallel()

	// Test edge case: CurrentValue=nil + Delete -> Delete (no-op)
	// This is an inconsistent state in practice, but the reducer handles it gracefully
	state := EntryState{
		CurrentValue: nil,                      // Resource doesn't exist on AWS
		StagedState:  EntryStagedStateDelete{}, // But staged for delete
	}
	result := ReduceEntry(state, EntryActionDelete{})
	// This should still return error since the resource doesn't exist
	assert.Equal(t, ErrCannotDeleteNotFound, result.Error)
	assert.Equal(t, EntryStagedStateDelete{}, result.NewState.StagedState)
}

// Test interface marker methods for coverage.
func TestEntryAction_Marker(t *testing.T) {
	t.Parallel()
	// These tests exist to cover the sealed interface marker methods
	var _ EntryAction = EntryActionAdd{}

	var _ EntryAction = EntryActionEdit{}

	var _ EntryAction = EntryActionDelete{}

	var _ EntryAction = EntryActionReset{}

	// Call the marker methods directly
	EntryActionAdd{Value: "test"}.isEntryAction()
	EntryActionEdit{Value: "test"}.isEntryAction()
	EntryActionDelete{}.isEntryAction()
	EntryActionReset{}.isEntryAction()
}

func TestTagAction_Marker(t *testing.T) {
	t.Parallel()
	// These tests exist to cover the sealed interface marker methods
	var _ TagAction = TagActionTag{}

	var _ TagAction = TagActionUntag{}

	// Call the marker methods directly
	TagActionTag{Tags: map[string]string{}}.isTagAction()
	TagActionUntag{Keys: maputil.NewSet[string]()}.isTagAction()
}

func TestEntryStagedState_Marker(t *testing.T) {
	t.Parallel()
	// These tests exist to cover the sealed interface marker methods
	var _ EntryStagedState = EntryStagedStateNotStaged{}

	var _ EntryStagedState = EntryStagedStateCreate{}

	var _ EntryStagedState = EntryStagedStateUpdate{}

	var _ EntryStagedState = EntryStagedStateDelete{}

	// Call the marker methods directly
	EntryStagedStateNotStaged{}.isEntryStagedState()
	EntryStagedStateCreate{DraftValue: "test"}.isEntryStagedState()
	EntryStagedStateUpdate{DraftValue: "test"}.isEntryStagedState()
	EntryStagedStateDelete{}.isEntryStagedState()
}

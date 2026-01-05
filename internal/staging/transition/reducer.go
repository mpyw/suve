package transition

import "errors"

// Error definitions for transition failures.
var (
	ErrCannotAddToUpdate = errors.New("cannot add: already staged for update")
	ErrCannotAddToDelete = errors.New("cannot add: already staged for deletion")
	ErrCannotEditDelete  = errors.New("cannot edit: staged for deletion, reset first")
	ErrCannotTagDelete   = errors.New("cannot tag: resource staged for deletion")
	ErrCannotUntagDelete = errors.New("cannot untag: resource staged for deletion")
)

// EntryTransitionResult holds the result of an entry state transition.
type EntryTransitionResult struct {
	NewState    EntryState
	DiscardTags bool // True if tags should also be unstaged (e.g., when deleting a CREATE)
	Error       error
}

// TagTransitionResult holds the result of a tag state transition.
type TagTransitionResult struct {
	NewStagedTags StagedTags
	Error         error
}

// ReduceEntry applies an entry action to produce a new state.
func ReduceEntry(state EntryState, action EntryAction) EntryTransitionResult {
	var result EntryTransitionResult
	switch a := action.(type) {
	case EntryActionAdd:
		result = reduceAdd(state, a)
	case EntryActionEdit:
		result = reduceEdit(state, a)
	case EntryActionDelete:
		result = reduceDelete(state)
	case EntryActionReset:
		result = reduceReset(state)
	}
	return result
}

// ReduceTag applies a tag action to produce new staged tags.
func ReduceTag(entryState EntryStagedState, stagedTags StagedTags, action TagAction) TagTransitionResult {
	var result TagTransitionResult
	switch a := action.(type) {
	case TagActionTag:
		result = reduceTag(entryState, stagedTags, a)
	case TagActionUntag:
		result = reduceUntag(entryState, stagedTags, a)
	}
	return result
}

// reduceAdd handles the ADD action.
//
// Transition rules:
//   - NotStaged → Create     (stage as create)
//   - Create    → Create     (update draft value)
//   - Update    → ERROR      (cannot add to update)
//   - Delete    → ERROR      (cannot add to delete)
func reduceAdd(state EntryState, action EntryActionAdd) EntryTransitionResult {
	var err error
	switch state.StagedState.(type) {
	case EntryStagedStateNotStaged, EntryStagedStateCreate:
		state.StagedState = EntryStagedStateCreate{DraftValue: action.Value}
	case EntryStagedStateUpdate:
		err = ErrCannotAddToUpdate
	case EntryStagedStateDelete:
		err = ErrCannotAddToDelete
	}
	return EntryTransitionResult{NewState: state, Error: err}
}

// reduceEdit handles the EDIT action.
//
// Transition rules:
//   - NotStaged → Update     (if value != AWS)
//   - NotStaged → NotStaged  (if value == AWS, auto-skip)
//   - Create    → Create     (update draft value)
//   - Update    → Update     (if value != AWS)
//   - Update    → NotStaged  (if value == AWS, auto-unstage)
//   - Delete    → ERROR      (must reset first to edit)
func reduceEdit(state EntryState, action EntryActionEdit) EntryTransitionResult {
	var err error
	switch state.StagedState.(type) {
	case EntryStagedStateNotStaged:
		// Auto-skip if value matches AWS current value
		if state.CurrentValue == nil || *state.CurrentValue != action.Value {
			state.StagedState = EntryStagedStateUpdate{DraftValue: action.Value}
		}
	case EntryStagedStateCreate:
		state.StagedState = EntryStagedStateCreate{DraftValue: action.Value}
	case EntryStagedStateUpdate:
		// Auto-unstage if value matches AWS current value
		if state.CurrentValue != nil && *state.CurrentValue == action.Value {
			state.StagedState = EntryStagedStateNotStaged{}
		} else {
			state.StagedState = EntryStagedStateUpdate{DraftValue: action.Value}
		}
	case EntryStagedStateDelete:
		err = ErrCannotEditDelete
	}
	return EntryTransitionResult{NewState: state, Error: err}
}

// reduceDelete handles the DELETE action.
//
// Transition rules:
//   - NotStaged → Delete     (stage for deletion)
//   - Create    → NotStaged  (unstage, also unstage tags)
//   - Update    → Delete     (convert to delete)
//   - Delete    → Delete     (no-op)
func reduceDelete(state EntryState) EntryTransitionResult {
	var discardTags bool
	switch state.StagedState.(type) {
	case EntryStagedStateNotStaged, EntryStagedStateUpdate:
		state.StagedState = EntryStagedStateDelete{}
	case EntryStagedStateCreate:
		// Discard tags too - resource was never created
		state.StagedState = EntryStagedStateNotStaged{}
		discardTags = true
	case EntryStagedStateDelete:
		// no-op
	}
	return EntryTransitionResult{NewState: state, DiscardTags: discardTags}
}

// reduceReset handles the RESET action.
//
// Transition rules:
//   - Any → NotStaged  (unstage entry only, tags preserved)
func reduceReset(state EntryState) EntryTransitionResult {
	state.StagedState = EntryStagedStateNotStaged{}
	return EntryTransitionResult{NewState: state}
}

// reduceTag handles the TAG action.
//
// Transition rules:
//   - Entry=Delete      → ERROR  (cannot tag resource staged for deletion)
//   - AWS same          → skip   (auto-skip tags matching AWS, unless CurrentAWSTags is nil)
//   - Otherwise         → ToSet  (add to staged tags)
func reduceTag(entryState EntryStagedState, stagedTags StagedTags, action TagActionTag) TagTransitionResult {
	// Block tagging if entry is staged for deletion
	if _, isDelete := entryState.(EntryStagedStateDelete); isDelete {
		return TagTransitionResult{
			NewStagedTags: stagedTags,
			Error:         ErrCannotTagDelete,
		}
	}

	// Clone existing staged tags
	cloned := stagedTags.Clone()

	// Process tags with auto-skip for matching AWS values
	for key, value := range action.Tags {
		// Always clear from ToUnset (user wants to set this key)
		cloned.ToUnset.Remove(key)

		// Auto-skip if value matches AWS current value (unless CurrentAWSTags is nil)
		if action.CurrentAWSTags != nil {
			if awsValue, exists := action.CurrentAWSTags[key]; exists && awsValue == value {
				delete(cloned.ToSet, key)
				continue
			}
		}
		cloned.ToSet[key] = value
	}

	return TagTransitionResult{NewStagedTags: cloned}
}

// reduceUntag handles the UNTAG action.
//
// Transition rules:
//   - Entry=Delete      → ERROR  (cannot untag resource staged for deletion)
//   - Not on AWS        → skip   (auto-skip non-existent tags, unless CurrentAWSTagKeys is nil)
//   - Otherwise         → ToUnset (add to staged untags)
func reduceUntag(entryState EntryStagedState, stagedTags StagedTags, action TagActionUntag) TagTransitionResult {
	// Block untagging if entry is staged for deletion
	if _, isDelete := entryState.(EntryStagedStateDelete); isDelete {
		return TagTransitionResult{
			NewStagedTags: stagedTags,
			Error:         ErrCannotUntagDelete,
		}
	}

	// Clone existing staged tags
	cloned := stagedTags.Clone()

	// Process untag keys with auto-skip for non-existent AWS tags
	for key := range action.Keys {
		// Always clear from ToSet (user wants to remove this key)
		delete(cloned.ToSet, key)

		// Auto-skip if tag doesn't exist on AWS (unless CurrentAWSTagKeys is nil)
		if action.CurrentAWSTagKeys != nil && !action.CurrentAWSTagKeys.Contains(key) {
			cloned.ToUnset.Remove(key)
			continue
		}
		cloned.ToUnset.Add(key)
	}

	return TagTransitionResult{NewStagedTags: cloned}
}

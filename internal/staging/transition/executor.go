package transition

import (
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
)

// Executor executes state transitions and persists results to the store.
type Executor struct {
	Store staging.StoreReadWriter
}

// NewExecutor creates a new Executor.
func NewExecutor(store staging.StoreReadWriter) *Executor {
	return &Executor{Store: store}
}

// EntryExecuteOptions holds optional metadata for entry execution.
type EntryExecuteOptions struct {
	BaseModifiedAt *time.Time // Base modification time for conflict detection
	Description    *string    // Optional description for the staged entry
}

// ExecuteEntry executes an entry action and persists the result.
func (e *Executor) ExecuteEntry(service staging.Service, name string, state EntryState, action EntryAction, opts *EntryExecuteOptions) (EntryTransitionResult, error) {
	result := ReduceEntry(state, action)
	if result.Error != nil {
		return result, result.Error
	}

	// Persist the new state
	if err := e.persistEntryState(service, name, state, result, opts); err != nil {
		return result, err
	}

	// Handle tag unstaging if needed
	if result.DiscardTags {
		if err := e.Store.UnstageTag(service, name); err != nil {
			// Ignore ErrNotStaged - it's fine if there were no tags
			if !errors.Is(err, staging.ErrNotStaged) {
				return result, err
			}
		}
	}

	return result, nil
}

// ExecuteTag executes a tag action and persists the result.
func (e *Executor) ExecuteTag(service staging.Service, name string, entryState EntryStagedState, stagedTags StagedTags, action TagAction, baseModifiedAt *time.Time) (TagTransitionResult, error) {
	result := ReduceTag(entryState, stagedTags, action)
	if result.Error != nil {
		return result, result.Error
	}

	// Persist the new staged tags
	if err := e.persistTagState(service, name, result.NewStagedTags, baseModifiedAt); err != nil {
		return result, err
	}

	return result, nil
}

// persistEntryState saves the entry state to the store.
func (e *Executor) persistEntryState(service staging.Service, name string, oldState EntryState, result EntryTransitionResult, opts *EntryExecuteOptions) error {
	var err error
	switch s := result.NewState.StagedState.(type) {
	case EntryStagedStateNotStaged:
		// Unstage if was previously staged
		if _, wasStaged := oldState.StagedState.(EntryStagedStateNotStaged); !wasStaged {
			err = e.Store.UnstageEntry(service, name)
		}

	case EntryStagedStateCreate:
		entry := staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr(s.DraftValue),
			StagedAt:  time.Now(),
		}
		if opts != nil && opts.Description != nil {
			entry.Description = opts.Description
		}
		err = e.Store.StageEntry(service, name, entry)

	case EntryStagedStateUpdate:
		entry := staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr(s.DraftValue),
			StagedAt:  time.Now(),
		}
		if opts != nil {
			entry.BaseModifiedAt = opts.BaseModifiedAt
			if opts.Description != nil {
				entry.Description = opts.Description
			}
		}
		err = e.Store.StageEntry(service, name, entry)

	case EntryStagedStateDelete:
		entry := staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		}
		if opts != nil {
			entry.BaseModifiedAt = opts.BaseModifiedAt
		}
		err = e.Store.StageEntry(service, name, entry)
	}
	return err
}

// persistTagState saves the tag state to the store.
func (e *Executor) persistTagState(service staging.Service, name string, stagedTags StagedTags, baseModifiedAt *time.Time) error {
	// If no tags to stage, unstage
	if stagedTags.IsEmpty() {
		err := e.Store.UnstageTag(service, name)
		if errors.Is(err, staging.ErrNotStaged) {
			return nil // Already not staged, that's fine
		}
		return err
	}

	// Stage the tags
	return e.Store.StageTag(service, name, staging.TagEntry{
		Add:            stagedTags.ToSet,
		Remove:         stagedTags.ToUnset,
		StagedAt:       time.Now(),
		BaseModifiedAt: baseModifiedAt,
	})
}

// LoadEntryState loads the current entry state from the store and AWS info.
func LoadEntryState(store staging.StoreReader, service staging.Service, name string, currentAWSValue *string) (EntryState, error) {
	state, _, err := LoadEntryStateWithMetadata(store, service, name, currentAWSValue)
	return state, err
}

// LoadEntryStateWithMetadata loads the current entry state and returns BaseModifiedAt metadata.
// BaseModifiedAt is used for conflict detection when applying changes.
func LoadEntryStateWithMetadata(store staging.StoreReader, service staging.Service, name string, currentAWSValue *string) (EntryState, *time.Time, error) {
	stagedEntry, err := store.GetEntry(service, name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return EntryState{}, nil, err
	}

	state := EntryState{
		CurrentValue: currentAWSValue,
		StagedState:  EntryStagedStateNotStaged{},
	}

	var baseModifiedAt *time.Time
	if stagedEntry != nil {
		baseModifiedAt = stagedEntry.BaseModifiedAt
		switch stagedEntry.Operation {
		case staging.OperationCreate:
			state.StagedState = EntryStagedStateCreate{
				DraftValue: lo.FromPtr(stagedEntry.Value),
			}
		case staging.OperationUpdate:
			state.StagedState = EntryStagedStateUpdate{
				DraftValue: lo.FromPtr(stagedEntry.Value),
			}
		case staging.OperationDelete:
			state.StagedState = EntryStagedStateDelete{}
		}
	}

	return state, baseModifiedAt, nil
}

// LoadStagedTags loads the current staged tags from the store.
func LoadStagedTags(store staging.StoreReader, service staging.Service, name string) (StagedTags, *time.Time, error) {
	tagEntry, err := store.GetTag(service, name)
	if err != nil {
		if errors.Is(err, staging.ErrNotStaged) {
			return StagedTags{}, nil, nil
		}
		return StagedTags{}, nil, err
	}

	return StagedTags{
		ToSet:   tagEntry.Add,
		ToUnset: tagEntry.Remove,
	}, tagEntry.BaseModifiedAt, nil
}

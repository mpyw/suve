package transition

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// Executor executes state transitions and persists results to the store.
type Executor struct {
	Store store.ReadWriteOperator
}

// NewExecutor creates a new Executor.
func NewExecutor(store store.ReadWriteOperator) *Executor {
	return &Executor{Store: store}
}

// EntryExecutorOptions holds optional metadata for entry execution.
type EntryExecutorOptions struct {
	BaseModifiedAt *time.Time       // Base modification time for conflict detection
	Description    *string          // Optional description for the staged entry
	ValueType      domain.ValueType // Provider-neutral value type (e.g. an SSM Parameter Store type); empty means unset
}

// ExecuteEntry executes an entry action and persists the result.
func (e *Executor) ExecuteEntry(
	ctx context.Context,
	service staging.Service,
	key staging.EntryKey,
	state EntryState,
	action EntryAction,
	opts *EntryExecutorOptions,
) (EntryTransitionResult, error) {
	result := ReduceEntry(state, action)
	if result.Error != nil {
		return result, result.Error
	}

	// Persist the new state
	if err := e.persistEntryState(ctx, service, key, state, result, opts); err != nil {
		return result, err
	}

	// Handle tag unstaging if needed
	if result.DiscardTags {
		if err := e.Store.UnstageTag(ctx, service, key); err != nil {
			// Ignore ErrNotStaged - it's fine if there were no tags
			if !errors.Is(err, staging.ErrNotStaged) {
				return result, err
			}
		}
	}

	return result, nil
}

// ExecuteTag executes a tag action and persists the result.
func (e *Executor) ExecuteTag(
	ctx context.Context,
	service staging.Service,
	key staging.EntryKey,
	entryState EntryState,
	stagedTags StagedTags,
	action TagAction,
	baseModifiedAt *time.Time,
) (TagTransitionResult, error) {
	result := ReduceTag(entryState, stagedTags, action)
	if result.Error != nil {
		return result, result.Error
	}

	// Persist the new staged tags
	if err := e.persistTagState(ctx, service, key, result.NewStagedTags, baseModifiedAt); err != nil {
		return result, err
	}

	return result, nil
}

// persistEntryState saves the entry state to the store.
func (e *Executor) persistEntryState(
	ctx context.Context,
	service staging.Service,
	key staging.EntryKey,
	oldState EntryState,
	result EntryTransitionResult,
	opts *EntryExecutorOptions,
) error {
	var err error

	switch s := result.NewState.StagedState.(type) {
	case EntryStagedStateNotStaged:
		// Unstage if was previously staged
		if _, wasStaged := oldState.StagedState.(EntryStagedStateNotStaged); !wasStaged {
			err = e.Store.UnstageEntry(ctx, service, key)
		}

	case EntryStagedStateCreate:
		entry := staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr(s.DraftValue),
			StagedAt:  time.Now(),
		}
		if opts != nil {
			entry.ValueType = opts.ValueType
			if opts.Description != nil {
				entry.Description = opts.Description
			}
		}

		err = e.Store.StageEntry(ctx, service, key, entry)

	case EntryStagedStateUpdate:
		entry := staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr(s.DraftValue),
			StagedAt:  time.Now(),
		}
		if opts != nil {
			entry.BaseModifiedAt = opts.BaseModifiedAt
			entry.ValueType = opts.ValueType

			if opts.Description != nil {
				entry.Description = opts.Description
			}
		}

		err = e.Store.StageEntry(ctx, service, key, entry)

	case EntryStagedStateDelete:
		entry := staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		}
		if opts != nil {
			entry.BaseModifiedAt = opts.BaseModifiedAt
		}

		err = e.Store.StageEntry(ctx, service, key, entry)
	}

	return err
}

// persistTagState saves the tag state to the store.
func (e *Executor) persistTagState(
	ctx context.Context,
	service staging.Service,
	key staging.EntryKey,
	stagedTags StagedTags,
	baseModifiedAt *time.Time,
) error {
	// If no tags to stage, unstage
	if stagedTags.IsEmpty() {
		err := e.Store.UnstageTag(ctx, service, key)
		if errors.Is(err, staging.ErrNotStaged) {
			return nil // Already not staged, that's fine
		}

		return err
	}

	// Stage the tags
	return e.Store.StageTag(ctx, service, key, staging.TagEntry{
		Add:            stagedTags.ToSet,
		Remove:         stagedTags.ToUnset,
		StagedAt:       time.Now(),
		BaseModifiedAt: baseModifiedAt,
	})
}

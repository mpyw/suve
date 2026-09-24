package transition

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// LoadEntryState loads the current entry state from the store and the remote value.
func LoadEntryState(
	ctx context.Context,
	store store.ReadOperator,
	service staging.Service,
	key staging.EntryKey,
	currentRemoteValue *string,
) (EntryState, error) {
	state, _, err := LoadEntryStateWithMetadata(ctx, store, service, key, currentRemoteValue)

	return state, err
}

// LoadEntryStateWithMetadata loads the current entry state and returns BaseModifiedAt metadata.
// BaseModifiedAt is used for conflict detection when applying changes.
func LoadEntryStateWithMetadata(
	ctx context.Context,
	store store.ReadOperator,
	service staging.Service,
	key staging.EntryKey,
	currentRemoteValue *string,
) (EntryState, *time.Time, error) {
	stagedEntry, err := store.GetEntry(ctx, service, key)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return EntryState{}, nil, err
	}

	state := EntryState{
		CurrentValue: currentRemoteValue,
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
func LoadStagedTags(ctx context.Context, store store.ReadOperator, service staging.Service, key staging.EntryKey) (StagedTags, *time.Time, error) {
	tagEntry, err := store.GetTag(ctx, service, key)
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

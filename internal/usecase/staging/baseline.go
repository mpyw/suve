package staging

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/transition"
)

// BaselineInput holds input for getting baseline value.
type BaselineInput struct {
	Key staging.EntryKey
}

// BaselineOutput holds the baseline value for editing.
type BaselineOutput struct {
	Value        string
	IsStagedEdit bool // True if the baseline is from a staged edit (not the remote store)
}

// Baseline returns the baseline value for editing (staged value if exists, otherwise from the remote store).
func (u *EditUseCase) Baseline(ctx context.Context, input BaselineInput) (*BaselineOutput, error) {
	service := u.Strategy.Service()

	// Check if already staged
	stagedEntry, err := u.Store.GetEntry(ctx, service, input.Key)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	if stagedEntry != nil {
		switch stagedEntry.Operation {
		case staging.OperationCreate, staging.OperationUpdate:
			return &BaselineOutput{
				Value:        lo.FromPtr(stagedEntry.Value),
				IsStagedEdit: true,
			}, nil
		case staging.OperationDelete:
			// BLOCKED: Cannot edit something staged for deletion
			return nil, transition.ErrCannotEditDelete
		}
	}

	// Not staged → fetch from the remote store
	return u.fetchBaselineFromRemote(ctx, input.Key.Name)
}

// fetchBaselineFromRemote fetches the baseline value from the remote store.
func (u *EditUseCase) fetchBaselineFromRemote(ctx context.Context, name string) (*BaselineOutput, error) {
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, err
	}

	return &BaselineOutput{Value: result.Value}, nil
}

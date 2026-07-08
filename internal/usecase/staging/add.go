package staging

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/transition"
)

// AddInput holds input for the add use case. Key identifies the item by name and
// (Azure App Configuration) namespace; the namespace is empty for the
// null/default namespace and every other provider.
type AddInput struct {
	Key         staging.EntryKey
	Value       string
	Description string
}

// AddOutput holds the result of the add use case.
type AddOutput struct {
	Name string
}

// AddUseCase executes add operations.
type AddUseCase struct {
	Strategy staging.EditStrategy
	Store    store.ReadWriteOperator
}

// Execute runs the add use case.
func (u *AddUseCase) Execute(ctx context.Context, input AddInput) (*AddOutput, error) {
	service := u.Strategy.Service()

	// Parse and validate name
	name, err := u.Strategy.ParseName(input.Key.Name)
	if err != nil {
		return nil, err
	}

	// Check if resource already exists on AWS
	var currentValue *string

	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		// ResourceNotFoundError means resource doesn't exist - that's expected for add
		if notFoundErr := (*staging.ResourceNotFoundError)(nil); !errors.As(err, &notFoundErr) {
			return nil, err
		}
		// Resource doesn't exist, currentValue remains nil
	} else {
		// Resource exists, set currentValue
		currentValue = &result.Value
	}

	key := staging.EntryKey{Name: name, Namespace: input.Key.Namespace}

	// Load current state with AWS existence check
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, key, currentValue)
	if err != nil {
		return nil, err
	}

	// Execute the transition
	executor := transition.NewExecutor(u.Store)

	opts := &transition.EntryExecuteOptions{}
	if input.Description != "" {
		opts.Description = &input.Description
	}

	_, err = executor.ExecuteEntry(ctx, service, key, entryState, transition.EntryActionAdd{Value: input.Value}, opts)
	if err != nil {
		return nil, err
	}

	return &AddOutput{Name: name}, nil
}

// DraftInput holds input for getting draft (staged create) value.
type DraftInput struct {
	Key staging.EntryKey
}

// DraftOutput holds the draft value if any.
type DraftOutput struct {
	Value    string
	IsStaged bool
}

// Draft returns the currently staged create value (draft) for re-editing.
func (u *AddUseCase) Draft(ctx context.Context, input DraftInput) (*DraftOutput, error) {
	service := u.Strategy.Service()

	// Parse and validate name
	name, err := u.Strategy.ParseName(input.Key.Name)
	if err != nil {
		return nil, err
	}

	stagedEntry, err := u.Store.GetEntry(ctx, service, staging.EntryKey{Name: name, Namespace: input.Key.Namespace})
	if err != nil {
		if errors.Is(err, staging.ErrNotStaged) {
			return &DraftOutput{IsStaged: false}, nil
		}

		return nil, err
	}

	if stagedEntry.Operation == staging.OperationCreate {
		return &DraftOutput{
			Value:    lo.FromPtr(stagedEntry.Value),
			IsStaged: true,
		}, nil
	}

	return &DraftOutput{IsStaged: false}, nil
}

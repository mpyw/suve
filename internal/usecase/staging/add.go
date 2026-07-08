package staging

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/transition"
)

// AddInput holds input for the add use case.
type AddInput struct {
	Name        string
	Value       string
	Description string
	// Namespace is the Azure App Configuration namespace to stage under (the
	// label axis); empty is the null/default namespace and the only value for
	// every other provider.
	Namespace string
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
	name, err := u.Strategy.ParseName(input.Name)
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

	// Load current state with AWS existence check
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, name, input.Namespace, currentValue)
	if err != nil {
		return nil, err
	}

	// Execute the transition
	executor := transition.NewExecutor(u.Store)

	opts := &transition.EntryExecuteOptions{}
	if input.Description != "" {
		opts.Description = &input.Description
	}

	_, err = executor.ExecuteEntry(ctx, service, name, input.Namespace, entryState, transition.EntryActionAdd{Value: input.Value}, opts)
	if err != nil {
		return nil, err
	}

	return &AddOutput{Name: name}, nil
}

// DraftInput holds input for getting draft (staged create) value.
type DraftInput struct {
	Name      string
	Namespace string
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
	name, err := u.Strategy.ParseName(input.Name)
	if err != nil {
		return nil, err
	}

	stagedEntry, err := u.Store.GetEntry(ctx, service, name, input.Namespace)
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

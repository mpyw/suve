package staging

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
)

// AddInput holds input for the add use case.
type AddInput struct {
	Name        string
	Value       string
	Description string
	Tags        map[string]string
}

// AddOutput holds the result of the add use case.
type AddOutput struct {
	Name string
}

// AddUseCase executes add operations.
type AddUseCase struct {
	Strategy staging.Parser
	Store    staging.StoreReadWriter
}

// Execute runs the add use case.
func (u *AddUseCase) Execute(_ context.Context, input AddInput) (*AddOutput, error) {
	service := u.Strategy.Service()

	// Parse and validate name
	name, err := u.Strategy.ParseName(input.Name)
	if err != nil {
		return nil, err
	}

	// Stage the change with OperationCreate
	entry := staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(input.Value),
		StagedAt:  time.Now(),
	}
	if input.Description != "" {
		entry.Description = &input.Description
	}
	if len(input.Tags) > 0 {
		entry.Tags = input.Tags
	}
	if err := u.Store.Stage(service, name, entry); err != nil {
		return nil, err
	}

	return &AddOutput{Name: name}, nil
}

// DraftInput holds input for getting draft (staged create) value.
type DraftInput struct {
	Name string
}

// DraftOutput holds the draft value if any.
type DraftOutput struct {
	Value    string
	IsStaged bool
}

// Draft returns the currently staged create value (draft) for re-editing.
func (u *AddUseCase) Draft(_ context.Context, input DraftInput) (*DraftOutput, error) {
	service := u.Strategy.Service()

	// Parse and validate name
	name, err := u.Strategy.ParseName(input.Name)
	if err != nil {
		return nil, err
	}

	stagedEntry, err := u.Store.Get(service, name)
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

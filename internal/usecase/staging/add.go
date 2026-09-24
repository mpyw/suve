package staging

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/mpyw/suve/internal/domain"
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
	// ValueType is the provider-neutral value type for the staged create. It is
	// only meaningful on the AWS SSM Parameter Store axis (String / SecureString /
	// StringList); other providers leave it empty. An empty value applies as
	// plaintext, so callers that do not set it keep the prior behavior.
	ValueType domain.ValueType
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

	// Reject non-UTF-8 values at ingestion (argv, $EDITOR, provider prefill)
	if !utf8.ValidString(input.Value) {
		return nil, ErrValueNotUTF8
	}

	// Parse and validate name
	name, err := u.Strategy.ParseName(input.Key.Name)
	if err != nil {
		return nil, err
	}

	// Check if resource already exists remotely
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

	// Resolve the staged value type. When the caller specifies none, preserve a
	// previously staged type so re-staging the create (e.g. re-editing the draft
	// without --secure) never silently downgrades a SecureString to plain String.
	valueType := input.ValueType
	if valueType == "" {
		existing, gerr := u.Store.GetEntry(ctx, service, key)

		switch {
		case gerr == nil:
			valueType = existing.ValueType
		case !errors.Is(gerr, staging.ErrNotStaged):
			return nil, gerr
		}
	}

	// Load current state with the remote existence check
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, key, currentValue)
	if err != nil {
		return nil, err
	}

	// Execute the transition
	executor := transition.NewExecutor(u.Store)

	opts := &transition.EntryExecuteOptions{ValueType: valueType}
	if input.Description != "" {
		opts.Description = &input.Description
	}

	_, err = executor.ExecuteEntry(ctx, service, key, entryState, transition.EntryActionAdd{Value: input.Value}, opts)
	if err != nil {
		return nil, err
	}

	return &AddOutput{Name: name}, nil
}

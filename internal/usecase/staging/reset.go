package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/transition"
)

// ResetInput holds input for the reset use case.
type ResetInput struct {
	Spec string // Name with optional version spec
	All  bool   // Reset all staged items for this service
}

// ResetResultType represents the type of reset result.
type ResetResultType int

const (
	ResetResultUnstaged ResetResultType = iota
	ResetResultUnstagedAll
	ResetResultRestored
	ResetResultNotStaged
	ResetResultNothingStaged
	ResetResultSkipped // Restore was skipped because value matches current AWS
)

// ResetOutput holds the result of the reset use case.
type ResetOutput struct {
	Type         ResetResultType
	Name         string
	VersionLabel string
	Count        int // Number of items unstaged (for UnstagedAll)
	ServiceName  string
	ItemName     string
}

// ResetUseCase executes reset operations.
type ResetUseCase struct {
	Parser  staging.Parser
	Fetcher staging.ResetStrategy
	Store   store.ReadWriteOperator
}

// Execute runs the reset use case.
func (u *ResetUseCase) Execute(ctx context.Context, input ResetInput) (*ResetOutput, error) {
	serviceName := u.Parser.ServiceName()
	itemName := u.Parser.ItemName()

	if input.All {
		return u.unstageAll(ctx, serviceName, itemName)
	}

	name, hasVersion, err := u.Parser.ParseSpec(input.Spec)
	if err != nil {
		return nil, err
	}

	if hasVersion {
		return u.restore(ctx, input.Spec, name)
	}

	return u.unstage(ctx, name, serviceName, itemName)
}

func (u *ResetUseCase) unstageAll(ctx context.Context, serviceName, itemName string) (*ResetOutput, error) {
	service := u.Parser.Service()

	staged, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}

	stagedTags, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}

	serviceStaged := staged[service]
	serviceStagedTags := stagedTags[service]
	totalCount := len(serviceStaged) + len(serviceStagedTags)

	// Always call UnstageAll to trigger daemon auto-shutdown check
	// even if there's nothing staged for this service
	// Use hint for context-aware shutdown message
	if hinted, ok := u.Store.(store.HintedUnstager); ok {
		if err := hinted.UnstageAllWithHint(ctx, service, store.HintReset); err != nil {
			return nil, err
		}
	} else {
		if err := u.Store.UnstageAll(ctx, service); err != nil {
			return nil, err
		}
	}

	if totalCount == 0 {
		return &ResetOutput{
			Type:        ResetResultNothingStaged,
			ServiceName: serviceName,
		}, nil
	}

	return &ResetOutput{
		Type:        ResetResultUnstagedAll,
		Count:       totalCount,
		ServiceName: serviceName,
		ItemName:    itemName,
	}, nil
}

func (u *ResetUseCase) unstage(ctx context.Context, name, serviceName, itemName string) (*ResetOutput, error) {
	service := u.Parser.Service()

	// Load current state (nil CurrentValue since we don't care about AWS state for reset)
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, name, nil)
	if err != nil {
		return nil, err
	}

	// Check if already not staged
	if _, isNotStaged := entryState.StagedState.(transition.EntryStagedStateNotStaged); isNotStaged {
		return &ResetOutput{
			Type: ResetResultNotStaged,
			Name: name,
		}, nil
	}

	// Execute the reset transition
	executor := transition.NewExecutor(u.Store)
	if _, err := executor.ExecuteEntry(ctx, service, name, entryState, transition.EntryActionReset{}, nil); err != nil {
		return nil, err
	}

	return &ResetOutput{
		Type: ResetResultUnstaged,
		Name: name,
	}, nil
}

func (u *ResetUseCase) restore(ctx context.Context, spec, name string) (*ResetOutput, error) {
	service := u.Parser.Service()

	if u.Fetcher == nil {
		return nil, errors.New("reset strategy required for restore operation")
	}

	value, versionLabel, err := u.Fetcher.FetchVersion(ctx, spec)
	if err != nil {
		return nil, err
	}

	// Fetch current AWS value for auto-skip detection
	fetchResult, err := u.Fetcher.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, err
	}
	// Always use the value pointer - empty string is a valid AWS value
	currentValue := &fetchResult.Value

	// Load current state with AWS value for auto-skip
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, name, currentValue)
	if err != nil {
		return nil, err
	}

	// Check if value matches current AWS (would be auto-skipped)
	_, wasNotStaged := entryState.StagedState.(transition.EntryStagedStateNotStaged)

	// Execute the edit transition with the restored value
	executor := transition.NewExecutor(u.Store)

	result, err := executor.ExecuteEntry(ctx, service, name, entryState, transition.EntryActionEdit{Value: value}, nil)
	if err != nil {
		return nil, err
	}

	// Check if auto-skipped (was NotStaged and still NotStaged)
	_, isNotStaged := result.NewState.StagedState.(transition.EntryStagedStateNotStaged)
	if wasNotStaged && isNotStaged {
		return &ResetOutput{
			Type:         ResetResultSkipped,
			Name:         name,
			VersionLabel: versionLabel,
		}, nil
	}

	return &ResetOutput{
		Type:         ResetResultRestored,
		Name:         name,
		VersionLabel: versionLabel,
	}, nil
}

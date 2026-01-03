package staging

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
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
	Store   staging.StoreReadWriter
}

// Execute runs the reset use case.
func (u *ResetUseCase) Execute(ctx context.Context, input ResetInput) (*ResetOutput, error) {
	serviceName := u.Parser.ServiceName()
	itemName := u.Parser.ItemName()

	if input.All {
		return u.unstageAll(serviceName, itemName)
	}

	name, hasVersion, err := u.Parser.ParseSpec(input.Spec)
	if err != nil {
		return nil, err
	}

	if hasVersion {
		return u.restore(ctx, input.Spec, name)
	}

	return u.unstage(name, serviceName, itemName)
}

func (u *ResetUseCase) unstageAll(serviceName, itemName string) (*ResetOutput, error) {
	service := u.Parser.Service()

	staged, err := u.Store.ListEntries(service)
	if err != nil {
		return nil, err
	}

	serviceStaged := staged[service]
	if len(serviceStaged) == 0 {
		return &ResetOutput{
			Type:        ResetResultNothingStaged,
			ServiceName: serviceName,
		}, nil
	}

	if err := u.Store.UnstageAll(service); err != nil {
		return nil, err
	}

	return &ResetOutput{
		Type:        ResetResultUnstagedAll,
		Count:       len(serviceStaged),
		ServiceName: serviceName,
		ItemName:    itemName,
	}, nil
}

func (u *ResetUseCase) unstage(name, serviceName, itemName string) (*ResetOutput, error) {
	service := u.Parser.Service()

	_, err := u.Store.GetEntry(service, name)
	if errors.Is(err, staging.ErrNotStaged) {
		return &ResetOutput{
			Type: ResetResultNotStaged,
			Name: name,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	if err := u.Store.UnstageEntry(service, name); err != nil {
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

	if err := u.Store.StageEntry(service, name, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(value),
		StagedAt:  time.Now(),
	}); err != nil {
		return nil, err
	}

	return &ResetOutput{
		Type:         ResetResultRestored,
		Name:         name,
		VersionLabel: versionLabel,
	}, nil
}

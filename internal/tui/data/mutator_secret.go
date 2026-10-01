//declscope:namespace mutator
//
// secretMutator is the secret-service Mutator. The mutator*.go files are one
// unit: mutator.go holds the interface and the staging helpers both
// mutators share.

package data

import (
	"context"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/usecase/tagging"
)

// secretMutator routes secret writes. Secrets have no namespace axis, so the
// key namespace is always empty.
type secretMutator struct {
	svcCap       capability.ServiceCapability
	store        provider.Store
	newStrategy  StrategyBuilder
	stagingStore StagingStoreResolver
}

// NewSecretMutator builds a secret Mutator over a resolved secret store.
func NewSecretMutator(
	svcCap capability.ServiceCapability,
	store provider.Store,
	newStrategy StrategyBuilder,
	stagingStore StagingStoreResolver,
) Mutator {
	return &secretMutator{svcCap: svcCap, store: store, newStrategy: newStrategy, stagingStore: stagingStore}
}

func (m *secretMutator) Capability() capability.ServiceCapability { return m.svcCap }

func (m *secretMutator) Create(
	ctx context.Context, key StagedKey, value, _, description string, staged bool,
) (WriteOutcome, error) {
	if staged {
		return m.stage(func(strategy staging.FullStrategy, st store.ReadWriteOperator) (WriteOutcome, error) {
			// Secrets have no value-type axis, so no value type is staged.
			return mutatorStageEntry(ctx, strategy, st, key, value, description, "", mutatorStageOpCreate)
		})
	}

	uc := &secret.CreateUseCase{Writer: m.store}
	_, err := uc.Execute(ctx, secret.CreateInput{Name: key.Name, Value: value, Description: description})

	return WriteOutcome{}, err
}

func (m *secretMutator) Update(
	ctx context.Context, key StagedKey, value, _, description string, staged bool,
) (WriteOutcome, error) {
	if staged {
		return m.stage(func(strategy staging.FullStrategy, st store.ReadWriteOperator) (WriteOutcome, error) {
			// Secrets have no value-type axis, so no value type is staged.
			return mutatorStageEntry(ctx, strategy, st, key, value, description, "", mutatorStageOpEdit)
		})
	}

	uc := &secret.UpdateUseCase{Store: m.store}
	_, err := uc.Execute(ctx, secret.UpdateInput{Name: key.Name, Value: value, Description: description})

	return WriteOutcome{}, err
}

func (m *secretMutator) Delete(
	ctx context.Context, key StagedKey, force bool, recoveryWindow int, staged bool,
) (WriteOutcome, error) {
	if staged {
		return m.stage(func(strategy staging.FullStrategy, st store.ReadWriteOperator) (WriteOutcome, error) {
			return mutatorStageDelete(ctx, strategy, st, key, force, recoveryWindow)
		})
	}

	uc := &secret.DeleteUseCase{Store: m.store}

	var options []provider.DeleteOption
	if force {
		options = append(options, provider.ForceDelete{})
	}

	_, err := uc.Execute(ctx, secret.DeleteInput{Name: key.Name, Options: options})

	return WriteOutcome{}, err
}

func (m *secretMutator) AddTag(
	ctx context.Context, key StagedKey, tagKey, tagValue string, staged bool,
) (WriteOutcome, error) {
	if staged {
		return m.stage(func(strategy staging.FullStrategy, st store.ReadWriteOperator) (WriteOutcome, error) {
			return mutatorStageAddTag(ctx, strategy, st, key, tagKey, tagValue)
		})
	}

	uc := &tagging.UseCase{Tagger: m.store}

	return WriteOutcome{}, uc.Execute(ctx, tagging.Input{Name: key.Name, Add: map[string]string{tagKey: tagValue}})
}

func (m *secretMutator) RemoveTag(
	ctx context.Context, key StagedKey, tagKey string, staged bool,
) (WriteOutcome, error) {
	if staged {
		return m.stage(func(strategy staging.FullStrategy, st store.ReadWriteOperator) (WriteOutcome, error) {
			return mutatorStageRemoveTag(ctx, strategy, st, key, tagKey)
		})
	}

	uc := &tagging.UseCase{Tagger: m.store}

	return WriteOutcome{}, uc.Execute(ctx, tagging.Input{Name: key.Name, Remove: []string{tagKey}})
}

func (m *secretMutator) Restore(ctx context.Context, name string) (WriteOutcome, error) {
	restorer, ok := m.store.(provider.Restorer)
	if !ok {
		return WriteOutcome{}, ErrRestoreUnsupported
	}

	uc := &secret.RestoreUseCase{Restorer: restorer}
	_, err := uc.Execute(ctx, secret.RestoreInput{Name: name})

	return WriteOutcome{}, err
}

// stage resolves the staging store + strategy for the secret service and runs
// fn against them.
func (m *secretMutator) stage(
	fn func(staging.FullStrategy, store.ReadWriteOperator) (WriteOutcome, error),
) (WriteOutcome, error) {
	st, err := m.stagingStore()
	if err != nil {
		return WriteOutcome{}, err
	}

	strategy, err := m.newStrategy(m.store)
	if err != nil {
		return WriteOutcome{}, err
	}

	return fn(strategy, st)
}

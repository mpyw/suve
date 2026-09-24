//declscope:namespace mutator
//
// paramMutator is the param-service Mutator. The mutator*.go files are one
// unit: mutator.go holds the interface and the staging helpers both
// mutators share.

package data

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/namespaces"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/usecase/tagging"
)

// paramMutator routes param writes. For Azure App Configuration each write
// targets one concrete (key, namespace): the namespace is validated
// (validateParamNamespace parity) and the store/strategy are resolved for it.
type paramMutator struct {
	svcCap       capability.ServiceCapability
	resolveStore StoreResolver
	newStrategy  StrategyBuilder
	stagingStore StagingStoreResolver
	namespaced   bool
}

// NewParamMutator builds a param Mutator. resolveStore returns the param store
// for a namespace (namespace ignored for non-App-Configuration providers);
// newStrategy builds the staged-write strategy over a store; stagingStore
// resolves the cached staging store (nil when the service has no staging).
func NewParamMutator(
	svcCap capability.ServiceCapability,
	resolveStore StoreResolver,
	newStrategy StrategyBuilder,
	stagingStore StagingStoreResolver,
) Mutator {
	return &paramMutator{
		svcCap:       svcCap,
		resolveStore: resolveStore,
		newStrategy:  newStrategy,
		stagingStore: stagingStore,
		namespaced:   svcCap.HasNamespaces,
	}
}

func (m *paramMutator) Capability() capability.ServiceCapability { return m.svcCap }

// literalNamespace validates and decodes a namespace for the App Configuration
// service (rejecting a `*`/`,` filter value); for every other provider it
// returns the namespace unchanged.
func (m *paramMutator) literalNamespace(ns string) (string, error) {
	if !m.namespaced {
		return ns, nil
	}

	return namespaces.Literal(ns)
}

func (m *paramMutator) Create(
	ctx context.Context, key StagedKey, value, typeLabel, description string, staged bool,
) (WriteOutcome, error) {
	ns, err := m.literalNamespace(key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	if staged {
		return m.stageEntry(
			ctx, StagedKey{Name: key.Name, Namespace: ns}, value, description, mutatorStagedValueType(typeLabel), mutatorStageOpCreate,
		)
	}

	store, err := m.resolveStore(ctx, ns)
	if err != nil {
		return WriteOutcome{}, err
	}

	valueType := paramtype.Parse(typeLabel)

	// Immediate create is a create-or-update (upsert), matching the GUI (ParamSet)
	// and the CLI (`param set`): try create first, and if the parameter already
	// exists fall back to update instead of surfacing the raw ErrAlreadyExists.
	// The staged branch above is untouched — stage-time add validation is unchanged.
	createUC := &param.CreateUseCase{Writer: store}

	_, err = createUC.Execute(ctx, param.CreateInput{
		Name: key.Name, Value: value, Type: valueType, Description: description,
	})
	if err == nil {
		return WriteOutcome{}, nil
	}

	if !errors.Is(err, provider.ErrAlreadyExists) {
		return WriteOutcome{}, err
	}

	updateUC := &param.UpdateUseCase{Store: store}

	_, err = updateUC.Execute(ctx, param.UpdateInput{
		Name: key.Name, Value: value, Type: valueType, Description: description,
	})
	if err != nil {
		return WriteOutcome{}, err
	}

	return WriteOutcome{Updated: true}, nil
}

func (m *paramMutator) Update(
	ctx context.Context, key StagedKey, value, typeLabel, description string, staged bool,
) (WriteOutcome, error) {
	ns, err := m.literalNamespace(key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	if staged {
		return m.stageEntry(
			ctx, StagedKey{Name: key.Name, Namespace: ns}, value, description, mutatorStagedValueType(typeLabel), mutatorStageOpEdit,
		)
	}

	store, err := m.resolveStore(ctx, ns)
	if err != nil {
		return WriteOutcome{}, err
	}

	uc := &param.UpdateUseCase{Store: store}

	_, err = uc.Execute(ctx, param.UpdateInput{
		Name: key.Name, Value: value, Type: paramtype.Parse(typeLabel), Description: description,
	})

	return WriteOutcome{}, err
}

func (m *paramMutator) Delete(
	ctx context.Context, key StagedKey, force bool, recoveryWindow int, staged bool,
) (WriteOutcome, error) {
	ns, err := m.literalNamespace(key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	if staged {
		return m.stageDelete(ctx, StagedKey{Name: key.Name, Namespace: ns}, force, recoveryWindow)
	}

	store, err := m.resolveStore(ctx, ns)
	if err != nil {
		return WriteOutcome{}, err
	}

	uc := &param.DeleteUseCase{Store: store}
	_, err = uc.Execute(ctx, param.DeleteInput{Name: key.Name})

	return WriteOutcome{}, err
}

func (m *paramMutator) AddTag(
	ctx context.Context, key StagedKey, tagKey, tagValue string, staged bool,
) (WriteOutcome, error) {
	ns, err := m.literalNamespace(key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	if staged {
		return m.stageAddTag(ctx, StagedKey{Name: key.Name, Namespace: ns}, tagKey, tagValue)
	}

	store, err := m.resolveStore(ctx, ns)
	if err != nil {
		return WriteOutcome{}, err
	}

	uc := &tagging.UseCase{Tagger: store}

	return WriteOutcome{}, uc.Execute(ctx, tagging.Input{Name: key.Name, Add: map[string]string{tagKey: tagValue}})
}

func (m *paramMutator) RemoveTag(
	ctx context.Context, key StagedKey, tagKey string, staged bool,
) (WriteOutcome, error) {
	ns, err := m.literalNamespace(key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	if staged {
		return m.stageRemoveTag(ctx, StagedKey{Name: key.Name, Namespace: ns}, tagKey)
	}

	store, err := m.resolveStore(ctx, ns)
	if err != nil {
		return WriteOutcome{}, err
	}

	uc := &tagging.UseCase{Tagger: store}

	return WriteOutcome{}, uc.Execute(ctx, tagging.Input{Name: key.Name, Remove: []string{tagKey}})
}

func (m *paramMutator) Restore(context.Context, string) (WriteOutcome, error) {
	return WriteOutcome{}, ErrRestoreUnsupported
}

// stageStrategy resolves the staged-write strategy and store for a namespace.
func (m *paramMutator) stageStrategy(
	ctx context.Context, namespace string,
) (staging.FullStrategy, store.ReadWriteOperator, error) {
	st, err := m.stagingStore()
	if err != nil {
		return nil, nil, err
	}

	provStore, err := m.resolveStore(ctx, namespace)
	if err != nil {
		return nil, nil, err
	}

	strategy, err := m.newStrategy(provStore)
	if err != nil {
		return nil, nil, err
	}

	return strategy, st, nil
}

func (m *paramMutator) stageEntry(
	ctx context.Context, key StagedKey, value, description string, valueType domain.ValueType, op mutatorStageOp,
) (WriteOutcome, error) {
	strategy, st, err := m.stageStrategy(ctx, key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	return mutatorStageEntry(ctx, strategy, st, key, value, description, valueType, op)
}

func (m *paramMutator) stageDelete(
	ctx context.Context, key StagedKey, force bool, recoveryWindow int,
) (WriteOutcome, error) {
	strategy, st, err := m.stageStrategy(ctx, key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	return mutatorStageDelete(ctx, strategy, st, key, force, recoveryWindow)
}

func (m *paramMutator) stageAddTag(ctx context.Context, key StagedKey, tagKey, tagValue string) (WriteOutcome, error) {
	strategy, st, err := m.stageStrategy(ctx, key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	return mutatorStageAddTag(ctx, strategy, st, key, tagKey, tagValue)
}

func (m *paramMutator) stageRemoveTag(ctx context.Context, key StagedKey, tagKey string) (WriteOutcome, error) {
	strategy, st, err := m.stageStrategy(ctx, key.Namespace)
	if err != nil {
		return WriteOutcome{}, err
	}

	return mutatorStageRemoveTag(ctx, strategy, st, key, tagKey)
}

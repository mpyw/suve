package data

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramtype"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/usecase/secret"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// Mutator is the write-path seam the mutation dialogs depend on. Every method is
// provider-neutral and routes to either the direct param/secret use cases
// (immediate) or the internal/usecase/staging use cases (staged), per the staged
// flag. The concrete param/secret mutators pair a per-scope-cached staging store
// with a scope-paired strategy, mirroring the GUI's serviceStrategyScoped
// discipline. Keeping the dialogs behind this interface lets a test drive them
// with a providermock-backed Mutator without touching a real cloud or keychain.
type Mutator interface {
	// Capability returns the service capability so a dialog can gate its controls
	// (mode toggle, type select, force/recovery rows, restore).
	Capability() capability.ServiceCapability
	// Create stages or applies a create for a new entry. typeLabel is the SSM type
	// display name for a typed param service (ignored elsewhere).
	Create(ctx context.Context, key StagedKey, value, typeLabel, description string, staged bool) (WriteOutcome, error)
	// Update stages or applies an update to an existing entry.
	Update(ctx context.Context, key StagedKey, value, typeLabel, description string, staged bool) (WriteOutcome, error)
	// Delete stages or applies a delete. force/recoveryWindow apply only to a
	// service with HasForceDelete/HasRecoveryWindow (AWS secret).
	Delete(ctx context.Context, key StagedKey, force bool, recoveryWindow int, staged bool) (WriteOutcome, error)
	// AddTag stages or applies a tag add/update.
	AddTag(ctx context.Context, key StagedKey, tagKey, tagValue string, staged bool) (WriteOutcome, error)
	// RemoveTag stages or applies a tag removal.
	RemoveTag(ctx context.Context, key StagedKey, tagKey string, staged bool) (WriteOutcome, error)
	// Restore applies an immediate restore of a soft-deleted entry (there is no
	// staged restore); it errors when the provider offers none.
	Restore(ctx context.Context, name string) (WriteOutcome, error)
}

// =============================================================================
// Param mutator
// =============================================================================

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

	return aznamespace.Literal(ns)
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

	uc := &param.TagUseCase{Tagger: store}

	return WriteOutcome{}, uc.Execute(ctx, param.TagInput{Name: key.Name, Add: map[string]string{tagKey: tagValue}})
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

	uc := &param.TagUseCase{Tagger: store}

	return WriteOutcome{}, uc.Execute(ctx, param.TagInput{Name: key.Name, Remove: []string{tagKey}})
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

	return m.newStrategy(provStore), st, nil
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

// =============================================================================
// Secret mutator
// =============================================================================

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

	uc := &secret.TagUseCase{Tagger: m.store}

	return WriteOutcome{}, uc.Execute(ctx, secret.TagInput{Name: key.Name, Add: map[string]string{tagKey: tagValue}})
}

func (m *secretMutator) RemoveTag(
	ctx context.Context, key StagedKey, tagKey string, staged bool,
) (WriteOutcome, error) {
	if staged {
		return m.stage(func(strategy staging.FullStrategy, st store.ReadWriteOperator) (WriteOutcome, error) {
			return mutatorStageRemoveTag(ctx, strategy, st, key, tagKey)
		})
	}

	uc := &secret.TagUseCase{Tagger: m.store}

	return WriteOutcome{}, uc.Execute(ctx, secret.TagInput{Name: key.Name, Remove: []string{tagKey}})
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

	return fn(m.newStrategy(m.store), st)
}

// =============================================================================
// Shared staged-write helpers
// =============================================================================

// mutatorStageOp selects the entry transition a staged write performs.
type mutatorStageOp int

const (
	mutatorStageOpCreate mutatorStageOp = iota
	mutatorStageOpEdit
)

// mutatorStagedValueType maps a Type display label to the value type to stage. The
// dialog passes an empty label when it presents no Type control (a secret, an App
// Configuration setting, or the staging-review edit that cannot seed the current
// type); an empty value means "no explicit type", which the staging apply treats
// as plaintext for a create and as "preserve the existing type" for an edit — so
// an edit from a surface with no Type control never downgrades a staged
// SecureString. A non-empty label (an offered Type select) is mapped through
// paramtype.Parse so the chosen type is stored and applied.
func mutatorStagedValueType(typeLabel string) domain.ValueType {
	if typeLabel == "" {
		return ""
	}

	return paramtype.Parse(typeLabel)
}

// mutatorStageEntry stages a create or edit and maps the use-case outcome (Skipped/
// Unstaged for edit) onto the neutral WriteOutcome. valueType carries the AWS SSM
// param value type (String / SecureString / StringList) into the staging store so
// a staged SecureString create/edit applies as SecureString; an empty value
// preserves the existing type on edit (and applies plaintext on create). It is
// empty for providers with no value-type axis (secret, App Configuration).
func mutatorStageEntry(
	ctx context.Context, strategy staging.FullStrategy, st store.ReadWriteOperator,
	key StagedKey, value, description string, valueType domain.ValueType, op mutatorStageOp,
) (WriteOutcome, error) {
	entryKey := staging.EntryKey{Name: key.Name, Namespace: key.Namespace}

	if op == mutatorStageOpCreate {
		uc := &stagingusecase.AddUseCase{Strategy: strategy, Store: st}
		_, err := uc.Execute(ctx, stagingusecase.AddInput{
			Key: entryKey, Value: value, Description: description, ValueType: valueType,
		})

		return WriteOutcome{}, err
	}

	uc := &stagingusecase.EditUseCase{Strategy: strategy, Store: st}

	out, err := uc.Execute(ctx, stagingusecase.EditInput{
		Key: entryKey, Value: value, Description: description, ValueType: valueType,
	})
	if err != nil {
		return WriteOutcome{}, err
	}

	return WriteOutcome{Skipped: out.Skipped, Unstaged: out.Unstaged}, nil
}

// mutatorStageDelete stages a delete and reports the auto-unstage outcome.
func mutatorStageDelete(
	ctx context.Context, strategy staging.FullStrategy, st store.ReadWriteOperator,
	key StagedKey, force bool, recoveryWindow int,
) (WriteOutcome, error) {
	deleteStrategy, ok := any(strategy).(staging.DeleteStrategy)
	if !ok {
		return WriteOutcome{}, errors.New("staging strategy does not support delete")
	}

	uc := &stagingusecase.DeleteUseCase{Strategy: deleteStrategy, Store: st}

	out, err := uc.Execute(ctx, stagingusecase.DeleteInput{
		Key:            staging.EntryKey{Name: key.Name, Namespace: key.Namespace},
		Force:          force,
		RecoveryWindow: recoveryWindow,
	})
	if err != nil {
		return WriteOutcome{}, err
	}

	return WriteOutcome{Unstaged: out.Unstaged}, nil
}

// mutatorStageAddTag stages a tag add/update.
func mutatorStageAddTag(
	ctx context.Context, strategy staging.FullStrategy, st store.ReadWriteOperator,
	key StagedKey, tagKey, tagValue string,
) (WriteOutcome, error) {
	uc := &stagingusecase.TagUseCase{Strategy: strategy, Store: st}

	_, err := uc.Tag(ctx, stagingusecase.TagInput{
		Key:  staging.EntryKey{Name: key.Name, Namespace: key.Namespace},
		Tags: map[string]string{tagKey: tagValue},
	})

	return WriteOutcome{}, err
}

// mutatorStageRemoveTag stages a tag removal.
func mutatorStageRemoveTag(
	ctx context.Context, strategy staging.FullStrategy, st store.ReadWriteOperator,
	key StagedKey, tagKey string,
) (WriteOutcome, error) {
	uc := &stagingusecase.TagUseCase{Strategy: strategy, Store: st}

	_, err := uc.Untag(ctx, stagingusecase.UntagInput{
		Key:     staging.EntryKey{Name: key.Name, Namespace: key.Namespace},
		TagKeys: maputil.NewSet(tagKey),
	})

	return WriteOutcome{}, err
}

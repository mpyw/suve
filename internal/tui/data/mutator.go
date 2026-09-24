package data

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
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

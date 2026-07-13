package data

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// =============================================================================
// Neutral staging-review output types
// =============================================================================

// StagedDiffType classifies a staged entry's diff row (mirrors the staging
// DiffUseCase's DiffEntryType), so the page can color/label auto-unstaged and
// warning rows distinctly.
type StagedDiffType int

// StagedDiffType values.
const (
	StagedDiffNormal StagedDiffType = iota
	StagedDiffCreate
	StagedDiffAutoUnstaged
	StagedDiffWarning
)

// StagedDiffRow is one staged entry rendered as a Remote-vs-Staged diff. The
// page shows RemoteValue/StagedValue in diff view and StagedValue in value view;
// a delete carries an empty StagedValue.
type StagedDiffRow struct {
	Name        string
	Namespace   string
	Type        StagedDiffType
	Operation   string // "create" / "update" / "delete"
	RemoteValue string
	StagedValue string
	Warning     string
	// Secret reports whether this row's values are secret material (a secret
	// service, or a SecureString param), so the page masks them per-row rather
	// than keying off the section's service axis alone (#677).
	Secret bool
}

// StagedTagRow is one item's staged tag changes: independent +add and −remove
// deltas the page renders as separate cancellable rows.
type StagedTagRow struct {
	Name      string
	Namespace string
	// Adds are the staged tag adds/updates (key=value).
	Adds []Tag
	// Removes are the staged tag removals: the tag key plus the current remote
	// value (empty when unknown), so the row reads "−key (was value)".
	Removes []TagRemoval
}

// TagRemoval is a staged tag removal: the key and its current remote value.
type TagRemoval struct {
	Key   string
	Value string
}

// StagingReview is the full staged picture for one service — entries (as diffs)
// plus independent tag changes.
type StagingReview struct {
	Entries []StagedDiffRow
	Tags    []StagedTagRow
}

// EntryCount is the number of still-staged entry rows (auto-unstaged rows are
// excluded — they were removed from the store during the review).
func (r StagingReview) EntryCount() int {
	return lo.CountBy(r.Entries, func(e StagedDiffRow) bool {
		return e.Type != StagedDiffAutoUnstaged
	})
}

// TagCount is the number of staged tag-change rows.
func (r StagingReview) TagCount() int { return len(r.Tags) }

// AutoUnstaged returns the keys of entries auto-unstaged during the review
// (staged value equalled remote, or the target vanished), for the dismissible
// notice.
func (r StagingReview) AutoUnstaged() []StagedKey {
	return lo.FilterMap(r.Entries, func(e StagedDiffRow, _ int) (StagedKey, bool) {
		return StagedKey{Name: e.Name, Namespace: e.Namespace}, e.Type == StagedDiffAutoUnstaged
	})
}

// ApplyEntryResult is one entry's apply outcome.
type ApplyEntryResult struct {
	Name      string
	Namespace string
	// Status is "created" / "updated" / "deleted" / "failed".
	Status string
	// Error is the cloud-write failure (empty on success).
	Error string
	// UnstageError is set when the cloud write succeeded but the entry could not
	// be cleared from staging afterwards — the page must always surface it.
	UnstageError string
}

// ApplyTagResult is one item's tag-apply outcome.
type ApplyTagResult struct {
	Name         string
	Namespace    string
	Adds         []Tag
	Removes      []string
	Error        string
	UnstageError string
}

// StagingApplyResult is the aggregated result of applying a service's staged
// changes.
type StagingApplyResult struct {
	// ServiceLabel is the service's display name for the results header.
	ServiceLabel string
	Entries      []ApplyEntryResult
	Tags         []ApplyTagResult
	// Conflicts are the labels of entries rejected because remote changed after
	// staging (empty unless conflict detection tripped).
	Conflicts []string
}

// StagingResetType mirrors the staging ResetUseCase's ResetResultType so the
// page can voice the exact outcome.
type StagingResetType int

// StagingResetType values (mirror stagingusecase.ResetResultType).
const (
	StagingResetUnstaged StagingResetType = iota
	StagingResetUnstagedAll
	StagingResetRestored
	StagingResetNotStaged
	StagingResetNothingStaged
	StagingResetSkipped
	StagingResetUnstagedTag
)

// StagingResetResult is the outcome of resetting a service.
type StagingResetResult struct {
	Type         StagingResetType
	Count        int
	ServiceLabel string
}

// StagingService is the review/apply/reset seam the staging page depends on for
// one service. It wraps the internal/usecase/staging use cases over a
// per-scope-cached staging store paired with a scope-matched strategy, mirroring
// the GUI's staging methods. Keeping the page behind this interface lets a test
// drive it over providermock + an in-memory staging store without a keychain.
type StagingService interface {
	// Service is the internal key ("param" / "secret").
	Service() string
	// Label is the display name for the section/results header (e.g. "Key Vault").
	Label() string
	// Capability gates the section's controls.
	Capability() capability.ServiceCapability
	// Review returns the staged entries (as diffs) and tag changes; it may
	// auto-unstage entries whose staged value now equals remote.
	Review(ctx context.Context) (StagingReview, error)
	// Apply applies the service's staged changes. A conflict rejection or a
	// per-entry failure returns a POPULATED result (the detail is in its fields),
	// not an error; only a hard store failure returns a non-nil error.
	Apply(ctx context.Context, ignoreConflicts bool) (StagingApplyResult, error)
	// Reset unstages every staged change for the service.
	Reset(ctx context.Context) (StagingResetResult, error)
	// Unstage removes one item's staged entry and its staged tags.
	Unstage(ctx context.Context, key StagedKey) error
	// CancelAddTag drops one staged tag add.
	CancelAddTag(ctx context.Context, key StagedKey, tagKey string) error
	// CancelRemoveTag drops one staged tag removal.
	CancelRemoveTag(ctx context.Context, key StagedKey, tagKey string) error
}

// StagingResources bundle the resolved staging store and strategy for one
// service. Store and Strategy MUST be paired to the same scope (the
// serviceStrategyScoped discipline).
type StagingResources struct {
	Store    store.ReadWriteOperator
	Strategy staging.FullStrategy
	// StrategyFor resolves a per-namespace strategy for Azure App Configuration,
	// whose settings share one staging store across namespaces; nil for every
	// other provider/service (the single Strategy handles all).
	StrategyFor func(namespace string) (staging.FullStrategy, error)
}

// StagingResolver lazily builds a service's staging resources. It is invoked
// inside the async Review/Apply/Reset commands (never at page construction), so
// touching the keychain/registry never blocks the update loop; a key-loss
// hard-fail surfaces as the returned error.
type StagingResolver func(ctx context.Context) (StagingResources, error)

// stagingService is the concrete StagingService over the staging use cases.
type stagingService struct {
	service staging.Service
	key     string // "param" / "secret"
	label   string
	svcCap  capability.ServiceCapability
	resolve StagingResolver
}

// NewStagingService builds a StagingService for one service. resolve lazily
// yields the scope-paired store and strategy.
func NewStagingService(svcCap capability.ServiceCapability, label string, resolve StagingResolver) StagingService {
	return &stagingService{
		service: staging.Service(svcCap.Service),
		key:     svcCap.Service,
		label:   label,
		svcCap:  svcCap,
		resolve: resolve,
	}
}

func (s *stagingService) Service() string                          { return s.key }
func (s *stagingService) Label() string                            { return s.label }
func (s *stagingService) Capability() capability.ServiceCapability { return s.svcCap }

func (s *stagingService) Review(ctx context.Context) (StagingReview, error) {
	res, err := s.resolve(ctx)
	if err != nil {
		return StagingReview{}, err
	}

	uc := &stagingusecase.DiffUseCase{Strategy: res.Strategy, Store: res.Store}
	if res.StrategyFor != nil {
		uc.StrategyFor = func(ns string) (staging.DiffStrategy, error) {
			return res.StrategyFor(ns)
		}
	}

	out, err := uc.Execute(ctx, stagingusecase.DiffInput{})
	if err != nil {
		return StagingReview{}, err
	}

	// DiffUseCase appends entries/tags in map-iteration (nondeterministic) order;
	// sort by (name, namespace) so the page — and its goldens — render stably.
	review := StagingReview{
		Entries: lo.Map(out.Entries, func(e stagingusecase.DiffEntry, _ int) StagedDiffRow {
			return StagedDiffRow{
				Name:        e.Name,
				Namespace:   e.Namespace,
				Type:        stagedDiffType(e.Type),
				Operation:   string(e.Operation),
				RemoteValue: e.AWSValue,
				StagedValue: e.StagedValue,
				Warning:     e.Warning,
				Secret:      e.Secret,
			}
		}),
		Tags: lo.Map(out.TagEntries, func(t stagingusecase.DiffTagEntry, _ int) StagedTagRow {
			return StagedTagRow{
				Name:      t.Name,
				Namespace: t.Namespace,
				Adds:      mapToTags(t.Add),
				Removes: sortedRemovals(lo.MapToSlice(t.Remove, func(k, v string) TagRemoval {
					return TagRemoval{Key: k, Value: v}
				})),
			}
		}),
	}

	slices.SortFunc(review.Entries, func(a, b StagedDiffRow) int { return compareKey(a.Name, a.Namespace, b.Name, b.Namespace) })
	slices.SortFunc(review.Tags, func(a, b StagedTagRow) int { return compareKey(a.Name, a.Namespace, b.Name, b.Namespace) })

	return review, nil
}

// compareKey orders two (name, namespace) pairs deterministically.
func compareKey(aName, aNS, bName, bNS string) int {
	if c := strings.Compare(aName, bName); c != 0 {
		return c
	}

	return strings.Compare(aNS, bNS)
}

func (s *stagingService) Apply(ctx context.Context, ignoreConflicts bool) (StagingApplyResult, error) {
	res, err := s.resolve(ctx)
	if err != nil {
		return StagingApplyResult{}, err
	}

	uc := &stagingusecase.ApplyUseCase{Strategy: res.Strategy, Store: res.Store}
	if res.StrategyFor != nil {
		uc.StrategyFor = func(ns string) (staging.ApplyStrategy, error) {
			return res.StrategyFor(ns)
		}
	}

	// A conflict/partial-failure returns a populated output with an error; only a
	// nil output (a store read failure) is a hard error with nothing to show.
	out, err := uc.Execute(ctx, stagingusecase.ApplyInput{IgnoreConflicts: ignoreConflicts})
	if out == nil {
		return StagingApplyResult{}, err
	}

	return s.newApplyResult(out), nil
}

func (s *stagingService) newApplyResult(out *stagingusecase.ApplyOutput) StagingApplyResult {
	return StagingApplyResult{
		ServiceLabel: s.label,
		Conflicts: lo.Map(out.Conflicts, func(k staging.EntryKey, _ int) string {
			return k.Label()
		}),
		Entries: lo.Map(out.EntryResults, func(r stagingusecase.ApplyEntryResult, _ int) ApplyEntryResult {
			entry := ApplyEntryResult{
				Name:      r.Name,
				Namespace: r.Namespace,
				Status:    applyStatusLabel(r.Status),
			}
			if r.Status == stagingusecase.ApplyResultFailed && r.Error != nil {
				entry.Error = r.Error.Error()
			}

			if r.UnstageError != nil {
				entry.UnstageError = r.UnstageError.Error()
			}

			return entry
		}),
		Tags: lo.Map(out.TagResults, func(r stagingusecase.ApplyTagResult, _ int) ApplyTagResult {
			tag := ApplyTagResult{
				Name:      r.Name,
				Namespace: r.Namespace,
				Adds:      mapToTags(r.AddTags),
				Removes:   r.RemoveTag.Values(),
			}
			if r.Error != nil {
				tag.Error = r.Error.Error()
			}

			if r.UnstageError != nil {
				tag.UnstageError = r.UnstageError.Error()
			}

			return tag
		}),
	}
}

func (s *stagingService) Reset(ctx context.Context) (StagingResetResult, error) {
	res, err := s.resolve(ctx)
	if err != nil {
		return StagingResetResult{}, err
	}

	uc := &stagingusecase.ResetUseCase{Parser: res.Strategy, Store: res.Store}

	out, err := uc.Execute(ctx, stagingusecase.ResetInput{All: true})
	if err != nil {
		return StagingResetResult{}, err
	}

	return StagingResetResult{
		Type:         stagingResetType(out.Type),
		Count:        out.Count,
		ServiceLabel: s.label,
	}, nil
}

func (s *stagingService) Unstage(ctx context.Context, key StagedKey) error {
	res, err := s.resolve(ctx)
	if err != nil {
		return err
	}

	entryKey := staging.EntryKey{Name: key.Name, Namespace: key.Namespace}

	if err := res.Store.UnstageEntry(ctx, s.service, entryKey); err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return err
	}

	if err := res.Store.UnstageTag(ctx, s.service, entryKey); err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return err
	}

	return nil
}

func (s *stagingService) CancelAddTag(ctx context.Context, key StagedKey, tagKey string) error {
	return s.editStagedTag(ctx, key, func(tag *staging.TagEntry) {
		delete(tag.Add, tagKey)
	})
}

func (s *stagingService) CancelRemoveTag(ctx context.Context, key StagedKey, tagKey string) error {
	return s.editStagedTag(ctx, key, func(tag *staging.TagEntry) {
		tag.Remove.Remove(tagKey)
	})
}

// editStagedTag loads a staged tag entry, applies edit, then re-stages it (or
// unstages it when nothing is left) — mirroring the GUI's cancel handlers.
func (s *stagingService) editStagedTag(ctx context.Context, key StagedKey, edit func(*staging.TagEntry)) error {
	res, err := s.resolve(ctx)
	if err != nil {
		return err
	}

	entryKey := staging.EntryKey{Name: key.Name, Namespace: key.Namespace}

	tag, err := res.Store.GetTag(ctx, s.service, entryKey)
	if err != nil {
		return err
	}

	edit(tag)

	if len(tag.Add) == 0 && tag.Remove.Len() == 0 {
		return res.Store.UnstageTag(ctx, s.service, entryKey)
	}

	return res.Store.StageTag(ctx, s.service, entryKey, *tag)
}

// mapToTags renders a tag map as a slice of neutral Tags sorted by key, so the
// page (and its goldens) render staged tags in a stable order.
func mapToTags(m map[string]string) []Tag {
	tags := lo.MapToSlice(m, func(k, v string) Tag { return Tag{Key: k, Value: v} })
	slices.SortFunc(tags, func(a, b Tag) int { return strings.Compare(a.Key, b.Key) })

	return tags
}

// sortedRemovals sorts tag removals by key for a stable render order.
func sortedRemovals(rs []TagRemoval) []TagRemoval {
	slices.SortFunc(rs, func(a, b TagRemoval) int { return strings.Compare(a.Key, b.Key) })

	return rs
}

// stagedDiffType maps the use-case diff type onto the neutral one.
func stagedDiffType(t stagingusecase.DiffEntryType) StagedDiffType {
	switch t {
	case stagingusecase.DiffEntryCreate:
		return StagedDiffCreate
	case stagingusecase.DiffEntryAutoUnstaged:
		return StagedDiffAutoUnstaged
	case stagingusecase.DiffEntryWarning:
		return StagedDiffWarning
	default:
		return StagedDiffNormal
	}
}

// stagingResetType maps the use-case reset type onto the neutral one.
func stagingResetType(t stagingusecase.ResetResultType) StagingResetType {
	switch t {
	case stagingusecase.ResetResultUnstagedAll:
		return StagingResetUnstagedAll
	case stagingusecase.ResetResultRestored:
		return StagingResetRestored
	case stagingusecase.ResetResultNotStaged:
		return StagingResetNotStaged
	case stagingusecase.ResetResultNothingStaged:
		return StagingResetNothingStaged
	case stagingusecase.ResetResultSkipped:
		return StagingResetSkipped
	case stagingusecase.ResetResultUnstagedTag:
		return StagingResetUnstagedTag
	default:
		return StagingResetUnstaged
	}
}

// applyStatusLabel maps an apply status enum onto its display verb.
func applyStatusLabel(s stagingusecase.ApplyResultStatus) string {
	switch s {
	case stagingusecase.ApplyResultCreated:
		return "created"
	case stagingusecase.ApplyResultUpdated:
		return "updated"
	case stagingusecase.ApplyResultDeleted:
		return "deleted"
	default:
		return "failed"
	}
}

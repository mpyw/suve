// The Source seam and its neutral display types (Item, Detail, HistoryRow, ...)
// are the unit this package is named for: the other files implement or feed
// them, and every caller spells them as data.X, so no file prefix fits these
// names.
//declscope:core

// Package data is the TUI's read-path data seam. It exposes a small, provider-
// neutral Source interface that the browser and diff pages depend on, plus
// concrete implementations backed by the internal/usecase/{param,secret} use
// cases over a provider.Store. Keeping the pages behind this interface lets a
// test drive them with a providermock-backed Source (mirroring how production
// resolves a store from the registry+scope) without touching a real cloud.
//
// The neutral output types here are deliberately pre-formatted for display
// (type labels, dates as strings) so a page renders them verbatim and never
// reaches back into the usecase/domain packages.
package data

import (
	"context"
	"regexp"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// historyLimit caps how many version-history rows the browser fetches per
// selection. It matches the GUI's cap (ParamLog/SecretLog pass 10) so both
// front-ends bound the same surface identically; without it a secret with many
// versions would fetch every version's value on each selection change (#747).
const historyLimit int32 = 10

// ListParams are the list inputs a browser header collects.
type ListParams struct {
	Prefix    string
	Filter    string
	Recursive bool
	WithValue bool
	// Namespace filters an Azure App Configuration listing. Empty means the null
	// namespace, aznamespace.AllNamespacesFilter ("*") means every namespace, and
	// any other value is a single concrete namespace. Ignored for other providers.
	Namespace string
}

// Item is one row in the entry list.
type Item struct {
	Name string
	// Value is the entry's value when the list was loaded WithValue; nil otherwise.
	Value *string
	// TypeLabel is the display value type (e.g. "SecureString"); empty when the
	// provider has no value type or the value was not fetched.
	TypeLabel string
	// Secret reports whether the value must be masked in the UI.
	Secret bool
	// Namespace is the entry's Azure App Configuration namespace (empty for the
	// null namespace and every other provider).
	Namespace string
}

// ListResult is a page of list items plus the paging cursor.
type ListResult struct {
	Items []Item
	// NextToken is the secret-service paging cursor; empty when there are no more
	// pages (every provider today lists all names, so it is always empty, but the
	// field keeps the load-more wiring honest).
	NextToken string
}

// MetaRow is one capability-gated label/value line in the detail pane.
type MetaRow struct {
	Label string
	Value string
}

// Tag is a neutral key/value tag.
type Tag struct {
	Key   string
	Value string
}

// Detail is the current-version detail of one entry.
type Detail struct {
	Name  string
	Value string
	// Secret reports whether Value must be masked by default.
	Secret bool
	// Meta are the capability-gated metadata rows (version/type/dates/etc.),
	// pre-built so the pane renders them verbatim.
	Meta []MetaRow
	// State is the per-version lifecycle state (Google Cloud / Azure Key Vault),
	// empty when the version carries staging labels instead.
	State string
	// Labels are the current version's movable labels (AWS staging labels),
	// empty when the version carries a State instead. Never infer one from the
	// other (#419).
	Labels      []string
	Description string
	Tags        []Tag
	Namespace   string
	// TypeLabel is the entry's display value type (e.g. "SecureString"), so the
	// edit dialog can preserve the type on an update. Empty for services with no
	// value type (secret, App Configuration).
	TypeLabel string
	// ARN is the Secrets Manager ARN surfaced from the entry's Extra metadata,
	// empty for providers that expose none.
	ARN string
}

// HistoryRow is one version row in the detail history.
type HistoryRow struct {
	// Version is the raw provider version identifier used to re-fetch/diff (a
	// numeric string for param, a version id for secret).
	Version string
	// Label is the display form ("#14" for param, a shortened id for secret).
	Label string
	// Date is the pre-formatted creation/modification date, empty when unknown.
	Date      string
	IsCurrent bool
	State     string
	Labels    []string
	// Value is this version's raw value, fetched alongside the metadata so the
	// history can show what the value was at each revision (GUI parity). Empty when
	// the value could not be fetched. It is masked in the UI when Secret is set.
	Value string
	// Secret reports whether Value is secret material and must be masked by default
	// (a secret-service value, or a SecureString param value).
	Secret bool
	// Tags are this version's tags (Azure Key Vault per-version tags only).
	Tags []Tag
}

// DiffContent carries the two raw version values and their labels so the diff
// page can compute (and re-compute, for parse-json) the unified diff itself.
type DiffContent struct {
	OldLabel string
	NewLabel string
	OldValue string
	NewValue string
	// Secret reports whether the two values are secrets and must be masked before
	// diffing, so a secret diff never renders a revealed value. The source is the
	// authority on secret-ness (the browser's OpenDiff carries no such flag).
	Secret bool
}

// Source is the read-path seam the browser and diff pages depend on. Every
// method is provider-neutral; the concrete param/secret sources map the
// usecase outputs onto these types and capability-gate the metadata.
type Source interface {
	// Capability returns the service's capability descriptor so a page can gate
	// its controls (history, namespaces, tags-per-version, …).
	Capability() capability.ServiceCapability
	// List returns the entries matching params.
	List(ctx context.Context, params ListParams) (ListResult, error)
	// Show returns the current-version detail of name (namespace applies only to
	// Azure App Configuration).
	Show(ctx context.Context, name, namespace string) (Detail, error)
	// History returns name's version history (empty when the service is
	// unversioned).
	History(ctx context.Context, name, namespace string) ([]HistoryRow, error)
	// VersionContents fetches the two versions' raw values for a diff.
	VersionContents(ctx context.Context, name, oldVersion, newVersion, namespace string) (DiffContent, error)
	// Namespaces lists the discovered Azure App Configuration namespaces (nil for
	// every other provider), so the header can offer them in its filter.
	Namespaces(ctx context.Context) ([]string, error)
}

// StoreResolver resolves a param provider.Store for an App Configuration
// namespace. For non-App-Configuration providers the namespace is ignored and
// the same store is returned for every call.
type StoreResolver func(ctx context.Context, namespace string) (provider.Store, error)

// appConfigNamespaceLister is the App-Config-specific extension the param source
// type-asserts on the resolved store to list entries across ALL namespaces,
// mirroring the GUI (#425). Only the Azure App Configuration store implements it.
type appConfigNamespaceLister interface {
	ListWithNamespaces(ctx context.Context) ([]appconfig.KeyNamespace, error)
}

// =============================================================================
// Param source
// =============================================================================

// paramSource maps the param usecases onto the neutral Source types.
type paramSource struct {
	svcCap  capability.ServiceCapability
	resolve StoreResolver
}

// NewParamSource builds a param Source. resolve returns the param store for a
// given App Configuration namespace; for other providers it must ignore the
// namespace and return the single resolved store.
func NewParamSource(svcCap capability.ServiceCapability, resolve StoreResolver) Source {
	return &paramSource{svcCap: svcCap, resolve: resolve}
}

func (s *paramSource) Capability() capability.ServiceCapability { return s.svcCap }

func (s *paramSource) List(ctx context.Context, params ListParams) (ListResult, error) {
	store, err := s.resolve(ctx, "")
	if err != nil {
		return ListResult{}, err
	}

	// Azure App Configuration: list every namespace so each item carries its own,
	// then filter by the requested namespace client-side (GUI parity, #425).
	if lister, ok := store.(appConfigNamespaceLister); ok {
		return s.listWithNamespaces(ctx, lister, params)
	}

	uc := &param.ListUseCase{Reader: store}

	out, err := uc.Execute(ctx, param.ListInput{
		Prefix:    params.Prefix,
		Recursive: params.Recursive,
		Filter:    params.Filter,
		WithValue: params.WithValue,
	})
	if err != nil {
		return ListResult{}, err
	}

	items := lo.Map(out.Entries, func(e param.ListEntry, _ int) Item {
		return Item{
			Name:      e.Name,
			Value:     e.Value,
			TypeLabel: typeLabel(e.Type, e.Value != nil),
			Secret:    e.Type == domain.ValueTypeSecret,
		}
	})

	return ListResult{Items: items}, nil
}

// listWithNamespaces builds the App Configuration listing from the all-namespace
// load, applying the same prefix/recursive/regex filters as param.ListUseCase
// (via param.MatchPrefix) plus the namespace filter.
func (s *paramSource) listWithNamespaces(
	ctx context.Context, lister appConfigNamespaceLister, params ListParams,
) (ListResult, error) {
	re, err := compileFilter(params.Filter)
	if err != nil {
		return ListResult{}, err
	}

	rows, err := lister.ListWithNamespaces(ctx)
	if err != nil {
		return ListResult{}, err
	}

	items := lo.FilterMap(rows, func(row appconfig.KeyNamespace, _ int) (Item, bool) {
		if !param.MatchPrefix(row.Key, params.Prefix, params.Recursive) {
			return Item{}, false
		}

		if re != nil && !re.MatchString(row.Key) {
			return Item{}, false
		}

		if !namespaceMatches(params.Namespace, row.Namespace) {
			return Item{}, false
		}

		item := Item{Name: row.Key, Namespace: row.Namespace, TypeLabel: paramtype.Display(domain.ValueTypePlaintext)}
		if params.WithValue {
			item.Value = lo.ToPtr(row.Value)
		}

		return item, true
	})

	return ListResult{Items: items}, nil
}

func (s *paramSource) Show(ctx context.Context, name, namespace string) (Detail, error) {
	store, err := s.resolve(ctx, namespace)
	if err != nil {
		return Detail{}, err
	}

	uc := &param.ShowUseCase{Reader: store}

	out, err := uc.Execute(ctx, param.ShowInput{Name: name})
	if err != nil {
		return Detail{}, err
	}

	d := Detail{
		Name:        out.Name,
		Value:       out.Value,
		Secret:      out.Type == domain.ValueTypeSecret,
		Description: out.Description,
		Namespace:   namespace,
		TypeLabel:   typeLabel(out.Type, true),
		Tags: lo.Map(out.Tags, func(t param.ShowTag, _ int) Tag {
			return Tag{Key: t.Key, Value: t.Value}
		}),
	}

	if s.svcCap.HasVersionHistory {
		d.Meta = append(d.Meta, MetaRow{Label: "Version", Value: currentVersionLabel(out.Version)})
	}

	// App Configuration values are untyped, so only a typed param service (AWS
	// SSM) shows a Type row — matching the GUI hiding it for App Config.
	if !s.svcCap.HasNamespaces {
		d.Meta = append(d.Meta, MetaRow{Label: "Type", Value: typeLabel(out.Type, true)})
	}

	if s.svcCap.HasNamespaces {
		d.Meta = append(d.Meta, MetaRow{Label: "Namespace", Value: namespaceDisplay(namespace)})
	}

	if out.LastModified != nil {
		d.Meta = append(d.Meta, MetaRow{Label: "Modified", Value: timeutil.FormatDateTime(*out.LastModified)})
	}

	return d, nil
}

func (s *paramSource) History(ctx context.Context, name, namespace string) ([]HistoryRow, error) {
	if !s.svcCap.HasVersionHistory {
		return nil, nil
	}

	store, err := s.resolve(ctx, namespace)
	if err != nil {
		return nil, err
	}

	uc := &param.LogUseCase{Reader: store}

	out, err := uc.Execute(ctx, param.LogInput{Name: name, MaxResults: historyLimit})
	if err != nil {
		return nil, err
	}

	return lo.Map(out.Entries, func(e param.LogEntry, _ int) HistoryRow {
		return HistoryRow{
			Version:   e.Version,
			Label:     "#" + e.Version,
			Date:      formatDate(e.LastModified),
			IsCurrent: e.IsCurrent,
			Value:     e.Value,
			// A SecureString param value is secret material on the value-type axis, so
			// it is masked by default even though this is the param service (#733).
			Secret: e.Type == domain.ValueTypeSecret,
		}
	}), nil
}

func (s *paramSource) VersionContents(
	ctx context.Context, name, oldVersion, newVersion, namespace string,
) (DiffContent, error) {
	store, err := s.resolve(ctx, namespace)
	if err != nil {
		return DiffContent{}, err
	}

	uc := &param.DiffUseCase{Reader: store}

	out, err := uc.Execute(ctx, param.DiffInput{
		Name1: name, Suffix1: versionSuffix(oldVersion),
		Name2: name, Suffix2: versionSuffix(newVersion),
	})
	if err != nil {
		return DiffContent{}, err
	}

	return DiffContent{
		OldLabel: out.OldName + "#" + out.OldVersion,
		NewLabel: out.NewName + "#" + out.NewVersion,
		OldValue: out.OldValue,
		NewValue: out.NewValue,
		// A SecureString param is a secret on the value-type axis, so the diff page
		// masks both sides — even though this is the param service (#677).
		Secret: out.Secret,
	}, nil
}

func (s *paramSource) Namespaces(ctx context.Context) ([]string, error) {
	if !s.svcCap.HasNamespaces {
		return nil, nil
	}

	store, err := s.resolve(ctx, "")
	if err != nil {
		return nil, err
	}

	lister, ok := store.(appConfigNamespaceLister)
	if !ok {
		return nil, nil
	}

	rows, err := lister.ListWithNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	return lo.Uniq(lo.Map(rows, func(row appconfig.KeyNamespace, _ int) string {
		return row.Namespace
	})), nil
}

// =============================================================================
// Secret source
// =============================================================================

// secretSource maps the secret usecases onto the neutral Source types.
type secretSource struct {
	svcCap capability.ServiceCapability
	store  provider.Store
}

// NewSecretSource builds a secret Source over a resolved secret store.
func NewSecretSource(svcCap capability.ServiceCapability, store provider.Store) Source {
	return &secretSource{svcCap: svcCap, store: store}
}

func (s *secretSource) Capability() capability.ServiceCapability { return s.svcCap }

func (s *secretSource) List(ctx context.Context, params ListParams) (ListResult, error) {
	uc := &secret.ListUseCase{Reader: s.store}

	out, err := uc.Execute(ctx, secret.ListInput{
		Prefix:    params.Prefix,
		Filter:    params.Filter,
		WithValue: params.WithValue,
	})
	if err != nil {
		return ListResult{}, err
	}

	items := lo.Map(out.Entries, func(e secret.ListEntry, _ int) Item {
		return Item{Name: e.Name, Value: e.Value, Secret: true}
	})

	return ListResult{Items: items, NextToken: out.NextToken}, nil
}

func (s *secretSource) Show(ctx context.Context, name, _ string) (Detail, error) {
	uc := &secret.ShowUseCase{Reader: s.store}

	out, err := uc.Execute(ctx, secret.ShowInput{Name: name})
	if err != nil {
		return Detail{}, err
	}

	d := Detail{
		Name:        out.Name,
		Value:       out.Value,
		Secret:      true,
		State:       out.State,
		Labels:      out.Labels,
		Description: out.Description,
		ARN:         lo.FindOrElse(out.Extra, domain.Field{}, func(f domain.Field) bool { return f.Label == "ARN" }).Value,
		Tags: lo.Map(out.Tags, func(t secret.ShowTag, _ int) Tag {
			return Tag{Key: t.Key, Value: t.Value}
		}),
	}

	d.Meta = append(d.Meta, MetaRow{Label: "Version ID", Value: out.Version})

	if out.CreatedDate != nil {
		d.Meta = append(d.Meta, MetaRow{Label: "Created", Value: timeutil.FormatDateTime(*out.CreatedDate)})
	}

	if d.ARN != "" {
		d.Meta = append(d.Meta, MetaRow{Label: "ARN", Value: d.ARN})
	}

	return d, nil
}

func (s *secretSource) History(ctx context.Context, name, _ string) ([]HistoryRow, error) {
	if !s.svcCap.HasVersionHistory {
		return nil, nil
	}

	uc := &secret.LogUseCase{Reader: s.store}

	out, err := uc.Execute(ctx, secret.LogInput{Name: name, MaxResults: historyLimit})
	if err != nil {
		return nil, err
	}

	return lo.Map(out.Entries, func(e secret.LogEntry, _ int) HistoryRow {
		return HistoryRow{
			Version:   e.Version,
			Label:     shortID(e.Version),
			Date:      formatDate(e.CreatedDate),
			IsCurrent: e.IsCurrent,
			State:     e.State,
			Labels:    e.Labels,
			Value:     e.Value,
			// Every secret-service value is secret material and masked by default.
			Secret: true,
			Tags: lo.Map(e.Tags, func(t domain.Tag, _ int) Tag {
				return Tag{Key: t.Key, Value: t.Value}
			}),
		}
	}), nil
}

func (s *secretSource) VersionContents(
	ctx context.Context, name, oldVersion, newVersion, _ string,
) (DiffContent, error) {
	uc := &secret.DiffUseCase{Reader: s.store}

	out, err := uc.Execute(ctx, secret.DiffInput{
		Name1: name, Suffix1: versionSuffix(oldVersion),
		Name2: name, Suffix2: versionSuffix(newVersion),
	})
	if err != nil {
		return DiffContent{}, err
	}

	return DiffContent{
		OldLabel: out.OldName + "#" + shortID(out.OldVersion),
		NewLabel: out.NewName + "#" + shortID(out.NewVersion),
		OldValue: out.OldValue,
		NewValue: out.NewValue,
		Secret:   true,
	}, nil
}

func (s *secretSource) Namespaces(context.Context) ([]string, error) { return nil, nil }

// =============================================================================
// Shared helpers
// =============================================================================

// compileFilter compiles a regex filter, treating an empty pattern as "no
// filter" (nil).
func compileFilter(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil //nolint:nilnil // nil regex is the documented "no filter" sentinel
	}

	return regexp.Compile(pattern)
}

// namespaceMatches reports whether an entry's namespace passes the header
// filter: "*" matches every namespace, and an empty/other filter matches an
// exactly-equal namespace (empty being the null namespace).
func namespaceMatches(filter, entry string) bool {
	if filter == aznamespace.AllNamespacesFilter {
		return true
	}

	return filter == entry
}

// namespaceDisplay renders a namespace for the UI, showing the null namespace as
// aznamespace.NullDisplay ("(NULL)").
func namespaceDisplay(namespace string) string {
	if namespace == "" {
		return aznamespace.NullDisplay
	}

	return namespace
}

// typeLabel renders a value type, returning "" when the type is unknown (a
// name-only listing) so the UI omits it.
func typeLabel(t domain.ValueType, known bool) string {
	if !known || t == "" {
		return ""
	}

	return paramtype.Display(t)
}

// currentVersionLabel annotates a param version number as the current one.
func currentVersionLabel(version string) string {
	return version + " (current)"
}

// versionSuffix addresses a history row's raw version id for Resolve; an empty
// id is the current version (no suffix).
func versionSuffix(version string) string {
	if version == "" {
		return ""
	}

	return "#" + version
}

// shortID shortens a long opaque version id for compact history/diff labels,
// keeping short ids (e.g. numeric) intact.
func shortID(id string) string {
	const keep = 8
	if len(id) <= keep {
		return id
	}

	return id[:keep] + "…"
}

// formatDate renders an optional timestamp as a YYYY-MM-DD date, "" when unset.
func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}

	return timeutil.FormatDate(*t)
}

// =============================================================================
// Shared seam types
//
// These types are shared across the seam files (mutator.go, stage.go,
// stagingservice.go) and their consumers; they live here because their names
// are package-wide vocabulary rather than any one file's.
// =============================================================================

// WriteOutcome carries the semantic result of a mutation the UI must voice.
// Skipped is set when a staged edit equalled the live value (nothing staged);
// Unstaged when an edit-back-to-base or a delete-of-staged-create auto-unstaged
// the entry (EditOutput.Skipped/Unstaged, DeleteOutput.Unstaged); Updated when
// an immediate create fell back to update because the entry already existed
// (the create-or-update/upsert branch), so the status voices an update rather
// than a create — matching the GUI (ParamSet) and CLI (`param set`).
type WriteOutcome struct {
	Skipped  bool
	Unstaged bool
	Updated  bool
}

// ErrRestoreUnsupported is returned by Restore when the resolved store does not
// implement provider.Restorer (the capability gate should prevent reaching it).
var ErrRestoreUnsupported = stringError("restore is not supported by this provider")

// stringError is a small sentinel error type for the data seam.
type stringError string

func (e stringError) Error() string { return string(e) }

// StrategyBuilder builds the provider-specific staging strategy over a resolved
// provider.Store. The returned FullStrategy satisfies staging.EditStrategy and
// (via the concrete type) staging.DeleteStrategy, matching the GUI's
// serviceStrategyScoped narrowing. It fails for a provider that has no strategy
// for the service.
type StrategyBuilder func(store provider.Store) (staging.FullStrategy, error)

// StoreUnavailableError marks a StagingProbe failure that comes from CONSTRUCTING
// the on-disk staging store (a keychain hard-fail / key-loss while encrypted state
// exists), as opposed to a transient status read. This class of failure is
// persistent and actionable, so the browser surfaces it on the error line, while
// keeping ordinary probe read errors quiet (badges just do not show). The epic
// requires a key-loss to be visible on the read path, not only the write path.
type StoreUnavailableError struct{ Err error }

func (e *StoreUnavailableError) Error() string { return e.Err.Error() }

func (e *StoreUnavailableError) Unwrap() error { return e.Err }

// TagRemoval is a staged tag removal: the key and its current remote value.
type TagRemoval struct {
	Key   string
	Value string
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

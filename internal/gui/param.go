//go:build production || dev

package gui

import (
	"context"
	"errors"
	"regexp"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramtype"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
)

// appConfigNamespaceLister is the App-Config-specific extension the GUI type-
// asserts on the resolved param store to load entries across ALL namespaces
// (#425). Only the Azure App Configuration store implements it; the neutral
// provider.Reader.List contract is untouched, so other providers never match.
type appConfigNamespaceLister interface {
	ListWithNamespaces(ctx context.Context) ([]appconfig.KeyNamespace, error)
}

// =============================================================================
// Param Types
// =============================================================================

// ParamListResult represents the result of listing parameters.
type ParamListResult struct {
	Entries   []ParamListEntry `json:"entries"`
	NextToken string           `json:"nextToken,omitempty"`
}

// ParamListEntry represents a single parameter in the list.
type ParamListEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Secret reports whether the value is a secret (masked in the UI),
	// provider-neutrally derived from the domain value type.
	Secret bool    `json:"secret"`
	Value  *string `json:"value,omitempty"`
	// Namespace carries each entry's namespace for Azure App Configuration (the
	// axis Azure calls a "label"); empty is the null/default namespace. It is
	// populated only when the current scope is Azure App Configuration (the list
	// is then loaded across ALL namespaces so the GUI can filter client-side,
	// #425); for every other provider it stays empty.
	Namespace string `json:"namespace"`
}

// ParamShowTag represents a tag key-value pair.
type ParamShowTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParamShowResult represents the result of showing a parameter.
type ParamShowResult struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	Type    string `json:"type"`
	// Secret reports whether the value is a secret (masked in the UI),
	// provider-neutrally derived from the domain value type.
	Secret       bool           `json:"secret"`
	Description  string         `json:"description,omitempty"`
	LastModified string         `json:"lastModified,omitempty"`
	Tags         []ParamShowTag `json:"tags"`
}

// ParamLogResult represents the result of showing parameter history.
type ParamLogResult struct {
	Name    string          `json:"name"`
	Entries []ParamLogEntry `json:"entries"`
}

// ParamLogEntry represents a single version in the history.
type ParamLogEntry struct {
	Version int64  `json:"version"`
	Value   string `json:"value"`
	Type    string `json:"type"`
	// Secret reports whether the value is a secret (masked in the UI),
	// provider-neutrally derived from the domain value type.
	Secret       bool   `json:"secret"`
	IsCurrent    bool   `json:"isCurrent"`
	LastModified string `json:"lastModified,omitempty"`
}

// ParamDiffResult represents the result of comparing parameters.
type ParamDiffResult struct {
	OldName  string `json:"oldName"`
	NewName  string `json:"newName"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

// ParamSetResult represents the result of setting a parameter.
type ParamSetResult struct {
	Name      string `json:"name"`
	Version   int64  `json:"version"`
	IsCreated bool   `json:"isCreated"`
}

// ParamDeleteResult represents the result of deleting a parameter.
type ParamDeleteResult struct {
	Name string `json:"name"`
}

// =============================================================================
// Param Methods
// =============================================================================

// ParamList lists parameters. For Azure App Configuration it loads entries
// across ALL namespaces (each carrying its namespace) so the GUI can filter by
// namespace client-side (#425); every other provider uses the neutral
// param.ListUseCase path and leaves Namespace empty.
func (a *App) ParamList(prefix string, recursive bool, withValue bool, filter string, _ int, _ string) (*ParamListResult, error) {
	store, err := a.paramStore()
	if err != nil {
		return nil, err
	}

	// Azure App Configuration: if the store exposes the cross-namespace lister,
	// list every namespace so each entry carries its own namespace.
	if lister, ok := store.(appConfigNamespaceLister); ok {
		return a.paramListWithNamespaces(lister, prefix, recursive, withValue, filter)
	}

	uc := &param.ListUseCase{Reader: store}

	result, err := uc.Execute(a.ctx, param.ListInput{
		Prefix:    prefix,
		Recursive: recursive,
		WithValue: withValue,
		Filter:    filter,
	})
	if err != nil {
		return nil, err
	}

	entries := lo.Map(result.Entries, func(e param.ListEntry, _ int) ParamListEntry {
		return ParamListEntry{
			Name:   e.Name,
			Type:   paramtype.Display(e.Type),
			Secret: e.Type == domain.ValueTypeSecret,
			Value:  e.Value,
		}
	})

	return &ParamListResult{Entries: entries}, nil
}

// paramListWithNamespaces builds the list for Azure App Configuration from the
// all-namespaces load, so each entry carries its namespace. The same
// prefix/recursive/regex client-side filtering as param.ListUseCase is applied
// (via param.MatchPrefix); namespace filtering itself is done in the frontend.
func (a *App) paramListWithNamespaces(
	lister appConfigNamespaceLister, prefix string, recursive, withValue bool, filter string,
) (*ParamListResult, error) {
	var filterRegex *regexp.Regexp

	if filter != "" {
		re, err := regexp.Compile(filter)
		if err != nil {
			return nil, err
		}

		filterRegex = re
	}

	items, err := lister.ListWithNamespaces(a.ctx)
	if err != nil {
		return nil, err
	}

	entries := lo.FilterMap(items, func(item appconfig.KeyNamespace, _ int) (ParamListEntry, bool) {
		if !param.MatchPrefix(item.Key, prefix, recursive) {
			return ParamListEntry{}, false
		}

		if filterRegex != nil && !filterRegex.MatchString(item.Key) {
			return ParamListEntry{}, false
		}

		// App Configuration values are always plaintext (never a secret), so the
		// domain value type is fixed; mirror it into Type/Secret like the SSM path.
		entry := ParamListEntry{
			Name:      item.Key,
			Type:      paramtype.Display(domain.ValueTypePlaintext),
			Secret:    false,
			Namespace: item.Namespace,
		}
		if withValue {
			entry.Value = lo.ToPtr(item.Value)
		}

		return entry, true
	})

	return &ParamListResult{Entries: entries}, nil
}

// ParamShow shows a parameter value. namespace selects the Azure App
// Configuration namespace of the setting (the label axis); empty is the
// null/default namespace and is ignored by every other provider. It must be the
// entry's own namespace so a namespaced setting is read under its own label
// rather than the shared read scope's (which the footer filter never changes).
func (a *App) ParamShow(specStr, namespace string) (*ParamShowResult, error) {
	spec, err := a.parseParamSpec(specStr)
	if err != nil {
		return nil, err
	}

	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	uc := &param.ShowUseCase{Reader: store}

	result, err := uc.Execute(a.ctx, param.ShowInput{Spec: spec})
	if err != nil {
		return nil, err
	}

	r := &ParamShowResult{
		Name:        result.Name,
		Value:       result.Value,
		Version:     result.Version,
		Type:        paramtype.Display(result.Type),
		Secret:      result.Type == domain.ValueTypeSecret,
		Description: result.Description,
		Tags: lo.Map(result.Tags, func(tag param.ShowTag, _ int) ParamShowTag {
			return ParamShowTag{Key: tag.Key, Value: tag.Value}
		}),
	}
	if result.LastModified != nil {
		r.LastModified = timeutil.FormatRFC3339(*result.LastModified)
	}

	return r, nil
}

// ParamLog shows parameter version history. namespace selects the entry's Azure
// App Configuration namespace (empty for the null/default namespace and every
// other provider).
func (a *App) ParamLog(name string, maxResults int32, namespace string) (*ParamLogResult, error) {
	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	uc := &param.LogUseCase{Reader: store}

	result, err := uc.Execute(a.ctx, param.LogInput{
		Name:       name,
		MaxResults: maxResults,
	})
	if err != nil {
		return nil, err
	}

	entries := lo.Map(result.Entries, func(e param.LogEntry, _ int) ParamLogEntry {
		entry := ParamLogEntry{
			Version:   e.Version,
			Value:     e.Value,
			Type:      paramtype.Display(e.Type),
			Secret:    e.Type == domain.ValueTypeSecret,
			IsCurrent: e.IsCurrent,
		}
		if e.LastModified != nil {
			entry.LastModified = timeutil.FormatRFC3339(*e.LastModified)
		}

		return entry
	})

	return &ParamLogResult{Name: result.Name, Entries: entries}, nil
}

// ParamDiff compares two parameter versions. namespace selects the entry's Azure
// App Configuration namespace (empty for the null/default namespace and every
// other provider).
func (a *App) ParamDiff(spec1Str, spec2Str, namespace string) (*ParamDiffResult, error) {
	spec1, err := a.parseParamSpec(spec1Str)
	if err != nil {
		return nil, err
	}

	spec2, err := a.parseParamSpec(spec2Str)
	if err != nil {
		return nil, err
	}

	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	uc := &param.DiffUseCase{Reader: store}

	result, err := uc.Execute(a.ctx, param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	if err != nil {
		return nil, err
	}

	return &ParamDiffResult{
		OldName:  result.OldName,
		NewName:  result.NewName,
		OldValue: result.OldValue,
		NewValue: result.NewValue,
	}, nil
}

// ParamSet creates or updates a parameter.
// It first tries to create the parameter; if it already exists, it updates instead.
//
// namespace selects the Azure App Configuration namespace to write under (the
// label axis); empty is the null/default namespace and it is ignored by every
// other provider. It must name a single concrete namespace — a filter value
// (`*` or a `,`-list) is rejected, since a write targets exactly one
// (key, namespace).
func (a *App) ParamSet(name, value, paramType, namespace string) (*ParamSetResult, error) {
	namespace, err := a.validateParamNamespace(namespace)
	if err != nil {
		return nil, err
	}

	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	valueType := paramtype.Parse(paramType)

	// Try to create first; if the parameter already exists, update it instead.
	createUC := &param.CreateUseCase{Writer: store}

	createResult, err := createUC.Execute(a.ctx, param.CreateInput{
		Name:  name,
		Value: value,
		Type:  valueType,
	})
	if err == nil {
		return &ParamSetResult{
			Name:      createResult.Name,
			Version:   createResult.Version,
			IsCreated: true,
		}, nil
	}

	if !errors.Is(err, provider.ErrAlreadyExists) {
		return nil, err
	}

	updateUC := &param.UpdateUseCase{Store: store}

	updateResult, err := updateUC.Execute(a.ctx, param.UpdateInput{
		Name:  name,
		Value: value,
		Type:  valueType,
	})
	if err != nil {
		return nil, err
	}

	return &ParamSetResult{
		Name:      updateResult.Name,
		Version:   updateResult.Version,
		IsCreated: false,
	}, nil
}

// ParamDelete deletes a parameter. namespace selects the entry's Azure App
// Configuration namespace (empty for the null/default namespace and every other
// provider) so a namespaced setting is deleted under its own label, not the
// shared read scope's.
func (a *App) ParamDelete(name, namespace string) (*ParamDeleteResult, error) {
	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	uc := &param.DeleteUseCase{Store: store}

	result, err := uc.Execute(a.ctx, param.DeleteInput{Name: name})
	if err != nil {
		return nil, err
	}

	return &ParamDeleteResult{Name: result.Name}, nil
}

// ParamAddTag adds or updates a tag on a parameter. namespace selects the entry's
// Azure App Configuration namespace (empty for the null/default namespace and
// every other provider).
func (a *App) ParamAddTag(name, key, value, namespace string) error {
	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return err
	}

	uc := &param.TagUseCase{Tagger: store}

	return uc.Execute(a.ctx, param.TagInput{
		Name: name,
		Add:  map[string]string{key: value},
	})
}

// ParamRemoveTag removes a tag from a parameter. namespace selects the entry's
// Azure App Configuration namespace (empty for the null/default namespace and
// every other provider).
func (a *App) ParamRemoveTag(name, key, namespace string) error {
	store, err := a.paramStoreForNamespace(namespace)
	if err != nil {
		return err
	}

	uc := &param.TagUseCase{Tagger: store}

	return uc.Execute(a.ctx, param.TagInput{
		Name:   name,
		Remove: []string{key},
	})
}

// ParamTypeOptions returns the selectable parameter type display names for the
// current provider (AWS: "String", "SecureString", "StringList"). The frontend
// renders its type dropdown from this list instead of hardcoding SSM strings.
// Only AWS SSM has a value type; Azure App Configuration values are untyped, so
// the list is empty there and the frontend hides the Type dropdown.
func (a *App) ParamTypeOptions() []string {
	if a.currentScope().Provider != provider.ProviderAWS {
		return []string{}
	}

	return paramtype.Options()
}

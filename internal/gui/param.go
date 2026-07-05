//go:build production || dev

package gui

import (
	"errors"

	"github.com/mpyw/suve/internal/cli/commands/param/paramtype"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
)

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

// ParamList lists SSM parameters.
func (a *App) ParamList(prefix string, recursive bool, withValue bool, filter string, _ int, _ string) (*ParamListResult, error) {
	store, err := a.paramStore()
	if err != nil {
		return nil, err
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

	entries := make([]ParamListEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = ParamListEntry{
			Name:  e.Name,
			Value: e.Value,
		}
	}

	return &ParamListResult{Entries: entries}, nil
}

// ParamShow shows a parameter value.
func (a *App) ParamShow(specStr string) (*ParamShowResult, error) {
	spec, err := a.parseParamSpec(specStr)
	if err != nil {
		return nil, err
	}

	store, err := a.paramStore()
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
		Tags:        make([]ParamShowTag, 0, len(result.Tags)),
	}
	if result.LastModified != nil {
		r.LastModified = timeutil.FormatRFC3339(*result.LastModified)
	}

	for _, tag := range result.Tags {
		r.Tags = append(r.Tags, ParamShowTag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}

	return r, nil
}

// ParamLog shows parameter version history.
func (a *App) ParamLog(name string, maxResults int32) (*ParamLogResult, error) {
	store, err := a.paramStore()
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

	entries := make([]ParamLogEntry, len(result.Entries))
	for i, e := range result.Entries {
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

		entries[i] = entry
	}

	return &ParamLogResult{Name: result.Name, Entries: entries}, nil
}

// ParamDiff compares two parameter versions.
func (a *App) ParamDiff(spec1Str, spec2Str string) (*ParamDiffResult, error) {
	spec1, err := a.parseParamSpec(spec1Str)
	if err != nil {
		return nil, err
	}

	spec2, err := a.parseParamSpec(spec2Str)
	if err != nil {
		return nil, err
	}

	store, err := a.paramStore()
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
func (a *App) ParamSet(name, value, paramType string) (*ParamSetResult, error) {
	store, err := a.paramStore()
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

// ParamDelete deletes a parameter.
func (a *App) ParamDelete(name string) (*ParamDeleteResult, error) {
	store, err := a.paramStore()
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

// ParamAddTag adds or updates a tag on a parameter.
func (a *App) ParamAddTag(name, key, value string) error {
	store, err := a.paramStore()
	if err != nil {
		return err
	}

	uc := &param.TagUseCase{Tagger: store}

	return uc.Execute(a.ctx, param.TagInput{
		Name: name,
		Add:  map[string]string{key: value},
	})
}

// ParamRemoveTag removes a tag from a parameter.
func (a *App) ParamRemoveTag(name, key string) error {
	store, err := a.paramStore()
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

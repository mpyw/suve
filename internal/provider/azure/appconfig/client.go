package appconfig

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
	"github.com/samber/lo"
)

// apiClient adapts the concrete *azappconfig.Client to the narrow Client
// interface, draining the SDK's list pager into a slice. It is the only place
// the concrete SDK client and its pager are referenced.
//
// The label carrying suve's namespace is applied via the high-level SDK options
// (GetSettingOptions.Label, SetSettingOptions.Label, ...) for single-key ops and
// SettingSelector.LabelFilter for List. An empty label leaves the options nil so
// the request is byte-for-byte identical to addressing the null (default) label.
type apiClient struct {
	c *azappconfig.Client
}

// Wrap adapts a concrete App Configuration client to the narrow Client interface.
func Wrap(c *azappconfig.Client) Client {
	return &apiClient{c: c}
}

// Compile-time assertion that apiClient satisfies Client.
var _ Client = (*apiClient)(nil)

// An empty label leaves the options pointer nil so the request addresses the
// null (default) label exactly as it did before the namespace axis existed; a
// non-empty label is carried via the option's Label field.

func (a *apiClient) GetSetting(ctx context.Context, key, label string) (azappconfig.GetSettingResponse, error) {
	var opts *azappconfig.GetSettingOptions
	if label != "" {
		opts = &azappconfig.GetSettingOptions{Label: lo.ToPtr(label)}
	}

	return a.c.GetSetting(ctx, key, opts)
}

// SetSetting upserts key=value under label, carrying the given tags, the given
// content-type, and (when etag is non-nil) an OnlyIfUnchanged precondition. App
// Configuration's PUT replaces the whole key-value, so both tags and
// content-type must always be re-sent to be preserved (a nil content-type
// leaves it unset); a nil etag makes the write unconditional.
func (a *apiClient) SetSetting(
	ctx context.Context, key, value, label string, tags map[string]*string, contentType *string, etag *azcore.ETag,
) (azappconfig.SetSettingResponse, error) {
	opts := &azappconfig.SetSettingOptions{
		ContentType:     contentType,
		Tags:            tags,
		OnlyIfUnchanged: etag,
	}
	if label != "" {
		opts.Label = lo.ToPtr(label)
	}

	return a.c.SetSetting(ctx, key, lo.ToPtr(value), opts)
}

func (a *apiClient) AddSetting(ctx context.Context, key, value, label string) (azappconfig.AddSettingResponse, error) {
	var opts *azappconfig.AddSettingOptions
	if label != "" {
		opts = &azappconfig.AddSettingOptions{Label: lo.ToPtr(label)}
	}

	return a.c.AddSetting(ctx, key, lo.ToPtr(value), opts)
}

func (a *apiClient) DeleteSetting(ctx context.Context, key, label string) (azappconfig.DeleteSettingResponse, error) {
	var opts *azappconfig.DeleteSettingOptions
	if label != "" {
		opts = &azappconfig.DeleteSettingOptions{Label: lo.ToPtr(label)}
	}

	return a.c.DeleteSetting(ctx, key, opts)
}

// listSettingSelector is the selector used to enumerate settings: all keys,
// restricted by the given LabelFilter. The filter is resolved by the store from
// the raw --namespace value (empty -> the null-label filter, aznamespace.Filter).
// A nil LabelFilter (SettingSelector{}) would enumerate every label.
func listSettingSelector(filter string) azappconfig.SettingSelector {
	return azappconfig.SettingSelector{
		LabelFilter: lo.ToPtr(filter),
	}
}

func (a *apiClient) ListSettings(ctx context.Context, filter string) ([]azappconfig.Setting, error) {
	pager := a.c.NewListSettingsPager(listSettingSelector(filter), nil)

	var out []azappconfig.Setting

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		out = append(out, page.Settings...)
	}

	return out, nil
}

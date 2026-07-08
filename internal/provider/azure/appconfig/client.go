package appconfig

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/samber/lo"
)

// nullLabelFilter is the reserved App Configuration label filter that matches
// ONLY settings with no label. The Get/Set/Add/Delete single-key operations all
// address the null (default) label, so List must restrict to the same label —
// otherwise it enumerates every label and surfaces phantom keys that show/stage
// then cannot fetch. The service encodes this reserved value as label=%00.
const nullLabelFilter = "\x00"

// apiClient adapts the concrete *azappconfig.Client to the narrow Client
// interface, draining the SDK's list pager into a slice. It is the only place
// the concrete SDK client and its pager are referenced.
//
// Every operation uses the default (no) App Configuration label: options are
// left nil so the store addresses the single default-labeled setting per key,
// which the adapter presents as an unversioned parameter.
type apiClient struct {
	c *azappconfig.Client
}

// Wrap adapts a concrete App Configuration client to the narrow Client interface.
func Wrap(c *azappconfig.Client) Client {
	return &apiClient{c: c}
}

// Compile-time assertion that apiClient satisfies Client.
var _ Client = (*apiClient)(nil)

func (a *apiClient) GetSetting(ctx context.Context, key string) (azappconfig.GetSettingResponse, error) {
	return a.c.GetSetting(ctx, key, nil)
}

func (a *apiClient) SetSetting(ctx context.Context, key, value string) (azappconfig.SetSettingResponse, error) {
	return a.c.SetSetting(ctx, key, lo.ToPtr(value), nil)
}

func (a *apiClient) AddSetting(ctx context.Context, key, value string) (azappconfig.AddSettingResponse, error) {
	return a.c.AddSetting(ctx, key, lo.ToPtr(value), nil)
}

func (a *apiClient) DeleteSetting(ctx context.Context, key string) (azappconfig.DeleteSettingResponse, error) {
	return a.c.DeleteSetting(ctx, key, nil)
}

// listSettingSelector is the selector used to enumerate settings: all keys but
// ONLY the null (default) label, so List matches what the single-key operations
// address. A nil LabelFilter (SettingSelector{}) would enumerate every label.
func listSettingSelector() azappconfig.SettingSelector {
	return azappconfig.SettingSelector{
		LabelFilter: lo.ToPtr(nullLabelFilter),
	}
}

func (a *apiClient) ListSettings(ctx context.Context) ([]azappconfig.Setting, error) {
	pager := a.c.NewListSettingsPager(listSettingSelector(), nil)

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

// secretSource is the Source over the secret use case (AWS Secrets Manager,
// Google Cloud Secret Manager and Azure Key Vault).

package data

import (
	"context"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/secret"
)

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

	return ListResult{Items: items}, nil
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
		Tags: lo.Map(out.Tags, func(t secret.ShowTag, _ int) Tag {
			return Tag{Key: t.Key, Value: t.Value}
		}),
	}

	d.Meta = append(d.Meta, MetaRow{Label: "Version ID", Value: out.Version})

	if out.CreatedDate != nil {
		d.Meta = append(d.Meta, MetaRow{Label: "Created", Value: timeutil.FormatDateTime(*out.CreatedDate)})
	}

	// Provider-specific, display-only metadata (e.g. the Secrets Manager ARN),
	// rendered verbatim.
	d.Meta = append(d.Meta, lo.Map(out.Extra, func(f domain.Field, _ int) MetaRow {
		return MetaRow{Label: f.Label, Value: f.Value}
	})...)

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

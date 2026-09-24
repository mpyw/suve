// paramSource is the Source over the param use case (AWS Parameter Store and
// Azure App Configuration).

package data

import (
	"context"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
)

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

	// Only a service with typed values (HasValueType) shows a Type row —
	// matching the GUI hiding it elsewhere.
	if s.svcCap.HasValueType {
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

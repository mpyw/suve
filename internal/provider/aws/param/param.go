// Package param implements the provider.Store contract for AWS Systems Manager
// Parameter Store. It confines all SSM SDK types to this package: version
// resolution (absolute #version and ~shift against history) lives here, while
// spec PARSING stays generic via paramversion.Parse.
package param

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/paramversion"
)

// Client is the narrow SSM Parameter Store surface this adapter needs. The
// concrete *ssm.Client satisfies it; tests provide their own mock.
type Client interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	GetParameterHistory(
		ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options),
	) (*ssm.GetParameterHistoryOutput, error)
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	DescribeParameters(
		ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options),
	) (*ssm.DescribeParametersOutput, error)
	AddTagsToResource(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(
		ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options),
	) (*ssm.RemoveTagsFromResourceOutput, error)
	ListTagsForResource(
		ctx context.Context, params *ssm.ListTagsForResourceInput, optFns ...func(*ssm.Options),
	) (*ssm.ListTagsForResourceOutput, error)
}

// Store is the SSM Parameter Store implementation of provider.Store.
type Store struct {
	client Client
}

// Compile-time assertion that Store implements the provider contract.
var _ provider.Store = (*Store)(nil)

// New builds a Store backed by the given SSM client.
func New(client Client) *Store {
	return &Store{client: client}
}

// Resolve parses the version spec (generic) and resolves it (SSM-specific) to
// an opaque VersionRef holding the concrete version number. An empty/latest
// spec resolves to the latest ref (empty id).
func (s *Store) Resolve(ctx context.Context, name, spec string) (provider.VersionRef, error) {
	parsed, err := paramversion.Parse(name + spec)
	if err != nil {
		return provider.VersionRef{}, err
	}

	// No shift: the ref is the explicit version (if any), otherwise latest.
	if !parsed.HasShift() {
		if parsed.Absolute.Version != nil {
			return provider.NewVersionRef(strconv.FormatInt(*parsed.Absolute.Version, 10)), nil
		}

		return provider.NewVersionRef(""), nil
	}

	// Shift present: walk the FULL history newest-first from the base version.
	params, err := s.getFullHistory(ctx, name)
	if err != nil {
		return provider.VersionRef{}, fmt.Errorf("failed to get parameter history: %w", err)
	}

	if len(params) == 0 {
		return provider.VersionRef{}, fmt.Errorf("parameter not found: %s", name)
	}

	slices.Reverse(params) // AWS returns oldest-first; make it newest-first.

	baseIdx := 0

	if parsed.Absolute.Version != nil {
		var found bool

		_, baseIdx, found = lo.FindIndexOf(params, func(p types.ParameterHistory) bool {
			return p.Version == *parsed.Absolute.Version
		})
		if !found {
			return provider.VersionRef{}, fmt.Errorf("version %d not found", *parsed.Absolute.Version)
		}
	}

	targetIdx := baseIdx + parsed.Shift
	if targetIdx < 0 || targetIdx >= len(params) {
		return provider.VersionRef{}, fmt.Errorf("version shift out of range: ~%d", parsed.Shift)
	}

	return provider.NewVersionRef(strconv.FormatInt(params[targetIdx].Version, 10)), nil
}

// Get retrieves the parameter at the given ref (latest when ref is latest),
// decrypting SecureString values, and maps it to a domain.Entry with tags.
func (s *Store) Get(ctx context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
	target := name
	if !ref.IsLatest() {
		target = fmt.Sprintf("%s:%s", name, ref.ID())
	}

	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(target),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		var notFound *types.ParameterNotFound
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%w: %s", provider.ErrNotFound, name)
		}

		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	p := result.Parameter

	entry := &domain.Entry{
		Name:  aws.ToString(p.Name),
		Value: aws.ToString(p.Value),
		Type:  mapTypeToDomain(p.Type),
		Version: domain.Version{
			ID:      strconv.FormatInt(p.Version, 10),
			Created: p.LastModifiedDate,
		},
		Modified: p.LastModifiedDate,
	}

	// Tags are best-effort: a tagging failure must not fail the read.
	tagsOutput, err := s.client.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
		ResourceType: types.ResourceTypeForTaggingParameter,
		ResourceId:   p.Name,
	})
	if err == nil && tagsOutput != nil {
		entry.Tags = lo.Map(tagsOutput.TagList, func(t types.Tag, _ int) domain.Tag {
			return domain.Tag{Key: aws.ToString(t.Key), Value: aws.ToString(t.Value)}
		})
	}

	return entry, nil
}

// History returns the parameter's version history, newest first.
func (s *Store) History(ctx context.Context, name string) ([]domain.Version, error) {
	params, err := s.getFullHistory(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter history: %w", err)
	}

	versions := lo.Map(params, func(p types.ParameterHistory, _ int) domain.Version {
		return domain.Version{
			ID:      strconv.FormatInt(p.Version, 10),
			Created: p.LastModifiedDate,
		}
	})

	slices.Reverse(versions) // newest first

	return versions, nil
}

// getFullHistory returns the parameter's complete version history (oldest
// first, as AWS returns it), paging through NextToken. GetParameterHistory caps
// each page at 50 items while SSM retains up to 100 versions, so a single
// unpaged call would treat page 1 as the whole history — making index 0 look
// like the latest version and silently mis-resolving ~N / #N~M / log.
func (s *Store) getFullHistory(ctx context.Context, name string) ([]types.ParameterHistory, error) {
	var (
		all   []types.ParameterHistory
		token *string
	)

	for {
		out, err := s.client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
			Name:           aws.String(name),
			WithDecryption: aws.Bool(true),
			NextToken:      token,
		})
		if err != nil {
			return nil, err
		}

		all = append(all, out.Parameters...)

		if aws.ToString(out.NextToken) == "" {
			break
		}

		token = out.NextToken
	}

	return all, nil
}

// List returns the names of all parameters, paging through DescribeParameters.
func (s *Store) List(ctx context.Context) ([]string, error) {
	d := debug.From(ctx)

	var (
		names []string
		token *string
		pages int
	)

	for {
		out, err := s.client.DescribeParameters(ctx, &ssm.DescribeParametersInput{
			NextToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe parameters: %w", err)
		}

		pages++
		d.Logf("aws ssm: DescribeParameters page %d -> %d parameters\n", pages, len(out.Parameters))

		for _, p := range out.Parameters {
			names = append(names, aws.ToString(p.Name))
		}

		if aws.ToString(out.NextToken) == "" {
			break
		}

		token = out.NextToken
	}

	// The total makes a successful-but-empty result (wrong region/account)
	// visible at a glance, which a bodyless HTTP log cannot.
	d.Logf("aws ssm: DescribeParameters total %d parameters in %d page(s)\n", len(names), pages)

	return names, nil
}

// Create creates a new parameter (Overwrite=false) and returns the resulting
// version. It returns a wrapped provider.ErrAlreadyExists if the parameter
// already exists.
func (s *Store) Create(
	ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...provider.WriteOption,
) (domain.Version, error) {
	input := &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      mapDomainToType(valueType),
		Overwrite: aws.Bool(false),
	}
	if description != "" {
		input.Description = aws.String(description)
	}

	applyWriteOptions(input, opts)

	out, err := s.client.PutParameter(ctx, input)
	if err != nil {
		var exists *types.ParameterAlreadyExists
		if errors.As(err, &exists) {
			return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
		}

		return domain.Version{}, fmt.Errorf("failed to create parameter: %w", err)
	}

	return domain.Version{ID: strconv.FormatInt(out.Version, 10)}, nil
}

// Put creates or updates a parameter (Overwrite=true) and returns the resulting version.
func (s *Store) Put(
	ctx context.Context, name, value string, valueType domain.ValueType, description string, opts ...provider.WriteOption,
) (domain.Version, error) {
	input := &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      mapDomainToType(valueType),
		Overwrite: aws.Bool(true),
	}
	if description != "" {
		input.Description = aws.String(description)
	}

	applyWriteOptions(input, opts)

	out, err := s.client.PutParameter(ctx, input)
	if err != nil {
		return domain.Version{}, fmt.Errorf("failed to put parameter: %w", err)
	}

	return domain.Version{ID: strconv.FormatInt(out.Version, 10)}, nil
}

// Delete removes a parameter. Parameter Store exposes no delete options, so any
// provided DeleteOptions are ignored. A missing parameter maps to the wrapped
// provider.ErrNotFound sentinel so callers can treat it idempotently.
func (s *Store) Delete(ctx context.Context, name string, _ ...provider.DeleteOption) error {
	_, err := s.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		var notFound *types.ParameterNotFound
		if errors.As(err, &notFound) {
			return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
		}

		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	return nil
}

// Tag adds or updates tags on a parameter.
func (s *Store) Tag(ctx context.Context, name string, add map[string]string) error {
	if len(add) == 0 {
		return nil
	}

	tags := lo.MapToSlice(add, func(k, v string) types.Tag {
		return types.Tag{Key: aws.String(k), Value: aws.String(v)}
	})

	_, err := s.client.AddTagsToResource(ctx, &ssm.AddTagsToResourceInput{
		ResourceId:   aws.String(name),
		ResourceType: types.ResourceTypeForTaggingParameter,
		Tags:         tags,
	})
	if err != nil {
		return fmt.Errorf("failed to add tags: %w", err)
	}

	return nil
}

// Untag removes tags (by key) from a parameter.
func (s *Store) Untag(ctx context.Context, name string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	_, err := s.client.RemoveTagsFromResource(ctx, &ssm.RemoveTagsFromResourceInput{
		ResourceId:   aws.String(name),
		ResourceType: types.ResourceTypeForTaggingParameter,
		TagKeys:      keys,
	})
	if err != nil {
		return fmt.Errorf("failed to remove tags: %w", err)
	}

	return nil
}

// mapTypeToDomain maps an SSM parameter type to the provider-neutral ValueType.
func mapTypeToDomain(t types.ParameterType) domain.ValueType {
	switch t {
	case types.ParameterTypeSecureString:
		return domain.ValueTypeSecret
	case types.ParameterTypeStringList:
		return domain.ValueTypeList
	case types.ParameterTypeString:
		return domain.ValueTypePlaintext
	default:
		return domain.ValueTypePlaintext
	}
}

// mapDomainToType maps a provider-neutral ValueType to an SSM parameter type.
func mapDomainToType(t domain.ValueType) types.ParameterType {
	switch t {
	case domain.ValueTypeSecret:
		return types.ParameterTypeSecureString
	case domain.ValueTypeList:
		return types.ParameterTypeStringList
	case domain.ValueTypePlaintext:
		return types.ParameterTypeString
	default:
		return types.ParameterTypeString
	}
}

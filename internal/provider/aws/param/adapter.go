// Package param provides AWS SSM Parameter Store adapter implementing provider interfaces.
package param

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// Client combines all AWS SSM API interfaces required by the adapter.
type Client interface {
	paramapi.GetParameterAPI
	paramapi.GetParameterHistoryAPI
	paramapi.DescribeParametersAPI
	paramapi.PutParameterAPI
	paramapi.DeleteParameterAPI
	paramapi.AddTagsToResourceAPI
	paramapi.RemoveTagsFromResourceAPI
	paramapi.ListTagsForResourceAPI
}

// Adapter implements provider.ParameterService for AWS SSM.
type Adapter struct {
	client Client
}

// NewAdapter creates a new AWS SSM adapter using the default AWS configuration.
func NewAdapter(ctx context.Context) (*Adapter, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Adapter{client: ssm.NewFromConfig(cfg)}, nil
}

// New creates a new AWS SSM adapter from an existing client.
func New(client Client) *Adapter {
	return &Adapter{client: client}
}

// ============================================================================
// ParameterReader Implementation
// ============================================================================

// GetParameter retrieves a parameter by name and optional version.
func (a *Adapter) GetParameter(
	ctx context.Context, name string, version string,
) (*model.Parameter, error) {
	input := &paramapi.GetParameterInput{
		Name:           lo.ToPtr(name),
		WithDecryption: lo.ToPtr(true),
	}

	// Add version selector if specified
	if version != "" {
		input.Name = lo.ToPtr(fmt.Sprintf("%s:%s", name, version))
	}

	output, err := a.client.GetParameter(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	return convertParameter(output.Parameter), nil
}

// GetParameterHistory retrieves all versions of a parameter.
func (a *Adapter) GetParameterHistory(
	ctx context.Context, name string,
) (*model.ParameterHistory, error) {
	input := &paramapi.GetParameterHistoryInput{
		Name:           lo.ToPtr(name),
		WithDecryption: lo.ToPtr(true),
	}

	var allHistory []paramapi.ParameterHistory

	// Paginate through all history
	for {
		output, err := a.client.GetParameterHistory(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to get parameter history: %w", err)
		}

		allHistory = append(allHistory, output.Parameters...)

		if output.NextToken == nil {
			break
		}

		input.NextToken = output.NextToken
	}

	return convertParameterHistory(name, allHistory), nil
}

// ListParameters lists parameters matching the given path prefix.
func (a *Adapter) ListParameters(
	ctx context.Context, path string, recursive bool,
) ([]*model.ParameterListItem, error) {
	input := &paramapi.DescribeParametersInput{
		ParameterFilters: []paramapi.ParameterStringFilter{
			{
				Key:    lo.ToPtr("Path"),
				Values: []string{path},
				Option: lo.If(recursive, lo.ToPtr("Recursive")).Else(lo.ToPtr("OneLevel")),
			},
		},
	}

	var items []*model.ParameterListItem

	// Paginate through all parameters
	paginator := paramapi.NewDescribeParametersPaginator(a.client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list parameters: %w", err)
		}

		for _, param := range output.Parameters {
			items = append(items, convertParameterMetadata(&param))
		}
	}

	return items, nil
}

// ============================================================================
// ParameterWriter Implementation
// ============================================================================

// PutParameter creates or updates a parameter.
func (a *Adapter) PutParameter(
	ctx context.Context, param *model.Parameter, overwrite bool,
) error {
	paramType := paramapi.ParameterTypeString

	input := &paramapi.PutParameterInput{
		Name:      lo.ToPtr(param.Name),
		Value:     lo.ToPtr(param.Value),
		Type:      paramType,
		Overwrite: lo.ToPtr(overwrite),
	}

	if param.Description != "" {
		input.Description = lo.ToPtr(param.Description)
	}

	// Add tags if present
	if len(param.Tags) > 0 {
		input.Tags = convertToAWSTags(param.Tags)
	}

	// Apply AWS-specific metadata if present
	if meta := param.AWSMeta(); meta != nil {
		if meta.Type != "" {
			input.Type = paramapi.ParameterType(meta.Type)
		}

		if meta.Tier != "" {
			input.Tier = paramapi.ParameterTier(meta.Tier)
		}

		if meta.DataType != "" {
			input.DataType = lo.ToPtr(meta.DataType)
		}

		if meta.AllowedPattern != "" {
			input.AllowedPattern = lo.ToPtr(meta.AllowedPattern)
		}

		if meta.Policies != "" {
			input.Policies = lo.ToPtr(meta.Policies)
		}
	}

	_, err := a.client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put parameter: %w", err)
	}

	return nil
}

// DeleteParameter deletes a parameter by name.
func (a *Adapter) DeleteParameter(ctx context.Context, name string) error {
	input := &paramapi.DeleteParameterInput{
		Name: lo.ToPtr(name),
	}

	_, err := a.client.DeleteParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	return nil
}

// ============================================================================
// ParameterTagger Implementation
// ============================================================================

// GetTags retrieves all tags for a parameter.
func (a *Adapter) GetTags(ctx context.Context, name string) (map[string]string, error) {
	input := &paramapi.ListTagsForResourceInput{
		ResourceId:   lo.ToPtr(name),
		ResourceType: paramapi.ResourceTypeForTaggingParameter,
	}

	output, err := a.client.ListTagsForResource(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return convertFromAWSTags(output.TagList), nil
}

// AddTags adds or updates tags on a parameter.
func (a *Adapter) AddTags(
	ctx context.Context, name string, tags map[string]string,
) error {
	input := &paramapi.AddTagsToResourceInput{
		ResourceId:   lo.ToPtr(name),
		ResourceType: paramapi.ResourceTypeForTaggingParameter,
		Tags:         convertToAWSTags(tags),
	}

	_, err := a.client.AddTagsToResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to add tags: %w", err)
	}

	return nil
}

// RemoveTags removes tags from a parameter by key names.
func (a *Adapter) RemoveTags(
	ctx context.Context, name string, keys []string,
) error {
	input := &paramapi.RemoveTagsFromResourceInput{
		ResourceId:   lo.ToPtr(name),
		ResourceType: paramapi.ResourceTypeForTaggingParameter,
		TagKeys:      keys,
	}

	_, err := a.client.RemoveTagsFromResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to remove tags: %w", err)
	}

	return nil
}

// ============================================================================
// Compile-time Interface Checks
// ============================================================================

var (
	_ provider.ParameterReader  = (*Adapter)(nil)
	_ provider.ParameterWriter  = (*Adapter)(nil)
	_ provider.ParameterTagger  = (*Adapter)(nil)
	_ provider.ParameterService = (*Adapter)(nil)
)

// ============================================================================
// Conversion Helpers (internal)
// ============================================================================

func convertParameter(p *paramapi.Parameter) *model.Parameter {
	if p == nil {
		return nil
	}

	version := ""
	if p.Version != 0 {
		version = strconv.FormatInt(p.Version, 10)
	}

	return &model.Parameter{
		Name:         lo.FromPtr(p.Name),
		Value:        lo.FromPtr(p.Value),
		Version:      version,
		LastModified: p.LastModifiedDate,
		Metadata: model.AWSParameterMeta{
			Type:     string(p.Type),
			ARN:      lo.FromPtr(p.ARN),
			DataType: lo.FromPtr(p.DataType),
		},
	}
}

func convertParameterHistory(name string, history []paramapi.ParameterHistory) *model.ParameterHistory {
	params := make([]*model.Parameter, len(history))
	for i, h := range history {
		version := ""
		if h.Version != 0 {
			version = strconv.FormatInt(h.Version, 10)
		}

		params[i] = &model.Parameter{
			Name:         name,
			Value:        lo.FromPtr(h.Value),
			Version:      version,
			Description:  lo.FromPtr(h.Description),
			LastModified: h.LastModifiedDate,
			Metadata: model.AWSParameterMeta{
				Type:           string(h.Type),
				Tier:           string(h.Tier),
				AllowedPattern: lo.FromPtr(h.AllowedPattern),
				Policies:       policiesToString(h.Policies),
			},
		}
	}

	return &model.ParameterHistory{
		Name:       name,
		Parameters: params,
	}
}

func convertParameterMetadata(m *paramapi.ParameterMetadata) *model.ParameterListItem {
	if m == nil {
		return nil
	}

	return &model.ParameterListItem{
		Name:         lo.FromPtr(m.Name),
		Description:  lo.FromPtr(m.Description),
		LastModified: m.LastModifiedDate,
		Metadata: model.AWSParameterListItemMeta{
			Type: string(m.Type),
		},
	}
}

func convertToAWSTags(tags map[string]string) []paramapi.Tag {
	result := make([]paramapi.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, paramapi.Tag{
			Key:   lo.ToPtr(k),
			Value: lo.ToPtr(v),
		})
	}

	return result
}

func convertFromAWSTags(tags []paramapi.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			result[*tag.Key] = *tag.Value
		}
	}

	return result
}

func policiesToString(policies []paramapi.ParameterInlinePolicy) string {
	if len(policies) == 0 {
		return ""
	}
	// For simplicity, just return the first policy's document
	if policies[0].PolicyText != nil {
		return *policies[0].PolicyText
	}

	return ""
}

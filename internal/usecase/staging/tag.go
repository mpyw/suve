package staging

import (
	"context"
	"errors"
	"time"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/transition"
)

// TagInput holds input for the tag staging use case.
type TagInput struct {
	Name string
	Tags map[string]string
}

// TagOutput holds the result of the tag staging use case.
type TagOutput struct {
	Name string
}

// UntagInput holds input for the untag staging use case.
type UntagInput struct {
	Name    string
	TagKeys maputil.Set[string]
}

// UntagOutput holds the result of the untag staging use case.
type UntagOutput struct {
	Name string
}

// TagUseCase executes tag staging operations.
type TagUseCase struct {
	Strategy staging.EditStrategy
	Store    staging.StoreReadWriter
}

// tagContext holds common context for tag operations.
type tagContext struct {
	service        staging.Service
	name           string
	entryState     transition.EntryState
	stagedTags     transition.StagedTags
	baseModifiedAt *time.Time
}

// loadTagContext loads common context needed for both Tag and Untag operations.
func (u *TagUseCase) loadTagContext(ctx context.Context, inputName string) (*tagContext, error) {
	service := u.Strategy.Service()

	// Parse and validate name
	name, err := u.Strategy.ParseName(inputName)
	if err != nil {
		return nil, err
	}

	// Fetch AWS resource to check existence and get base modified time
	currentValue, awsBaseModifiedAt, err := u.fetchAWSCurrentValue(ctx, name)
	if err != nil {
		return nil, err
	}

	// Load current entry state with CurrentValue for existence check in reducer
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, name, currentValue)
	if err != nil {
		return nil, err
	}

	// Load current staged tags
	stagedTags, baseModifiedAt, err := transition.LoadStagedTags(ctx, u.Store, service, name)
	if err != nil {
		return nil, err
	}

	// Use AWS base modified time if we don't have one yet
	if baseModifiedAt == nil {
		baseModifiedAt = awsBaseModifiedAt
	}

	return &tagContext{
		service:        service,
		name:           name,
		entryState:     entryState,
		stagedTags:     stagedTags,
		baseModifiedAt: baseModifiedAt,
	}, nil
}

// fetchAWSCurrentValue fetches the current value from AWS.
// Returns (value, lastModified, nil) if resource exists, (nil, nil, nil) if not found.
func (u *TagUseCase) fetchAWSCurrentValue(ctx context.Context, name string) (*string, *time.Time, error) {
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		// If resource doesn't exist, return nil
		if notFoundErr := (*staging.ResourceNotFoundError)(nil); errors.As(err, &notFoundErr) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var baseModifiedAt *time.Time
	if !result.LastModified.IsZero() {
		baseModifiedAt = &result.LastModified
	}
	return &result.Value, baseModifiedAt, nil
}

// Tag adds or updates tags on a staged resource.
func (u *TagUseCase) Tag(ctx context.Context, input TagInput) (*TagOutput, error) {
	if len(input.Tags) == 0 {
		return nil, errors.New("no tags specified")
	}

	tc, err := u.loadTagContext(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	// Build tag action
	// CurrentAWSTags is nil to disable auto-skip (conservative approach)
	// TODO: Extend Strategy interface to fetch current tags for proper auto-skip
	action := transition.TagActionTag{
		Tags:           input.Tags,
		CurrentAWSTags: nil,
	}

	// Execute the transition
	executor := transition.NewExecutor(u.Store)
	_, err = executor.ExecuteTag(ctx, tc.service, tc.name, tc.entryState, tc.stagedTags, action, tc.baseModifiedAt)
	if err != nil {
		return nil, err
	}

	return &TagOutput{Name: tc.name}, nil
}

// Untag removes tags from a staged resource.
func (u *TagUseCase) Untag(ctx context.Context, input UntagInput) (*UntagOutput, error) {
	if input.TagKeys.Len() == 0 {
		return nil, errors.New("no tag keys specified")
	}

	tc, err := u.loadTagContext(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	// For CREATE, use empty set (no tags on AWS) to enable auto-skip
	// For others, use nil to disable auto-skip (conservative approach)
	var currentAWSTagKeys maputil.Set[string]
	if _, isCreate := tc.entryState.StagedState.(transition.EntryStagedStateCreate); isCreate {
		currentAWSTagKeys = maputil.NewSet[string]()
	}

	// Build untag action
	action := transition.TagActionUntag{
		Keys:              input.TagKeys,
		CurrentAWSTagKeys: currentAWSTagKeys,
	}

	// Execute the transition
	executor := transition.NewExecutor(u.Store)
	_, err = executor.ExecuteTag(ctx, tc.service, tc.name, tc.entryState, tc.stagedTags, action, tc.baseModifiedAt)
	if err != nil {
		return nil, err
	}

	return &UntagOutput{Name: tc.name}, nil
}

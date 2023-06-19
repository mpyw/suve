package api

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/typeconv"
	"github.com/mpyw/suve/pkg/api/apicore"
	"github.com/mpyw/suve/pkg/core/revisioning"
	"github.com/mpyw/suve/pkg/core/versioning"
	"golang.org/x/exp/constraints"
)

var _ API = (*Manager)(nil)

// API provides high-level interfaces which can handle relative versions.
type API interface {
	GetRevision(ctx context.Context, input GetRevisionInput) (*revisioning.Revision, error)
	ListRevisions(ctx context.Context, input ListRevisionsInput) (apicore.RevisionList, error)
	GetCanonicalVersion(ctx context.Context, input GetCanonicalVersionInput) (versioning.CanonicalVersion, error)
	ListVersions(ctx context.Context, input ListVersionsInput) (apicore.VersionList, error)
}

type Manager struct {
	client apicore.APICore
}

type GetRevisionInput struct {
	Name               string
	VersionRequirement *versioning.VersionRequirement
	// MaxResultsToSearch is passed to an AWS native client.
	MaxResultsToSearch *int32
}

type ListRevisionsInput struct {
	Name                    string
	StartVersionRequirement *versioning.VersionRequirement
	// MaxResults indicates the actual max records to be returned.
	MaxResults *int32
	// MaxResultsToSearch is passed to an AWS native client.
	MaxResultsToSearch *int32
}

type GetCanonicalVersionInput struct {
	Name               string
	VersionRequirement versioning.VersionRequirement
	// MaxResultsToSearch is passed to an AWS native client.
	MaxResultsToSearch *int32
}

type ListVersionsInput struct {
	Name                    string
	StartVersionRequirement *versioning.VersionRequirement
	// MaxResults indicates the actual max records to be returned.
	MaxResults *int32
	// MaxResultsToSearch is passed to an AWS native client.
	MaxResultsToSearch *int32
}

func (m *Manager) GetRevision(ctx context.Context, input GetRevisionInput) (*revisioning.Revision, error) {
	// Get the reference revision to which the shift has not been applied
	var requirement *versioning.AbsoluteVersionRequirement
	if input.VersionRequirement != nil {
		requirement = typeconv.Ref((*input.VersionRequirement).WithoutShift())
	}
	if input.VersionRequirement == nil || (*input.VersionRequirement).GetNumberOfShift() < 1 {
		// Returns this result when shift processing is not required
		return m.client.GetRevision(ctx, apicore.GetRevisionInput{
			Name:               input.Name,
			VersionRequirement: requirement,
		})
	}
	baseCanonicalVersion, err := m.client.GetCanonicalVersion(ctx, apicore.GetCanonicalVersionInput{
		Name:               input.Name,
		VersionRequirement: *requirement,
	})
	if err != nil {
		return nil, err
	}

	if client, ok := m.client.(apicore.ParameterStoreAPI); ok {
		// Optimization to retrieve revision content along with version for Parameter Store
		revisions, err := client.ListRevisions(ctx, apicore.ListRevisionsInput{
			Name:       input.Name,
			MaxResults: input.MaxResultsToSearch,
		})
		if err != nil {
			return nil, err
		}
		// Apply shift processing
		shifted := findShiftedValue(revisions.Items, revisionEqualsTo(baseCanonicalVersion), (*input.VersionRequirement).GetNumberOfShift())
		if shifted == nil {
			return nil, fmt.Errorf("%w: %+v", versioning.ErrVersionNotFound, *input.VersionRequirement)
		}
		return *shifted, nil
	} else {
		versions, err := m.client.ListVersions(ctx, apicore.ListVersionsInput{
			Name:       input.Name,
			MaxResults: input.MaxResultsToSearch,
		})
		if err != nil {
			return nil, err
		}
		// Apply shift processing
		shifted := findShiftedValue(versions.Items, versionEqualsTo(baseCanonicalVersion), (*input.VersionRequirement).GetNumberOfShift())
		if shifted == nil {
			return nil, fmt.Errorf("%w: %+v", versioning.ErrVersionNotFound, *input.VersionRequirement)
		}
		return m.client.GetRevision(ctx, apicore.GetRevisionInput{
			Name: input.Name,
			VersionRequirement: &versioning.AbsoluteVersionRequirement{
				Type:           versioning.VersionRequirementTypeCanonical,
				CanonicalValue: shifted.CanonicalVersion,
			},
		})
	}
}

func (m *Manager) ListRevisions(ctx context.Context, input ListRevisionsInput) (apicore.RevisionList, error) {
	if client, ok := m.client.(apicore.ParameterStoreAPI); ok {
		// Optimization to retrieve revision content along with version for Parameter Store
		list, err := client.ListRevisions(ctx, apicore.ListRevisionsInput{
			Name:       input.Name,
			MaxResults: input.MaxResultsToSearch,
		})
		if err != nil {
			return apicore.RevisionList{}, err
		}
		if input.StartVersionRequirement != nil {
			// Get the reference revision to which the shift has not been applied
			startBaseCanonicalVersion, err := m.client.GetCanonicalVersion(ctx, apicore.GetCanonicalVersionInput{
				Name:               input.Name,
				VersionRequirement: (*input.StartVersionRequirement).WithoutShift(),
			})
			if err != nil {
				return apicore.RevisionList{}, err
			}
			// Apply shift processing
			startIndex := findShiftedIndex(list.Items, revisionEqualsTo(startBaseCanonicalVersion), (*input.StartVersionRequirement).GetNumberOfShift())
			if startIndex == nil {
				return apicore.RevisionList{}, fmt.Errorf("%w: %+v", versioning.ErrVersionNotFound, *input.StartVersionRequirement)
			}
			list.Items = list.Items[*startIndex:]
		}
		if input.MaxResults != nil && int32(len(list.Items)) > *input.MaxResults {
			// Cut out only the necessary number of items
			list.Items = list.Items[:*input.MaxResults]
		}
		return list, nil
	} else {
		// If optimization is not available, loop through the results of ListVersion and call GetRevision one by one
		versions, err := m.ListVersions(ctx, ListVersionsInput(input))
		if err != nil {
			return apicore.RevisionList{}, err
		}
		var revisions apicore.RevisionList
		for _, version := range versions.Items {
			revision, err := m.client.GetRevision(ctx, apicore.GetRevisionInput{
				Name:               input.Name,
				VersionRequirement: typeconv.Ref(version.AsRequirement()),
			})
			if err != nil {
				return revisions, err
			}
			revisions.Items = append(revisions.Items, revision)
		}
		return revisions, nil
	}
}

func (m *Manager) GetCanonicalVersion(ctx context.Context, input GetCanonicalVersionInput) (versioning.CanonicalVersion, error) {
	// Get the reference revision to which the shift has not been applied
	baseCanonicalVersion, err := m.client.GetCanonicalVersion(ctx, apicore.GetCanonicalVersionInput{
		Name:               input.Name,
		VersionRequirement: input.VersionRequirement.WithoutShift(),
	})
	if err != nil {
		return versioning.CanonicalVersion{}, err
	}
	if input.VersionRequirement.GetNumberOfShift() < 1 {
		// Returns this result when shift processing is not required
		return baseCanonicalVersion, nil
	}
	versions, err := m.client.ListVersions(ctx, apicore.ListVersionsInput{
		Name:       input.Name,
		MaxResults: input.MaxResultsToSearch,
	})
	if err != nil {
		return versioning.CanonicalVersion{}, err
	}
	// Apply shift processing
	shifted := findShiftedValue(versions.Items, versionEqualsTo(baseCanonicalVersion), input.VersionRequirement.GetNumberOfShift())
	if shifted == nil {
		return versioning.CanonicalVersion{}, fmt.Errorf("%w: %+v", versioning.ErrVersionNotFound, input.VersionRequirement)
	}
	return shifted.CanonicalVersion, nil
}

func (m *Manager) ListVersions(ctx context.Context, input ListVersionsInput) (apicore.VersionList, error) {
	list, err := m.client.ListVersions(ctx, apicore.ListVersionsInput{
		Name:       input.Name,
		MaxResults: input.MaxResultsToSearch,
	})
	if err != nil {
		return apicore.VersionList{}, err
	}
	if input.StartVersionRequirement != nil {
		// Optimization to retrieve revision content along with version for Parameter Store
		startBaseCanonicalVersion, err := m.client.GetCanonicalVersion(ctx, apicore.GetCanonicalVersionInput{
			Name:               input.Name,
			VersionRequirement: (*input.StartVersionRequirement).WithoutShift(),
		})
		if err != nil {
			return apicore.VersionList{}, err
		}
		// Apply shift processing
		startIndex := findShiftedIndex(list.Items, versionEqualsTo(startBaseCanonicalVersion), (*input.StartVersionRequirement).GetNumberOfShift())
		if startIndex == nil {
			return apicore.VersionList{}, fmt.Errorf("%w: %+v", versioning.ErrVersionNotFound, *input.StartVersionRequirement)
		}
		list.Items = list.Items[*startIndex:]
	}
	if input.MaxResults != nil && int32(len(list.Items)) > *input.MaxResults {
		// Cut out only the necessary number of items
		list.Items = list.Items[:*input.MaxResults]
	}
	return list, nil
}

func New(client apicore.APICore) *Manager {
	return &Manager{client}
}

// findShiftedIndex calculates the index of "the first position that satisfies condition + shift".
func findShiftedIndex[T any, S constraints.Integer](items []T, startPositionEqualityFn func(T) bool, shift S) *int {
	var startPosition *int
	for i, item := range items {
		if startPosition == nil && startPositionEqualityFn(item) {
			startPosition = typeconv.Ref(i)
		}
		if startPosition != nil && i == *startPosition+int(shift) {
			return typeconv.Ref(i)
		}
	}
	return nil
}

// findShiftedValue calculates the item of "the first position that satisfies condition + shift".
func findShiftedValue[T any, S constraints.Integer](items []T, startPositionEqualityFn func(T) bool, shift S) *T {
	index := findShiftedIndex(items, startPositionEqualityFn, shift)
	if index == nil {
		return nil
	}
	return &items[*index]
}

// versionEqualsTo defines equality check function for findShiftedIndex and findShiftedValue.
func versionEqualsTo(target versioning.CanonicalVersion) func(versioning.Version) bool {
	return func(v versioning.Version) bool {
		return v.CanonicalVersion == target
	}
}

// revisionEqualsTo defines equality check function for findShiftedIndex and findShiftedValue.
func revisionEqualsTo(target versioning.CanonicalVersion) func(revision *revisioning.Revision) bool {
	return func(r *revisioning.Revision) bool {
		return r.Version.CanonicalVersion == target
	}
}

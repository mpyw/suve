package apicore

import (
	"context"
	"errors"

	"github.com/mpyw/suve/pkg/core/revisioning"
	"github.com/mpyw/suve/pkg/core/versioning"
)

var ErrUnsupportedParameterType = errors.New("unsupported parameter type")

// APICore abstracts AWS SecretsManager and Parameter Store clients.
// Note that neither APICore nor ParameterStoreAPI can handle relative versions.
type APICore interface {
	// GetRevision retrieves a record of the latest or specific version.
	GetRevision(ctx context.Context, input GetRevisionInput) (*revisioning.Revision, error)
	// GetCanonicalVersion resolves a labeled/staged version to return a deterministic canonical version.
	GetCanonicalVersion(ctx context.Context, input GetCanonicalVersionInput) (versioning.CanonicalVersion, error)
	// ListVersions retrieves records of recent multiple versions.
	ListVersions(ctx context.Context, input ListVersionsInput) (VersionList, error)
}

// ParameterStoreAPI provides extended features for Parameter Store.
type ParameterStoreAPI interface {
	APICore
	// ListRevisions retrieves records of recent multiple versions and their revisions.
	ListRevisions(ctx context.Context, input ListRevisionsInput) (RevisionList, error)
}

type GetRevisionInput struct {
	Name               string
	VersionRequirement *versioning.AbsoluteVersionRequirement
}

type ListRevisionsInput struct {
	Name       string
	MaxResults *int32
}

type GetCanonicalVersionInput struct {
	Name               string
	VersionRequirement versioning.AbsoluteVersionRequirement
}

type ListVersionsInput struct {
	Name       string
	MaxResults *int32
}

type RevisionList struct {
	Items []*revisioning.Revision // sorted
}

type VersionList struct {
	Items []versioning.Version // sorted
}

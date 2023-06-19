package secretsmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/google/uuid"
	"github.com/mpyw/suve/internal/typeconv"
	"github.com/mpyw/suve/pkg/api/apicore"
	"github.com/mpyw/suve/pkg/core/revisioning"
	"github.com/mpyw/suve/pkg/core/versioning"
	"golang.org/x/exp/slices"
)

var _ apicore.APICore = (*Manager)(nil)

type Manager struct {
	client            *secretsmanager.Client
	includeDeprecated bool
}

func (m *Manager) GetRevision(ctx context.Context, input apicore.GetRevisionInput) (*revisioning.Revision, error) {
	clientInput, err := applyVersionRequirementToInput(secretsmanager.GetSecretValueInput{
		SecretId: aws.String(input.Name),
	}, input.VersionRequirement)
	if err != nil {
		return nil, err
	}
	secret, err := m.client.GetSecretValue(ctx, &clientInput)
	if err != nil {
		return nil, err
	}
	uuidValue, err := uuid.Parse(*secret.VersionId)
	if err != nil {
		return nil, err
	}
	content, err := revisionContentFromSecret(secret)
	if err != nil {
		return nil, err
	}
	return &revisioning.Revision{
		Version: versioning.Version{
			CanonicalVersion: versioning.CanonicalVersion{
				Type:      versioning.CanonicalVersionTypeUUID,
				UUIDValue: uuidValue,
			},
			StagesOrLabels: secret.VersionStages,
		},
		Content: content,
		Date:    typeconv.NonNilOrEmpty(secret.CreatedDate),
	}, nil
}

func (m *Manager) GetCanonicalVersion(ctx context.Context, input apicore.GetCanonicalVersionInput) (versioning.CanonicalVersion, error) {
	if input.VersionRequirement.Type == versioning.VersionRequirementTypeCanonical {
		return input.VersionRequirement.CanonicalValue, nil
	}
	revision, err := m.GetRevision(ctx, apicore.GetRevisionInput{
		Name:               input.Name,
		VersionRequirement: &input.VersionRequirement,
	})
	if err != nil {
		return versioning.CanonicalVersion{}, err
	}
	return revision.Version.CanonicalVersion, nil
}

func (m *Manager) ListVersions(ctx context.Context, input apicore.ListVersionsInput) (apicore.VersionList, error) {
	var list apicore.VersionList
	secrets, err := m.client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId:          aws.String(input.Name),
		IncludeDeprecated: aws.Bool(m.includeDeprecated),
		MaxResults:        input.MaxResults,
	})
	if err != nil {
		return list, err
	}
	// Temporarily combine versions and dates for sorting.
	type VersionWithTime struct {
		version versioning.Version
		date    time.Time
	}
	var versionsWithTime []VersionWithTime
	for _, secret := range secrets.Versions {
		uuidValue, err := uuid.Parse(*secret.VersionId)
		if err != nil {
			return list, err
		}
		versionsWithTime = append(versionsWithTime, VersionWithTime{
			version: versioning.Version{
				CanonicalVersion: versioning.CanonicalVersion{
					Type:        versioning.CanonicalVersionTypeUUID,
					NumberValue: 0,
					UUIDValue:   uuidValue,
				},
				StagesOrLabels: secret.VersionStages,
			},
			date: *secret.CreatedDate,
		})
	}
	// Important:
	//   The ListSecretVersionIds() results are not guaranteed to be sorted in descending order.
	//   Items can be sorted by version creation dates.
	slices.SortFunc(versionsWithTime, func(a, b VersionWithTime) bool {
		return a.date.After(b.date)
	})
	for _, v := range versionsWithTime {
		list.Items = append(list.Items, v.version)
	}
	return list, nil
}

func New(cfg aws.Config, includeDeprecated bool) *Manager {
	return &Manager{secretsmanager.NewFromConfig(cfg), includeDeprecated}
}

func applyVersionRequirementToInput(input secretsmanager.GetSecretValueInput, version *versioning.AbsoluteVersionRequirement) (secretsmanager.GetSecretValueInput, error) {
	if version == nil {
		return input, nil
	}
	switch version.Type {
	case versioning.VersionRequirementTypeStageOrLabel:
		input.VersionStage = aws.String(version.StageOrLabelValue)
		return input, nil
	case versioning.VersionRequirementTypeCanonical:
		if version.CanonicalValue.Type != versioning.CanonicalVersionTypeUUID {
			return input, fmt.Errorf("%w: %s", versioning.ErrUnsupportedCanonicalVersion, version.CanonicalValue.Type)
		}
		input.VersionId = aws.String(version.CanonicalValue.UUIDValue.String())
		return input, nil
	default:
		return input, fmt.Errorf("%w: %s", versioning.ErrUnsupportedVersionRequirement, version.Type)
	}
}

func revisionContentFromSecret(secret *secretsmanager.GetSecretValueOutput) (*revisioning.RevisionContent, error) {
	if secret.SecretString != nil {
		return &revisioning.RevisionContent{
			Type:              revisioning.RevisionContentTypeString,
			StringValue:       *secret.SecretString,
			EncryptionEnabled: true,
		}, nil
	}
	if secret.SecretBinary != nil {
		return &revisioning.RevisionContent{
			Type:              revisioning.RevisionContentTypeBytes,
			BytesValue:        secret.SecretBinary,
			EncryptionEnabled: true,
		}, nil
	}
	return nil, fmt.Errorf("%w: either SecretString or SecretBinary is empty", revisioning.ErrInvalidRevisionContent)
}

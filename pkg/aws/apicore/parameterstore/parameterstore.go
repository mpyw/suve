package parameterstore

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/mpyw/suve/internal/typeconv"
	"github.com/mpyw/suve/pkg/api/apicore"
	"github.com/mpyw/suve/pkg/core/revisioning"
	"github.com/mpyw/suve/pkg/core/versioning"
	"golang.org/x/exp/slices"
)

var _ apicore.APICore = (*Manager)(nil)

type Manager struct {
	client         *ssm.Client
	withDecryption bool
}

func (m *Manager) GetRevision(ctx context.Context, input apicore.GetRevisionInput) (*revisioning.Revision, error) {
	versionAppliedName, err := applyVersionRequirementToName(input.Name, input.VersionRequirement)
	if err != nil {
		return nil, err
	}
	param, err := m.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(versionAppliedName),
		WithDecryption: aws.Bool(m.withDecryption),
	})
	if err != nil {
		return nil, err
	}
	content, err := revisionContentFromParameter(param.Parameter)
	if err != nil {
		return nil, err
	}
	return &revisioning.Revision{
		Version: versioning.Version{
			CanonicalVersion: versioning.CanonicalVersion{
				Type:        versioning.CanonicalVersionTypeNumber,
				NumberValue: param.Parameter.Version,
			},
			StagesOrLabels: nil, // Currently labels cannot be retrieved,
		},
		Content: content,
		Date:    typeconv.NonNilOrEmpty(param.Parameter.LastModifiedDate),
	}, nil
}

func (m *Manager) ListRevisions(ctx context.Context, input apicore.ListRevisionsInput) (apicore.RevisionList, error) {
	var list apicore.RevisionList
	history, err := m.client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(input.Name),
		WithDecryption: aws.Bool(m.withDecryption),
		MaxResults:     input.MaxResults,
	})
	if err != nil {
		return list, err
	}
	for _, param := range history.Parameters {
		content, err := revisionContentFromHistory(typeconv.Ref(param))
		if err != nil {
			return list, err
		}
		list.Items = append(list.Items, &revisioning.Revision{
			Version: versioning.Version{
				CanonicalVersion: versioning.CanonicalVersion{
					Type:        versioning.CanonicalVersionTypeNumber,
					NumberValue: param.Version,
				},
				StagesOrLabels: nil, // Currently labels cannot be retrieved
			},
			Content: content,
			Date:    *param.LastModifiedDate,
		})
	}
	// Important:
	//   The GetParameterHistory() results are not guaranteed to be sorted in descending order.
	//   Items can be sorted by version numbers.
	slices.SortFunc(list.Items, func(a, b *revisioning.Revision) bool {
		return a.Version.CanonicalVersion.NumberValue > b.Version.CanonicalVersion.NumberValue
	})
	return list, nil
}

func (m *Manager) GetCanonicalVersion(ctx context.Context, input apicore.GetCanonicalVersionInput) (versioning.CanonicalVersion, error) {
	if input.VersionRequirement.Type == versioning.VersionRequirementTypeCanonical {
		return input.VersionRequirement.CanonicalValue, nil
	}
	// Extract a version from its revision
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
	history, err := m.client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(input.Name),
		WithDecryption: aws.Bool(m.withDecryption),
		MaxResults:     input.MaxResults,
	})
	if err != nil {
		return list, err
	}
	for _, param := range history.Parameters {
		list.Items = append(list.Items, versioning.Version{
			CanonicalVersion: versioning.CanonicalVersion{
				Type:        versioning.CanonicalVersionTypeNumber,
				NumberValue: param.Version,
			},
			StagesOrLabels: nil, // Currently labels cannot be retrieved
		})
	}
	// Important:
	//   The GetParameterHistory() results are not guaranteed to be sorted in descending order.
	//   Items can be sorted by version numbers.
	slices.SortFunc(list.Items, func(a, b versioning.Version) bool {
		return a.CanonicalVersion.NumberValue > b.CanonicalVersion.NumberValue
	})
	return list, nil
}

func New(cfg aws.Config, withDecryption bool) *Manager {
	return &Manager{ssm.NewFromConfig(cfg), withDecryption}
}

func applyVersionRequirementToName(name string, version *versioning.AbsoluteVersionRequirement) (string, error) {
	if version == nil {
		return name, nil
	}
	switch version.Type {
	case versioning.VersionRequirementTypeCanonical:
		if version.CanonicalValue.Type != versioning.CanonicalVersionTypeNumber {
			return name, fmt.Errorf("%w: %s", versioning.ErrUnsupportedCanonicalVersion, version.CanonicalValue.Type)
		}
		// "parameter-xxx:123"
		return name + ":" + strconv.Itoa(int(version.CanonicalValue.NumberValue)), nil
	case versioning.VersionRequirementTypeStageOrLabel:
		// "parameter-xxx:development"
		return name + ":" + version.StageOrLabelValue, nil
	default:
		return name, fmt.Errorf("%w: %s", versioning.ErrUnsupportedVersionRequirement, version.Type)
	}
}

func revisionContentFromParameter(param *types.Parameter) (*revisioning.RevisionContent, error) {
	switch param.Type {
	case types.ParameterTypeString, types.ParameterTypeStringList, types.ParameterTypeSecureString:
		return &revisioning.RevisionContent{
			Type:              revisioning.RevisionContentTypeString,
			StringValue:       typeconv.NonNilOrEmpty(param.Value),
			EncryptionEnabled: param.Type == types.ParameterTypeSecureString,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %s", apicore.ErrUnsupportedParameterType, param.Type)
	}
}

func revisionContentFromHistory(hist *types.ParameterHistory) (*revisioning.RevisionContent, error) {
	return revisionContentFromParameter(&types.Parameter{
		DataType:         hist.DataType,
		LastModifiedDate: hist.LastModifiedDate,
		Name:             hist.Name,
		Type:             hist.Type,
		Value:            hist.Value,
		Version:          hist.Version,
	})
}

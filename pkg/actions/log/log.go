package log

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/args"
	"github.com/mpyw/suve/internal/json"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/api"
	"github.com/mpyw/suve/pkg/core/revisioning"
	"github.com/mpyw/suve/pkg/core/versioning"
	"go.uber.org/multierr"
)

type ActionInput struct {
	actions.Dependencies
	Name               string
	PrettyJSON         bool
	Version            *versioning.VersionRequirement
	MaxResults         *int32
	MaxResultsToSearch *int32
}

func Action(ctx context.Context, input ActionInput) error {
	revisions, err := input.API.ListRevisions(ctx, api.ListRevisionsInput{
		Name:                    input.Name,
		StartVersionRequirement: input.Version,
		MaxResults:              input.MaxResults,
		MaxResultsToSearch:      input.MaxResultsToSearch,
	})
	if err != nil {
		return err
	}
	for _, revision := range revisions.Items {
		if revision.Content.Type != revisioning.RevisionContentTypeString {
			continue
		}
		if input.PrettyJSON {
			revision.Content.StringValue = json.Pretty(revision.Content.StringValue)
		}
	}
	for current := 1; current < len(revisions.Items)-1; current++ {
		newer := current - 1
		err = multierr.Combine(
			err,
			args.IgnoreFirst(fmt.Fprintln(input.Writer, input.Printer.GenerateVersionDescription(revisions.Items[current]))),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, "")),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, input.Printer.GenerateVersionDiffText(revisions.Items[current], revisions.Items[newer]))),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, "")),
		)
	}
	if len(revisions.Items) > 0 {
		last := revisions.Items[len(revisions.Items)-1]
		empty := &revisioning.Revision{
			Content: &revisioning.RevisionContent{
				Type: revisioning.RevisionContentTypeString,
			},
		}
		err = multierr.Combine(
			err,
			args.IgnoreFirst(fmt.Fprintln(input.Writer, input.Printer.GenerateVersionDescription(last))),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, "")),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, input.Printer.GenerateVersionDiffText(empty, last))),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, "")),
		)
	}
	return err
}

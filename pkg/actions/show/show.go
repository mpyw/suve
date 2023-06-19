package show

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/args"
	"github.com/mpyw/suve/internal/json"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/api"
	"github.com/mpyw/suve/pkg/core/versioning"
	"go.uber.org/multierr"
)

type ActionInput struct {
	actions.Dependencies
	Name               string
	PrettyJSON         bool
	Raw                bool
	Version            *versioning.VersionRequirement
	MaxResultsToSearch *int32
}

func Action(ctx context.Context, input ActionInput) error {
	revision, err := input.API.GetRevision(ctx, api.GetRevisionInput{
		Name:               input.Name,
		VersionRequirement: input.Version,
		MaxResultsToSearch: input.MaxResultsToSearch,
	})
	if err != nil {
		return err
	}
	content := revision.Content.String()
	if input.PrettyJSON {
		content = json.Pretty(content)
	}
	if !input.Raw {
		err = multierr.Combine(
			err,
			args.IgnoreFirst(fmt.Fprintln(input.Writer, input.Printer.GenerateVersionDescription(revision))),
			args.IgnoreFirst(fmt.Fprintln(input.Writer, "")),
		)
	}
	return multierr.Combine(err, args.IgnoreFirst(fmt.Fprintln(input.Writer, content)))
}

// Package aws provides the "suve aws" command group: the explicit,
// always-present home for the AWS backends (Parameter Store, Secrets Manager,
// and the AWS-only staging workflow). It mirrors the gcloud/azure groups so
// that AWS is a peer provider rather than a special top-level default. The
// top-level `param` / `secret` aliases are added separately (and only when AWS
// is the uniquely active provider); this group is what you always get with an
// explicit `suve aws ...`.
package aws

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/aws/param"
	"github.com/mpyw/suve/internal/cli/commands/aws/secret"
	"github.com/mpyw/suve/internal/cli/commands/aws/stage"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// Command returns the aws command group with the param (Parameter Store),
// secret (Secrets Manager), and stage (staging) subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "aws",
		Usage: "Interact with AWS Parameter Store / Secrets Manager (and staging)",
		Description: `Interact with Amazon Web Services parameter and secret stores.

  - "suve aws param"  targets Systems Manager Parameter Store.
  - "suve aws secret" targets Secrets Manager.
  - "suve aws stage"  stages changes to the above before applying them.

Region and credentials come from the ambient AWS configuration (environment,
shared config/credentials files, or an instance role).`,
		Commands: []*cli.Command{
			param.Command(),
			secret.Command(),
			stage.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

package sm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	internalaws "github.com/mpyw/suve/internal/aws"
)

func rmCommand() *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "Delete a secret",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Force deletion without recovery window",
			},
			&cli.IntFlag{
				Name:  "recovery-window",
				Usage: "Number of days before permanent deletion (7-30)",
				Value: 30,
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return fmt.Errorf("usage: suve sm rm <name>")
			}
			ctx := c.Context
			cfg, err := internalaws.LoadConfig(ctx)
			if err != nil {
				return err
			}
			client := secretsmanager.NewFromConfig(cfg)
			name := c.Args().First()
			force := c.Bool("force")
			recoveryWindow := c.Int("recovery-window")
			return runRm(ctx, c.App.Writer, client, name, force, recoveryWindow)
		},
	}
}

func runRm(ctx context.Context, w io.Writer, client RmClient, name string, force bool, recoveryWindow int) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: aws.String(name),
	}

	if force {
		input.ForceDeleteWithoutRecovery = aws.Bool(true)
	} else {
		input.RecoveryWindowInDays = aws.Int64(int64(recoveryWindow))
	}

	result, err := client.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	if force {
		_, _ = fmt.Fprintf(w, "%s Permanently deleted secret %s\n",
			yellow("!"),
			aws.ToString(result.Name),
		)
	} else {
		_, _ = fmt.Fprintf(w, "%s Scheduled deletion of secret %s (deletion date: %s)\n",
			yellow("!"),
			aws.ToString(result.Name),
			result.DeletionDate.Format("2006-01-02"),
		)
	}

	return nil
}

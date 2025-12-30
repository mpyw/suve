package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/fatih/color"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
)

var smCommand = &cli.Command{
	Name:    "sm",
	Aliases: []string{"secret"},
	Usage:   "Interact with AWS Secrets Manager",
	Subcommands: []*cli.Command{
		smShowCommand,
		smCatCommand,
		smLogCommand,
		smDiffCommand,
		smLsCommand,
		smCreateCommand,
		smSetCommand,
		smRmCommand,
		smRestoreCommand,
	},
}

var smShowCommand = &cli.Command{
	Name:      "show",
	Usage:     "Show secret value with metadata",
	ArgsUsage: "<name[@version][~shift][:label]>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "json",
			Aliases: []string{"j"},
			Usage:   "Pretty print JSON values",
		},
	},
	Action: smShowAction,
}

var smCatCommand = &cli.Command{
	Name:      "cat",
	Usage:     "Output raw secret value (for piping)",
	ArgsUsage: "<name[@version][~shift][:label]>",
	Action:    smCatAction,
}

var smLogCommand = &cli.Command{
	Name:      "log",
	Usage:     "Show secret version history",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "number",
			Aliases: []string{"n"},
			Value:   10,
			Usage:   "Number of versions to show",
		},
	},
	Action: smLogAction,
}

var smDiffCommand = &cli.Command{
	Name:      "diff",
	Usage:     "Show diff between two versions",
	ArgsUsage: "<name> <version1> <version2>",
	Action:    smDiffAction,
}

var smLsCommand = &cli.Command{
	Name:      "ls",
	Usage:     "List secrets",
	ArgsUsage: "[filter]",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "max",
			Aliases: []string{"m"},
			Value:   50,
			Usage:   "Maximum number of secrets to list",
		},
		&cli.StringSliceFlag{
			Name:    "filter",
			Aliases: []string{"f"},
			Usage:   "Filter by name (prefix match)",
		},
	},
	Action: smLsAction,
}

var smCreateCommand = &cli.Command{
	Name:      "create",
	Usage:     "Create a new secret",
	ArgsUsage: "<name> <value>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "description",
			Usage: "Secret description",
		},
	},
	Action: smCreateAction,
}

var smSetCommand = &cli.Command{
	Name:      "set",
	Usage:     "Update secret value",
	ArgsUsage: "<name> <value>",
	Action:    smSetAction,
}

var smRmCommand = &cli.Command{
	Name:      "rm",
	Usage:     "Delete secret",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Delete immediately without recovery window",
		},
		&cli.IntFlag{
			Name:  "recovery-window",
			Value: 30,
			Usage: "Number of days before permanent deletion (7-30)",
		},
	},
	Action: smRmAction,
}

var smRestoreCommand = &cli.Command{
	Name:      "restore",
	Usage:     "Restore a deleted secret",
	ArgsUsage: "<name>",
	Action:    smRestoreAction,
}

func smShowAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	secret, err := getSecretWithVersion(ctx, clients.SecretsManager, spec)
	if err != nil {
		return err
	}

	// Format output
	out := output.New(os.Stdout)
	out.Field("Name", aws.ToString(secret.Name))
	out.Field("ARN", aws.ToString(secret.ARN))
	if secret.VersionId != nil {
		out.Field("VersionId", aws.ToString(secret.VersionId))
	}
	if len(secret.VersionStages) > 0 {
		out.Field("Stages", fmt.Sprintf("%v", secret.VersionStages))
	}
	if secret.CreatedDate != nil {
		out.Field("Created", secret.CreatedDate.Format(time.RFC3339))
	}
	out.Separator()

	value := aws.ToString(secret.SecretString)
	if c.Bool("json") {
		value = prettyJSON(value)
	}
	out.Value(value)

	return nil
}

func smCatAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	spec, err := version.Parse(c.Args().First())
	if err != nil {
		return err
	}

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	secret, err := getSecretWithVersion(ctx, clients.SecretsManager, spec)
	if err != nil {
		return err
	}

	fmt.Print(aws.ToString(secret.SecretString))
	return nil
}

func smLogAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	result, err := clients.SecretsManager.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId:   aws.String(name),
		MaxResults: aws.Int32(maxResults),
	})
	if err != nil {
		return fmt.Errorf("failed to list secret versions: %w", err)
	}

	// Sort by created date descending
	versions := result.Versions
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedDate == nil {
			return false
		}
		if versions[j].CreatedDate == nil {
			return true
		}
		return versions[i].CreatedDate.After(*versions[j].CreatedDate)
	})

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	for i, v := range versions {
		versionLabel := fmt.Sprintf("Version %s", aws.ToString(v.VersionId)[:8])
		if len(v.VersionStages) > 0 {
			versionLabel += " " + green(fmt.Sprintf("%v", v.VersionStages))
		}
		fmt.Println(yellow(versionLabel))
		if v.CreatedDate != nil {
			fmt.Printf("%s %s\n", cyan("Date:"), v.CreatedDate.Format(time.RFC3339))
		}
		if i < len(versions)-1 {
			fmt.Println()
		}
	}

	return nil
}

func smDiffAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm diff <name> <version1> [version2]")
	}

	name := c.Args().Get(0)
	version1 := c.Args().Get(1)
	version2 := c.Args().Get(2)

	// If only one version specified, compare with current
	if version2 == "" {
		version2 = version1
		version1 = ":AWSCURRENT"
	}

	spec1, err := version.Parse(name + version1)
	if err != nil {
		return fmt.Errorf("invalid version1: %w", err)
	}

	spec2, err := version.Parse(name + version2)
	if err != nil {
		return fmt.Errorf("invalid version2: %w", err)
	}

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	secret1, err := getSecretWithVersion(ctx, clients.SecretsManager, spec1)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version1, err)
	}

	secret2, err := getSecretWithVersion(ctx, clients.SecretsManager, spec2)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version2, err)
	}

	diff := output.Diff(
		fmt.Sprintf("%s@%s", name, aws.ToString(secret1.VersionId)[:8]),
		fmt.Sprintf("%s@%s", name, aws.ToString(secret2.VersionId)[:8]),
		aws.ToString(secret1.SecretString),
		aws.ToString(secret2.SecretString),
	)
	fmt.Print(diff)

	return nil
}

func smLsAction(c *cli.Context) error {
	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	input := &secretsmanager.ListSecretsInput{
		MaxResults: aws.Int32(int32(c.Int("max"))),
	}

	// Add filters if specified
	if filters := c.StringSlice("filter"); len(filters) > 0 {
		for _, f := range filters {
			input.Filters = append(input.Filters, types.Filter{
				Key:    types.FilterNameStringTypeName,
				Values: []string{f},
			})
		}
	}

	result, err := clients.SecretsManager.ListSecrets(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	for _, secret := range result.SecretList {
		name := aws.ToString(secret.Name)
		modified := ""
		if secret.LastChangedDate != nil {
			modified = secret.LastChangedDate.Format("2006-01-02 15:04")
		}
		fmt.Printf("%s  %s\n", cyan(modified), name)
	}

	return nil
}

func smCreateAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm create <name> <value>")
	}

	name := c.Args().Get(0)
	value := c.Args().Get(1)

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(value),
	}

	if desc := c.String("description"); desc != "" {
		input.Description = aws.String(desc)
	}

	result, err := clients.SecretsManager.CreateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s %s (version %s)\n", green("Created"), name, aws.ToString(result.VersionId)[:8])

	return nil
}

func smSetAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm set <name> <value>")
	}

	name := c.Args().Get(0)
	value := c.Args().Get(1)

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	result, err := clients.SecretsManager.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(name),
		SecretString: aws.String(value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s %s (version %s)\n", green("Updated"), name, aws.ToString(result.VersionId)[:8])

	return nil
}

func smRmAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	name := c.Args().First()

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	input := &secretsmanager.DeleteSecretInput{
		SecretId: aws.String(name),
	}

	if c.Bool("force") {
		input.ForceDeleteWithoutRecovery = aws.Bool(true)
	} else {
		window := int64(c.Int("recovery-window"))
		input.RecoveryWindowInDays = aws.Int64(window)
	}

	result, err := clients.SecretsManager.DeleteSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	red := color.New(color.FgRed).SprintFunc()
	if c.Bool("force") {
		fmt.Printf("%s %s (permanently)\n", red("Deleted"), name)
	} else {
		fmt.Printf("%s %s (scheduled for %s)\n", red("Deleted"), name, result.DeletionDate.Format("2006-01-02"))
	}

	return nil
}

func smRestoreAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	name := c.Args().First()

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	_, err = clients.SecretsManager.RestoreSecret(ctx, &secretsmanager.RestoreSecretInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("failed to restore secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s %s\n", green("Restored"), name)

	return nil
}

// getSecretWithVersion retrieves a secret with version/shift/label support.
func getSecretWithVersion(ctx context.Context, client *secretsmanager.Client, spec *version.Spec) (*secretsmanager.GetSecretValueOutput, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(spec.Name),
	}

	// Handle label (stage)
	if spec.Label != nil {
		input.VersionStage = spec.Label
	}

	// Handle shift - need to list versions and find the right one
	if spec.HasShift() {
		versions, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
			SecretId: aws.String(spec.Name),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list versions: %w", err)
		}

		// Sort by created date descending
		versionList := versions.Versions
		sort.Slice(versionList, func(i, j int) bool {
			if versionList[i].CreatedDate == nil {
				return false
			}
			if versionList[j].CreatedDate == nil {
				return true
			}
			return versionList[i].CreatedDate.After(*versionList[j].CreatedDate)
		})

		if spec.Shift >= len(versionList) {
			return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
		}

		targetVersion := versionList[spec.Shift]
		input.VersionId = targetVersion.VersionId
	}

	return client.GetSecretValue(ctx, input)
}

// prettyJSON formats a JSON string with indentation.
func prettyJSON(s string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return s
	}
	pretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return s
	}
	return string(pretty)
}

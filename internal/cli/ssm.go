package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	awsutil "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version"
)

var ssmCommand = &cli.Command{
	Name:    "ssm",
	Aliases: []string{"ps", "param"},
	Usage:   "Interact with AWS Systems Manager Parameter Store",
	Subcommands: []*cli.Command{
		ssmShowCommand,
		ssmCatCommand,
		ssmLogCommand,
		ssmDiffCommand,
		ssmLsCommand,
		ssmSetCommand,
		ssmRmCommand,
	},
}

var ssmShowCommand = &cli.Command{
	Name:      "show",
	Usage:     "Show parameter value with metadata",
	ArgsUsage: "<name[@version][~shift]>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "decrypt",
			Aliases: []string{"d"},
			Value:   true,
			Usage:   "Decrypt SecureString values",
		},
	},
	Action: ssmShowAction,
}

var ssmCatCommand = &cli.Command{
	Name:      "cat",
	Usage:     "Output raw parameter value (for piping)",
	ArgsUsage: "<name[@version][~shift]>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "decrypt",
			Aliases: []string{"d"},
			Value:   true,
			Usage:   "Decrypt SecureString values",
		},
	},
	Action: ssmCatAction,
}

var ssmLogCommand = &cli.Command{
	Name:      "log",
	Usage:     "Show parameter version history",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "number",
			Aliases: []string{"n"},
			Value:   10,
			Usage:   "Number of versions to show",
		},
	},
	Action: ssmLogAction,
}

var ssmDiffCommand = &cli.Command{
	Name:      "diff",
	Usage:     "Show diff between two versions",
	ArgsUsage: "<name> <version1> <version2>",
	Action:    ssmDiffAction,
}

var ssmLsCommand = &cli.Command{
	Name:      "ls",
	Usage:     "List parameters",
	ArgsUsage: "[path-prefix]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "recursive",
			Aliases: []string{"r"},
			Usage:   "List parameters recursively",
		},
		&cli.IntFlag{
			Name:    "max",
			Aliases: []string{"m"},
			Value:   50,
			Usage:   "Maximum number of parameters to list",
		},
	},
	Action: ssmLsAction,
}

var ssmSetCommand = &cli.Command{
	Name:      "set",
	Usage:     "Set parameter value",
	ArgsUsage: "<name> <value>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			Value:   "String",
			Usage:   "Parameter type (String, StringList, SecureString)",
		},
		&cli.StringFlag{
			Name:  "description",
			Usage: "Parameter description",
		},
	},
	Action: ssmSetAction,
}

var ssmRmCommand = &cli.Command{
	Name:      "rm",
	Usage:     "Delete parameter",
	ArgsUsage: "<name>",
	Action:    ssmRmAction,
}

func ssmShowAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
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

	param, err := getSSMParameterWithVersion(ctx, clients.SSM, spec, c.Bool("decrypt"))
	if err != nil {
		return err
	}

	out := output.New(os.Stdout)
	out.Field("Name", aws.ToString(param.Name))
	out.Field("Version", fmt.Sprintf("%d", param.Version))
	out.Field("Type", string(param.Type))
	if param.LastModifiedDate != nil {
		out.Field("Modified", param.LastModifiedDate.Format(time.RFC3339))
	}
	out.Separator()
	out.Value(aws.ToString(param.Value))

	return nil
}

func ssmCatAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
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

	param, err := getSSMParameterWithVersion(ctx, clients.SSM, spec, c.Bool("decrypt"))
	if err != nil {
		return err
	}

	fmt.Print(aws.ToString(param.Value))
	return nil
}

func ssmLogAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	result, err := clients.SSM.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(name),
		MaxResults:     aws.Int32(maxResults),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter history: %w", err)
	}

	out := output.New(os.Stdout)
	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	for i, param := range result.Parameters {
		versionLabel := fmt.Sprintf("Version %d", param.Version)
		if i == 0 {
			versionLabel += " (current)"
		}
		fmt.Println(yellow(versionLabel))
		if param.LastModifiedDate != nil {
			fmt.Printf("%s %s\n", cyan("Date:"), param.LastModifiedDate.Format(time.RFC3339))
		}
		if param.LastModifiedUser != nil {
			fmt.Printf("%s %s\n", cyan("User:"), aws.ToString(param.LastModifiedUser))
		}
		out.ValuePreview(aws.ToString(param.Value), 100)
		if i < len(result.Parameters)-1 {
			fmt.Println()
		}
	}

	return nil
}

func ssmDiffAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve ssm diff <name> <version1> [version2]")
	}

	name := c.Args().Get(0)
	version1 := c.Args().Get(1)
	version2 := c.Args().Get(2)

	if version2 == "" {
		version2 = version1
		version1 = ""
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

	param1, err := getSSMParameterWithVersion(ctx, clients.SSM, spec1, true)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version1, err)
	}

	param2, err := getSSMParameterWithVersion(ctx, clients.SSM, spec2, true)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version2, err)
	}

	diff := output.Diff(
		fmt.Sprintf("%s@%d", name, param1.Version),
		fmt.Sprintf("%s@%d", name, param2.Version),
		aws.ToString(param1.Value),
		aws.ToString(param2.Value),
	)
	fmt.Print(diff)

	return nil
}

func ssmLsAction(c *cli.Context) error {
	prefix := "/"
	if c.NArg() > 0 {
		prefix = c.Args().First()
	}

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	input := &ssm.DescribeParametersInput{
		MaxResults: aws.Int32(int32(c.Int("max"))),
	}

	option := "OneLevel"
	if c.Bool("recursive") {
		option = "Recursive"
	}
	input.ParameterFilters = []types.ParameterStringFilter{
		{
			Key:    aws.String("Path"),
			Option: aws.String(option),
			Values: []string{prefix},
		},
	}

	result, err := clients.SSM.DescribeParameters(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to describe parameters: %w", err)
	}

	cyan := color.New(color.FgCyan).SprintFunc()
	for _, param := range result.Parameters {
		name := aws.ToString(param.Name)
		typeStr := string(param.Type)
		modified := ""
		if param.LastModifiedDate != nil {
			modified = param.LastModifiedDate.Format("2006-01-02 15:04")
		}
		fmt.Printf("%s  %s  %s\n", cyan(typeStr), modified, name)
	}

	return nil
}

func ssmSetAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve ssm set <name> <value>")
	}

	name := c.Args().Get(0)
	value := c.Args().Get(1)
	paramType := c.String("type")

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	input := &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      types.ParameterType(paramType),
		Overwrite: aws.Bool(true),
	}

	if desc := c.String("description"); desc != "" {
		input.Description = aws.String(desc)
	}

	result, err := clients.SSM.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s %s (version %d)\n", green("Set"), name, result.Version)

	return nil
}

func ssmRmAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()

	ctx := context.Background()
	clients, err := awsutil.NewClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %w", err)
	}

	_, err = clients.SSM.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	red := color.New(color.FgRed).SprintFunc()
	fmt.Printf("%s %s\n", red("Deleted"), name)

	return nil
}

// getSSMParameterWithVersion retrieves a parameter with version/shift support.
func getSSMParameterWithVersion(ctx context.Context, client *ssm.Client, spec *version.Spec, decrypt bool) (*types.ParameterHistory, error) {
	if spec.HasShift() {
		history, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
			Name:           aws.String(spec.Name),
			WithDecryption: aws.Bool(decrypt),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get parameter history: %w", err)
		}

		if len(history.Parameters) == 0 {
			return nil, fmt.Errorf("parameter not found: %s", spec.Name)
		}

		params := history.Parameters
		for i, j := 0, len(params)-1; i < j; i, j = i+1, j-1 {
			params[i], params[j] = params[j], params[i]
		}

		baseIdx := 0
		if spec.Version != nil {
			found := false
			for i, p := range params {
				if p.Version == *spec.Version {
					baseIdx = i
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("version %d not found", *spec.Version)
			}
		}

		targetIdx := baseIdx + spec.Shift
		if targetIdx >= len(params) {
			return nil, fmt.Errorf("version shift out of range: ~%d", spec.Shift)
		}

		return &params[targetIdx], nil
	}

	var nameWithVersion string
	if spec.Version != nil {
		nameWithVersion = fmt.Sprintf("%s:%d", spec.Name, *spec.Version)
	} else {
		nameWithVersion = spec.Name
	}

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(nameWithVersion),
		WithDecryption: aws.Bool(decrypt),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter: %w", err)
	}

	param := result.Parameter
	return &types.ParameterHistory{
		Name:             param.Name,
		Value:            param.Value,
		Type:             param.Type,
		Version:          param.Version,
		LastModifiedDate: param.LastModifiedDate,
	}, nil
}

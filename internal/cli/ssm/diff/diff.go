// Package diff provides the SSM diff command for comparing parameter versions.
//
// The diff command supports multiple argument formats:
//   - Fullspec: Both arguments include name and version (e.g., /param#1 /param#2)
//   - Fullspec single: One fullspec compared against latest (e.g., /param#3)
//   - Mixed: First arg with version, second is specifier only (e.g., /param#1 '#2')
//   - Legacy: Name followed by specifiers (e.g., /param '#1' '#2')
//
// When comparing identical versions, a warning and hint are displayed instead of empty diff.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the diff command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// ParsedSpec represents a parsed version specification for diff.
type ParsedSpec struct {
	Name    string
	Version *int64
	Shift   int
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> <version1> [version2]",
		Description: `Compare two versions of a parameter in unified diff format.
If only one version/spec is specified, compares against latest.

VERSION SPECIFIERS:
  #VERSION  Specific version (e.g., #3)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve ssm diff /app/config#1 /app/config#2   Compare v1 and v2 (fullspec)
  suve ssm diff /app/config#3                 Compare v3 with latest (fullspec)
  suve ssm diff /app/config#1 '#2'            Compare v1 and v2 (mixed)
  suve ssm diff /app/config '#1' '#2'         Compare v1 and v2 (legacy)
  suve ssm diff /app/config '~'               Compare previous with latest
  suve ssm diff -j /app/config#1 /app/config  JSON format before diffing`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	args := c.Args().Slice()
	spec1, spec2, err := ParseArgs(args)
	if err != nil {
		return err
	}

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return RunWithSpecs(c.Context, client, c.App.Writer, c.App.ErrWriter, spec1, spec2, c.Bool("json"))
}

// ParseArgs parses diff command arguments into two version specifications.
//
// Argument patterns supported:
//
//   - 1 arg (fullspec): "/param#3" → compare version 3 with latest
//   - 2 args (fullspec x2): "/param#1 /param#2" → compare v1 with v2
//   - 2 args (mixed): "/param#1 '#2'" → first with version, second specifier only
//   - 2 args (legacy omission): "/param '#3'" → compare v3 with latest
//   - 3 args (legacy): "/param '#1' '#2'" → compare v1 with v2
//
// The function detects the pattern by checking if the second argument starts with
// '#' or '~' (specifier-only format) or contains a full path (fullspec format).
//
// Returns two ParsedSpec pointers representing the versions to compare, or an error
// if the arguments cannot be parsed.
func ParseArgs(args []string) (*ParsedSpec, *ParsedSpec, error) {
	if len(args) == 0 || len(args) > 3 {
		return nil, nil, fmt.Errorf("usage: suve ssm diff <spec1> [spec2] | <name> <version1> [version2]")
	}

	switch len(args) {
	case 1:
		// 1 arg: fullspec vs latest
		return parseOneArg(args[0])
	case 2:
		// 2 args: check if second starts with # or ~
		return parseTwoArgs(args[0], args[1])
	case 3:
		// 3 args: legacy format (name, version1, version2)
		return parseThreeArgs(args[0], args[1], args[2])
	}

	return nil, nil, fmt.Errorf("usage: suve ssm diff <spec1> [spec2] | <name> <version1> [version2]")
}

func parseOneArg(arg string) (*ParsedSpec, *ParsedSpec, error) {
	spec, err := ssmversion.Parse(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version specification: %w", err)
	}

	spec1 := &ParsedSpec{
		Name:    spec.Name,
		Version: spec.Version,
		Shift:   spec.Shift,
	}
	spec2 := &ParsedSpec{
		Name:    spec.Name,
		Version: nil,
		Shift:   0,
	}
	return spec1, spec2, nil
}

func parseTwoArgs(arg1, arg2 string) (*ParsedSpec, *ParsedSpec, error) {
	// Parse first arg
	spec1Parsed, err := ssmversion.Parse(arg1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid first argument: %w", err)
	}

	// Check if second arg starts with # or ~ (version specifier only)
	if strings.HasPrefix(arg2, "#") || strings.HasPrefix(arg2, "~") {
		// Use name from first arg
		spec2Parsed, err := ssmversion.Parse(spec1Parsed.Name + arg2)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid second argument: %w", err)
		}

		// Check if first arg has version specifier (mixed pattern) or not (omission pattern)
		firstHasSpec := spec1Parsed.Version != nil || spec1Parsed.Shift > 0
		if firstHasSpec {
			// Mixed pattern: name#1 '#2' → v1 vs v2
			spec1 := &ParsedSpec{
				Name:    spec1Parsed.Name,
				Version: spec1Parsed.Version,
				Shift:   spec1Parsed.Shift,
			}
			spec2 := &ParsedSpec{
				Name:    spec1Parsed.Name,
				Version: spec2Parsed.Version,
				Shift:   spec2Parsed.Shift,
			}
			return spec1, spec2, nil
		}

		// Omission pattern: name '#3' → v3 vs latest (swap order)
		spec1 := &ParsedSpec{
			Name:    spec1Parsed.Name,
			Version: spec2Parsed.Version,
			Shift:   spec2Parsed.Shift,
		}
		spec2 := &ParsedSpec{
			Name:    spec1Parsed.Name,
			Version: nil,
			Shift:   0,
		}
		return spec1, spec2, nil
	}

	// Full path x2
	spec2Parsed, err := ssmversion.Parse(arg2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid second argument: %w", err)
	}

	spec1 := &ParsedSpec{
		Name:    spec1Parsed.Name,
		Version: spec1Parsed.Version,
		Shift:   spec1Parsed.Shift,
	}
	spec2 := &ParsedSpec{
		Name:    spec2Parsed.Name,
		Version: spec2Parsed.Version,
		Shift:   spec2Parsed.Shift,
	}
	return spec1, spec2, nil
}

func parseThreeArgs(name, version1, version2 string) (*ParsedSpec, *ParsedSpec, error) {
	spec1Parsed, err := ssmversion.Parse(name + version1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version1: %w", err)
	}

	spec2Parsed, err := ssmversion.Parse(name + version2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version2: %w", err)
	}

	spec1 := &ParsedSpec{
		Name:    spec1Parsed.Name,
		Version: spec1Parsed.Version,
		Shift:   spec1Parsed.Shift,
	}
	spec2 := &ParsedSpec{
		Name:    spec2Parsed.Name,
		Version: spec2Parsed.Version,
		Shift:   spec2Parsed.Shift,
	}
	return spec1, spec2, nil
}

// Run executes the diff command (legacy interface for backward compatibility).
func Run(ctx context.Context, client Client, w io.Writer, name, version1, version2 string) error {
	spec1, err := ssmversion.Parse(name + version1)
	if err != nil {
		return fmt.Errorf("invalid version1: %w", err)
	}

	spec2, err := ssmversion.Parse(name + version2)
	if err != nil {
		return fmt.Errorf("invalid version2: %w", err)
	}

	param1, err := ssmversion.GetParameterWithVersion(ctx, client, spec1, true)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version1, err)
	}

	param2, err := ssmversion.GetParameterWithVersion(ctx, client, spec2, true)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version2, err)
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%d", name, param1.Version),
		fmt.Sprintf("%s#%d", name, param2.Version),
		aws.ToString(param1.Value),
		aws.ToString(param2.Value),
	)
	_, _ = fmt.Fprint(w, diff)

	return nil
}

// RunWithSpecs executes the diff command with parsed specs.
func RunWithSpecs(ctx context.Context, client Client, w, errW io.Writer, spec1, spec2 *ParsedSpec, jsonFormat bool) error {
	ssmSpec1 := &ssmversion.Spec{
		Name:    spec1.Name,
		Version: spec1.Version,
		Shift:   spec1.Shift,
	}
	ssmSpec2 := &ssmversion.Spec{
		Name:    spec2.Name,
		Version: spec2.Version,
		Shift:   spec2.Shift,
	}

	param1, err := ssmversion.GetParameterWithVersion(ctx, client, ssmSpec1, true)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	param2, err := ssmversion.GetParameterWithVersion(ctx, client, ssmSpec2, true)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := aws.ToString(param1.Value)
	value2 := aws.ToString(param2.Value)

	// Format as JSON if enabled
	if jsonFormat {
		if !jsonutil.IsJSON(value1) || !jsonutil.IsJSON(value2) {
			output.Warning(errW, "--json has no effect: some values are not valid JSON")
		} else {
			value1 = jsonutil.Format(value1)
			value2 = jsonutil.Format(value2)
		}
	}

	if value1 == value2 {
		output.Warning(errW, "comparing identical versions")
		output.Hint(errW, "To compare with previous version, use: suve ssm diff %s~1", spec1.Name)
		return nil
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%d", spec1.Name, param1.Version),
		fmt.Sprintf("%s#%d", spec2.Name, param2.Version),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(w, diff)

	return nil
}

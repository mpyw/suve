// Package diff provides the SM diff command for comparing secret versions.
//
// The diff command supports multiple argument formats:
//   - Fullspec: Both arguments include name and version (e.g., secret:AWSPREVIOUS secret:AWSCURRENT)
//   - Fullspec single: One fullspec compared against AWSCURRENT (e.g., secret:AWSPREVIOUS)
//   - Mixed: First arg with version, second is specifier only (e.g., secret:AWSPREVIOUS ':AWSCURRENT')
//   - Legacy: Name followed by specifiers (e.g., secret ':AWSPREVIOUS' ':AWSCURRENT')
//
// When comparing identical versions, a warning and hints are displayed instead of empty diff.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the diff command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// ParsedSpec represents a parsed version specification for diff.
type ParsedSpec struct {
	Name  string
	ID    *string
	Label *string
	Shift int
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<spec1> [spec2] | <name> <version1> [version2]",
		Description: `Compare two versions of a secret in unified diff format.
If only one version/spec is specified, compares against AWSCURRENT.

VERSION SPECIFIERS:
  #VERSION  Specific version by VersionId
  :LABEL    Staging label (AWSCURRENT, AWSPREVIOUS)
  ~SHIFT    N versions ago; ~ alone means ~1

EXAMPLES:
  suve sm diff my-secret:AWSPREVIOUS my-secret:AWSCURRENT  Compare labels (fullspec)
  suve sm diff my-secret:AWSPREVIOUS                       Compare with current (fullspec)
  suve sm diff my-secret:AWSPREVIOUS ':AWSCURRENT'         Compare labels (mixed)
  suve sm diff my-secret ':AWSPREVIOUS' ':AWSCURRENT'      Compare labels (legacy)
  suve sm diff my-secret '~'                               Compare previous with current
  suve sm diff -j my-secret:AWSPREVIOUS                    JSON format before diffing`,
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

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return RunWithSpecs(c.Context, client, c.App.Writer, c.App.ErrWriter, spec1, spec2, c.Bool("json"))
}

// ParseArgs parses diff command arguments into two version specifications.
//
// Argument patterns supported:
//
//   - 1 arg (fullspec): "secret:AWSPREVIOUS" → compare AWSPREVIOUS with AWSCURRENT
//   - 2 args (fullspec x2): "secret:AWSPREVIOUS secret:AWSCURRENT" → compare two versions
//   - 2 args (mixed): "secret:AWSPREVIOUS ':AWSCURRENT'" → first with version, second specifier only
//   - 2 args (legacy omission): "secret ':AWSPREVIOUS'" → compare AWSPREVIOUS with AWSCURRENT
//   - 3 args (legacy): "secret ':AWSPREVIOUS' ':AWSCURRENT'" → compare two versions
//
// The function detects the pattern by checking if the second argument starts with
// '#', ':', or '~' (specifier-only format) or contains a full name (fullspec format).
//
// Returns two ParsedSpec pointers representing the versions to compare, or an error
// if the arguments cannot be parsed.
func ParseArgs(args []string) (*ParsedSpec, *ParsedSpec, error) {
	if len(args) == 0 || len(args) > 3 {
		return nil, nil, fmt.Errorf("usage: suve sm diff <spec1> [spec2] | <name> <version1> [version2]")
	}

	switch len(args) {
	case 1:
		// 1 arg: fullspec vs AWSCURRENT
		return parseOneArg(args[0])
	case 2:
		// 2 args: check if second starts with #, :, or ~
		return parseTwoArgs(args[0], args[1])
	case 3:
		// 3 args: legacy format (name, version1, version2)
		return parseThreeArgs(args[0], args[1], args[2])
	}

	return nil, nil, fmt.Errorf("usage: suve sm diff <spec1> [spec2] | <name> <version1> [version2]")
}

func parseOneArg(arg string) (*ParsedSpec, *ParsedSpec, error) {
	spec, err := smversion.Parse(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version specification: %w", err)
	}

	spec1 := &ParsedSpec{
		Name:  spec.Name,
		ID:    spec.ID,
		Label: spec.Label,
		Shift: spec.Shift,
	}
	spec2 := &ParsedSpec{
		Name:  spec.Name,
		ID:    nil,
		Label: nil,
		Shift: 0,
	}
	return spec1, spec2, nil
}

func parseTwoArgs(arg1, arg2 string) (*ParsedSpec, *ParsedSpec, error) {
	// Parse first arg
	spec1Parsed, err := smversion.Parse(arg1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid first argument: %w", err)
	}

	// Check if second arg starts with #, :, or ~ (version specifier only)
	if strings.HasPrefix(arg2, "#") || strings.HasPrefix(arg2, ":") || strings.HasPrefix(arg2, "~") {
		// Use name from first arg
		spec2Parsed, err := smversion.Parse(spec1Parsed.Name + arg2)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid second argument: %w", err)
		}

		// Check if first arg has version specifier (mixed pattern) or not (omission pattern)
		firstHasSpec := spec1Parsed.ID != nil || spec1Parsed.Label != nil || spec1Parsed.Shift > 0
		if firstHasSpec {
			// Mixed pattern: name:PREV ':CURR' → PREV vs CURR
			spec1 := &ParsedSpec{
				Name:  spec1Parsed.Name,
				ID:    spec1Parsed.ID,
				Label: spec1Parsed.Label,
				Shift: spec1Parsed.Shift,
			}
			spec2 := &ParsedSpec{
				Name:  spec1Parsed.Name,
				ID:    spec2Parsed.ID,
				Label: spec2Parsed.Label,
				Shift: spec2Parsed.Shift,
			}
			return spec1, spec2, nil
		}

		// Omission pattern: name ':PREV' → PREV vs AWSCURRENT (swap order)
		spec1 := &ParsedSpec{
			Name:  spec1Parsed.Name,
			ID:    spec2Parsed.ID,
			Label: spec2Parsed.Label,
			Shift: spec2Parsed.Shift,
		}
		spec2 := &ParsedSpec{
			Name:  spec1Parsed.Name,
			ID:    nil,
			Label: nil,
			Shift: 0,
		}
		return spec1, spec2, nil
	}

	// Full path x2
	spec2Parsed, err := smversion.Parse(arg2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid second argument: %w", err)
	}

	spec1 := &ParsedSpec{
		Name:  spec1Parsed.Name,
		ID:    spec1Parsed.ID,
		Label: spec1Parsed.Label,
		Shift: spec1Parsed.Shift,
	}
	spec2 := &ParsedSpec{
		Name:  spec2Parsed.Name,
		ID:    spec2Parsed.ID,
		Label: spec2Parsed.Label,
		Shift: spec2Parsed.Shift,
	}
	return spec1, spec2, nil
}

func parseThreeArgs(name, version1, version2 string) (*ParsedSpec, *ParsedSpec, error) {
	spec1Parsed, err := smversion.Parse(name + version1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version1: %w", err)
	}

	spec2Parsed, err := smversion.Parse(name + version2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version2: %w", err)
	}

	spec1 := &ParsedSpec{
		Name:  spec1Parsed.Name,
		ID:    spec1Parsed.ID,
		Label: spec1Parsed.Label,
		Shift: spec1Parsed.Shift,
	}
	spec2 := &ParsedSpec{
		Name:  spec2Parsed.Name,
		ID:    spec2Parsed.ID,
		Label: spec2Parsed.Label,
		Shift: spec2Parsed.Shift,
	}
	return spec1, spec2, nil
}

// Run executes the diff command (legacy interface for backward compatibility).
func Run(ctx context.Context, client Client, w io.Writer, name, version1, version2 string) error {
	spec1, err := smversion.Parse(name + version1)
	if err != nil {
		return fmt.Errorf("invalid version1: %w", err)
	}

	spec2, err := smversion.Parse(name + version2)
	if err != nil {
		return fmt.Errorf("invalid version2: %w", err)
	}

	secret1, err := smversion.GetSecretWithVersion(ctx, client, spec1)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version1, err)
	}

	secret2, err := smversion.GetSecretWithVersion(ctx, client, spec2)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version2, err)
	}

	v1 := aws.ToString(secret1.VersionId)
	if len(v1) > 8 {
		v1 = v1[:8]
	}
	v2 := aws.ToString(secret2.VersionId)
	if len(v2) > 8 {
		v2 = v2[:8]
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%s", name, v1),
		fmt.Sprintf("%s#%s", name, v2),
		aws.ToString(secret1.SecretString),
		aws.ToString(secret2.SecretString),
	)
	_, _ = fmt.Fprint(w, diff)

	return nil
}

// RunWithSpecs executes the diff command with parsed specs.
func RunWithSpecs(ctx context.Context, client Client, w, errW io.Writer, spec1, spec2 *ParsedSpec, jsonFormat bool) error {
	smSpec1 := &smversion.Spec{
		Name:  spec1.Name,
		ID:    spec1.ID,
		Label: spec1.Label,
		Shift: spec1.Shift,
	}
	smSpec2 := &smversion.Spec{
		Name:  spec2.Name,
		ID:    spec2.ID,
		Label: spec2.Label,
		Shift: spec2.Shift,
	}

	secret1, err := smversion.GetSecretWithVersion(ctx, client, smSpec1)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	secret2, err := smversion.GetSecretWithVersion(ctx, client, smSpec2)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := aws.ToString(secret1.SecretString)
	value2 := aws.ToString(secret2.SecretString)

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
		output.Hint(errW, "To compare with previous version, use: suve sm diff %s~1", spec1.Name)
		output.Hint(errW, "or: suve sm diff %s:AWSPREVIOUS", spec1.Name)
		return nil
	}

	v1 := aws.ToString(secret1.VersionId)
	if len(v1) > 8 {
		v1 = v1[:8]
	}
	v2 := aws.ToString(secret2.VersionId)
	if len(v2) > 8 {
		v2 = v2[:8]
	}

	diff := output.Diff(
		fmt.Sprintf("%s#%s", spec1.Name, v1),
		fmt.Sprintf("%s#%s", spec2.Name, v2),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(w, diff)

	return nil
}

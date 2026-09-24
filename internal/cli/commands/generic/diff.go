// diff.go provides the generic diff command shared by every provider.
//
// The scaffolding here owns the flow that is identical across providers: diff
// argument parsing (ParseDiffArgs), parse-json formatting, the identical-versions
// check, pager gating, and the JSON/text dispatch. The provider-specific parts —
// the version-spec grammar, the diff header labels, the JSON shape, and the
// identical-versions hints — live behind the DiffPresenter so each provider
// reproduces its own byte-identical output.

package generic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/version"
)

// DiffOptions holds the shared diff options.
type DiffOptions struct {
	ParseJSON bool
	NoPager   bool
	Output    output.Format
}

// DiffPresenter renders a diff for a specific provider. Implementations are stateful:
// Fetch loads both versions, and the subsequent methods render them.
type DiffPresenter interface {
	// Fetch resolves both specs and loads their entries via the provider usecase.
	Fetch(ctx context.Context) error
	// OldValue and NewValue return the two raw values to be compared.
	OldValue() string
	NewValue() string
	// Labels returns the unified-diff header labels for the old and new sides.
	Labels() (oldLabel, newLabel string)
	// RenderJSON writes the provider JSON output. diff is the pre-computed raw
	// unified diff (empty when the versions are identical).
	RenderJSON(stdout io.Writer, oldValue, newValue string, identical bool, diff string) error
	// Hints writes the provider-specific hints shown when versions are identical.
	Hints(stderr io.Writer)
}

// DiffRunner executes the diff command over a provider DiffPresenter.
type DiffRunner struct {
	Presenter DiffPresenter
	Options   DiffOptions
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run executes the diff command.
func (r *DiffRunner) Run(ctx context.Context) error {
	if err := r.Presenter.Fetch(ctx); err != nil {
		return err
	}

	rawValue1 := r.Presenter.OldValue()
	rawValue2 := r.Presenter.NewValue()

	// The identical decision is made on the RAW stored values, so a --parse-json
	// reformat (whitespace, key order, number spelling) never masks a real
	// stored difference (previously the compare ran AFTER formatting).
	identical := rawValue1 == rawValue2

	// Format as JSON if enabled — for rendering only.
	value1, value2 := rawValue1, rawValue2
	if r.Options.ParseJSON {
		value1, value2 = jsonutil.TryFormatOrWarn2(value1, value2, r.Stderr, "")
	}

	oldLabel, newLabel := r.Presenter.Labels()

	// JSON output mode
	if r.Options.Output == output.FormatJSON {
		diff := ""
		if !identical {
			diff = output.DiffRaw(oldLabel, newLabel, value1, value2)
		}

		return r.Presenter.RenderJSON(r.Stdout, value1, value2, identical, diff)
	}

	if identical {
		// The values are byte-identical. Distinguish self-comparison from two
		// distinct versions that merely happen to hold the same content, and
		// only offer the self-comparison hints in the former case.
		if oldLabel == newLabel {
			output.Warning(r.Stderr, "comparing identical versions")
			r.Presenter.Hints(r.Stderr)
		} else {
			output.Warning(r.Stderr, "versions differ but content is identical")
		}

		return nil
	}

	diff := output.Diff(r.Stdout, oldLabel, newLabel, value1, value2)

	// The raw values differ but --parse-json normalized them to the same form:
	// there is no textual diff to show, so say so explicitly instead of printing
	// nothing (and never claim the versions are identical).
	if diff == "" {
		output.Warning(r.Stderr, "values differ only in JSON formatting")

		return nil
	}

	output.Print(r.Stdout, diff)

	return nil
}

// DiffConfig holds the provider-specific configuration for the diff command.
type DiffConfig[S any] struct {
	// Usage is the one-line command usage string.
	Usage string
	// ArgsUsage is the positional-arguments usage string.
	ArgsUsage string
	// Description is the long help text.
	Description string
	// ParseDiffArgs parses the raw positional args into two version specs.
	ParseDiffArgs func(args []string) (S, S, error)
	// NewPresenter builds the provider DiffPresenter bound to the two specs.
	NewPresenter func(ctx context.Context, spec1, spec2 S) (DiffPresenter, error)
}

// DiffCommand returns the generic diff command wired with the provider DiffConfig.
func DiffCommand[S any](cfg DiffConfig[S]) *cli.Command {
	return &cli.Command{
		Name:        "diff",
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			spec1, spec2, err := cfg.ParseDiffArgs(cmd.Args().Slice())
			if err != nil {
				return err
			}

			presenter, err := cfg.NewPresenter(ctx, spec1, spec2)
			if err != nil {
				return err
			}

			outputFormat, err := output.ParseFormat(cmd.String("output"))
			if err != nil {
				return err
			}

			opts := DiffOptions{
				ParseJSON: cmd.Bool("parse-json"),
				NoPager:   cmd.Bool("no-pager"),
				Output:    outputFormat,
			}

			// JSON output disables pager
			noPager := opts.NoPager || opts.Output == output.FormatJSON

			return internal.WithPager(cmd, noPager, func(stdout, stderr io.Writer) error {
				r := &DiffRunner{Presenter: presenter, Options: opts, Stdout: stdout, Stderr: stderr}

				return r.Run(ctx)
			})
		},
	}
}

// ParseDiffArgs parses diff command arguments into two version specifications.
// Every versioned diff command (AWS, Google Cloud, Azure Key Vault) wraps it
// with its grammar from internal/version and its usage string.
//
// The diff command compares two versions of one parameter or secret. The
// examples below use AWS specs (Parameter Store "#N", Secrets Manager labels).
// ParseDiffArgs handles the argument patterns every provider supports.
//
// # Argument Patterns
//
// The diff command supports three formats for specifying versions to compare:
//
// ## Full Spec Format
//
// Each argument is a complete specification including name and version.
//
//   - 1 arg: Compare specified version against default (latest/AWSCURRENT)
//     suve aws param diff /app/config#3
//     suve aws secret diff my-secret:AWSPREVIOUS
//
//   - 2 args: Compare two fully-specified versions
//     suve aws param diff /app/config#1 /app/config#2
//     suve aws secret diff my-secret:AWSPREVIOUS my-secret:AWSCURRENT
//
// ## Partial Spec Format
//
// Name is specified separately from version specifiers.
//
//   - 2 args: Name + specifier → compare with default
//     suve aws param diff /app/config '#3'
//     suve aws secret diff my-secret ':AWSPREVIOUS'
//
//   - 3 args: Name + two specifiers
//     suve aws param diff /app/config '#1' '#2'
//     suve aws secret diff my-secret ':AWSPREVIOUS' ':AWSCURRENT'
//
// ## Mixed Format
//
// First argument is full spec, second is specifier-only (inherits name from first).
//
//   - 2 args: Full spec + specifier
//     suve aws param diff /app/config#1 '#2'
//     suve aws secret diff my-secret:AWSPREVIOUS ':AWSCURRENT'
//
// # Return Value Semantics
//
// ParseDiffArgs always returns (spec1, spec2) where the comparison is performed as:
//
//	diff(spec1, spec2) = "what changed from spec1 to spec2"
//
// This means spec1 is the "old" version and spec2 is the "new" version.
// The diff output will show:
//   - Lines removed from spec1 as "-" (red)
//   - Lines added in spec2 as "+" (green)
//
// This function is generic over the absolute specifier type A, which differs
// between SSM Parameter Store (AbsoluteSpec with Version *int64) and Secrets Manager (AbsoluteSpec with
// ID *string and Label *string).
//
// # Parameters
//
//   - args: Command line arguments (1-3 arguments supported)
//   - parse: Service-specific parser function (e.g., version.ParameterStore.Parse, version.SecretsManager.Parse)
//   - hasAbsolute: Returns true if the absolute specifier is set (non-zero).
//     Used to distinguish "mixed" pattern from "partial spec" pattern in 2-arg case.
//     For SSM Parameter Store: func(abs) bool { return abs.Version != nil }
//     For Secrets Manager: func(abs) bool { return abs.ID != nil || abs.Label != nil }
//   - prefixes: Characters that start a specifier (e.g., "#~" for SSM Parameter Store, "#:~" for Secrets Manager).
//     Used to detect if second argument is specifier-only.
//   - usage: Error message to show when argument count is invalid.
//
// # Return Values
//
// Returns (spec1, spec2, nil) on success, where:
//   - spec1: The "from" version (shown with "-" in diff)
//   - spec2: The "to" version (shown with "+" in diff)
//
// Returns (nil, nil, error) on parse failure or invalid argument count.
//
// # Examples
//
// SSM Parameter Store usage:
//
//	spec1, spec2, err := ParseDiffArgs(
//	    args,
//	    version.ParameterStore.Parse,
//	    version.NumericAbsolute.IsSet,
//	    "#~",
//	    "usage: suve aws param diff <spec1> [spec2] | <name> #<version1> [#<version2>]",
//	)
//
// Secrets Manager usage:
//
//	spec1, spec2, err := ParseDiffArgs(
//	    args,
//	    version.SecretsManager.Parse,
//	    version.OpaqueAbsolute.IsSet,
//	    "#:~",
//	    "usage: suve aws secret diff <spec1> [spec2] | <name> #<version1> [#<version2>]",
//	)
func ParseDiffArgs[A any](
	args []string,
	parse func(string) (*version.Spec[A], error),
	hasAbsolute func(A) bool,
	prefixes string,
	usage string,
) (*version.Spec[A], *version.Spec[A], error) {
	if len(args) == 0 || len(args) > 3 {
		return nil, nil, errors.New(usage)
	}

	switch len(args) {
	case 1:
		return parseDiffOneArg(args[0], parse)
	case 2: //nolint:mnd // two-arg case for version comparison
		return parseDiffTwoArgs(args[0], args[1], parse, hasAbsolute, prefixes)
	default: // case 3
		return parseDiffThreeArgs(args[0], args[1], args[2], parse, prefixes)
	}
}

// parseDiffOneArg handles full spec format with single argument.
//
// Pattern: "name#v" or "name~N" or "name:LABEL"
//
// The single argument specifies the "from" version, and it will be compared
// against the default version (latest for SSM Parameter Store, AWSCURRENT for Secrets Manager).
//
// Examples:
//
//	"/app/config#3"         → spec1=#3, spec2=latest
//	"my-secret:AWSPREVIOUS" → spec1=AWSPREVIOUS, spec2=AWSCURRENT
//	"/app/config~1"         → spec1=~1 (previous), spec2=latest
//
// Note: If the argument has no specifier (e.g., just "/app/config"),
// both spec1 and spec2 will be default, resulting in "identical versions" warning.
func parseDiffOneArg[A any](
	arg string,
	parse func(string) (*version.Spec[A], error),
) (*version.Spec[A], *version.Spec[A], error) {
	spec, err := parse(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version specification: %w", err)
	}

	// spec2 is the default version (zero absolute specifier, no shift).
	// For SSM Parameter Store: latest version
	// For Secrets Manager: AWSCURRENT label
	var zero A

	spec2 := &version.Spec[A]{Name: spec.Name, Absolute: zero, Shift: 0}

	return spec, spec2, nil
}

// parseDiffTwoArgs handles two argument patterns with automatic format detection.
//
// This function detects three formats based on the second argument:
//
// # Format A: Full Spec x2 (both args are complete specifications)
//
// Detected when: second argument does NOT start with a prefix character.
//
//	"/app/config#1" "/app/config#2" → spec1=#1, spec2=#2
//	"secret-a:PREV" "secret-b:CURR" → spec1=secret-a:PREV, spec2=secret-b:CURR
//
// # Format B: Mixed (first has specifier, second is specifier-only)
//
// Detected when: second argument starts with prefix AND first argument has a specifier.
//
//	"/app/config#1" "#2"     → spec1=#1, spec2=#2
//	"my-secret:PREV" ":CURR" → spec1=PREV, spec2=CURR
//
// # Format C: Partial Spec (first is name-only, second is specifier-only)
//
// Detected when: second argument starts with prefix AND first argument has NO specifier.
// In this case, the order is swapped: spec1 gets the specifier, spec2 gets default.
//
//	"/app/config" "#3"  → spec1=#3, spec2=latest (NOT spec1=latest, spec2=#3)
//	"my-secret" ":PREV" → spec1=PREV, spec2=AWSCURRENT
//
// The partial spec swap ensures consistent semantics: you're always comparing
// "the specified version" against "the default version", regardless of argument order.
func parseDiffTwoArgs[A any](
	arg1, arg2 string,
	parse func(string) (*version.Spec[A], error),
	hasAbsolute func(A) bool,
	prefixes string,
) (*version.Spec[A], *version.Spec[A], error) {
	// Parse the first argument (always a complete specification)
	spec1, err := parse(arg1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid first argument: %w", err)
	}

	// Check if second argument is specifier-only (starts with #, :, or ~)
	// Examples: "#3", ":AWSPREVIOUS", "~1"
	if len(arg2) > 0 && strings.ContainsRune(prefixes, rune(arg2[0])) {
		// Specifier-only: prepend the name from first argument
		// "/app/config" + "#3" → "/app/config#3"
		spec2, err := parse(spec1.Name + arg2)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid second argument: %w", err)
		}

		// Detect format: mixed vs partial spec
		// Mixed: first arg has a specifier (version, label, or shift)
		// Partial Spec: first arg is name-only (no specifier)
		firstHasSpec := hasAbsolute(spec1.Absolute) || spec1.Shift > 0
		if firstHasSpec {
			// Mixed format: both have specifiers
			// "/app/config#1" "#2" → compare #1 with #2
			return spec1, spec2, nil
		}

		// Partial spec format: first arg has no specifier, swap the order
		// "/app/config" "#3" → compare #3 with latest
		// This makes the specified version the "from" and default the "to"
		var zero A

		return spec2, &version.Spec[A]{Name: spec1.Name, Absolute: zero, Shift: 0}, nil
	}

	// Full spec x2: second argument is a complete specification
	// "/app/config#1" "/app/config#2" → compare #1 with #2
	spec2, err := parse(arg2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid second argument: %w", err)
	}

	return spec1, spec2, nil
}

// parseDiffThreeArgs handles partial spec format with separate name and specifiers.
//
// Pattern: "name" "specifier1" "specifier2"
//
// The name is specified once, and both specifiers use that same name.
// This format is useful when you want to compare two versions of the same
// parameter/secret without repeating the name.
//
// Examples:
//
//	"/app/config" "#1" "#2"                      → compare /app/config#1 with /app/config#2
//	"my-secret" ":AWSPREVIOUS" ":AWSCURRENT"     → compare AWSPREVIOUS with AWSCURRENT
//	"/app/config" "~2" "~1"                      → compare 2-versions-ago with 1-version-ago
//
// Each version argument MUST start with a specifier prefix (#, :, or ~ depending
// on the service). A bare token such as "3" is rejected with a hard error rather
// than silently concatenated onto the name: allowing it would turn
// `diff /app/config 3 1` into a comparison of parameters *named* "/app/config3"
// and "/app/config1", targeting the wrong resources. Requiring the prefix keeps
// this format aligned with the usage text.
func parseDiffThreeArgs[A any](
	name, version1, version2 string,
	parse func(string) (*version.Spec[A], error),
	prefixes string,
) (*version.Spec[A], *version.Spec[A], error) {
	// Both version arguments must be specifier-only (start with #, :, or ~).
	// Otherwise "name" + "3" would build the resource name "name3" instead of
	// selecting version 3 of "name".
	if err := requireDiffSpecifier("version1", version1, prefixes); err != nil {
		return nil, nil, err
	}

	if err := requireDiffSpecifier("version2", version2, prefixes); err != nil {
		return nil, nil, err
	}

	// Parse name + first specifier
	spec1, err := parse(name + version1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version1: %w", err)
	}

	// Parse name + second specifier
	spec2, err := parse(name + version2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version2: %w", err)
	}

	return spec1, spec2, nil
}

// requireDiffSpecifier verifies that a version argument in the 3-arg form starts
// with one of the service's specifier prefix characters. A bare token (e.g.
// "3") is rejected with a message that names the valid prefixes and suggests
// the prefixed form.
func requireDiffSpecifier(label, arg, prefixes string) error {
	if len(arg) > 0 && strings.ContainsRune(prefixes, rune(arg[0])) {
		return nil
	}

	return fmt.Errorf(
		"%s %q must start with a version specifier (%s); did you mean %q?",
		label, arg, formatDiffPrefixes(prefixes), string(prefixes[0])+arg,
	)
}

// formatDiffPrefixes renders the specifier prefix characters as a human-readable
// comma-separated list, e.g. "#~" → "#, ~".
func formatDiffPrefixes(prefixes string) string {
	parts := lo.Map([]rune(prefixes), func(r rune, _ int) string {
		return string(r)
	})

	return strings.Join(parts, ", ")
}

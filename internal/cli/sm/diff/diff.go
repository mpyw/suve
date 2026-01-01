// Package diff provides the SM diff command for comparing secret versions.
//
// The diff command supports multiple argument formats:
//   - Full Spec: Both arguments include name and version (e.g., secret:AWSPREVIOUS secret:AWSCURRENT)
//   - Full Spec single: One full spec compared against AWSCURRENT (e.g., secret:AWSPREVIOUS)
//   - Mixed: First arg with version, second is specifier only (e.g., secret:AWSPREVIOUS ':AWSCURRENT')
//   - Partial Spec: Name followed by specifiers (e.g., secret ':AWSPREVIOUS' ':AWSCURRENT')
//
// When comparing identical versions, a warning and hints are displayed instead of empty diff.
package diff

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/smutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the diff command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the diff command.
type Runner struct {
	Client Client
	Store  *stage.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the diff command.
type Options struct {
	Spec1      *smversion.Spec
	Spec2      *smversion.Spec
	JSONFormat bool
	NoPager    bool
	Staged     bool   // Compare staged value vs AWS current
	StagedName string // Secret name for staged diff (empty = all staged)
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
  suve sm diff my-secret:AWSPREVIOUS my-secret:AWSCURRENT  Compare labels (full spec)
  suve sm diff my-secret:AWSPREVIOUS                       Compare with current (full spec)
  suve sm diff my-secret:AWSPREVIOUS ':AWSCURRENT'         Compare labels (mixed)
  suve sm diff my-secret ':AWSPREVIOUS' ':AWSCURRENT'      Compare labels (partial spec)
  suve sm diff my-secret '~'                               Compare previous with current
  suve sm diff -j my-secret:AWSPREVIOUS                    JSON format before diffing
  suve sm diff --staged                                    Compare all staged vs AWS current
  suve sm diff --staged my-secret                          Compare specific staged vs AWS current`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.BoolFlag{
				Name:  "staged",
				Usage: "Compare staged value vs AWS current value",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	staged := cmd.Bool("staged")

	// Handle --staged mode
	if staged {
		var stagedName string

		if cmd.Args().Len() > 1 {
			return fmt.Errorf("usage: suve sm diff --staged [name]")
		}

		if cmd.Args().Len() == 1 {
			// Parse and validate the name (no version specifier allowed)
			spec, err := smversion.Parse(cmd.Args().First())
			if err != nil {
				return err
			}
			if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
				return fmt.Errorf("--staged requires a secret name without version specifier")
			}
			stagedName = spec.Name
		}

		store, err := stage.NewStore()
		if err != nil {
			return fmt.Errorf("failed to initialize stage store: %w", err)
		}

		client, err := awsutil.NewSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize AWS client: %w", err)
		}

		opts := Options{
			JSONFormat: cmd.Bool("json"),
			NoPager:    cmd.Bool("no-pager"),
			Staged:     true,
			StagedName: stagedName,
		}

		return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
			r := &Runner{
				Client: client,
				Store:  store,
				Stdout: w,
				Stderr: cmd.Root().ErrWriter,
			}
			return r.Run(ctx, opts)
		})
	}

	// Normal diff mode
	spec1, spec2, err := smversion.ParseDiffArgs(cmd.Args().Slice())
	if err != nil {
		return err
	}

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := Options{
		Spec1:      spec1,
		Spec2:      spec2,
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r := &Runner{
			Client: client,
			Stdout: w,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	if opts.Staged {
		return r.runStaged(ctx, opts)
	}
	return r.runNormal(ctx, opts)
}

func (r *Runner) runStaged(ctx context.Context, opts Options) error {
	// Get all staged entries for SM
	allEntries, err := r.Store.List(stage.ServiceSM)
	if err != nil {
		return err
	}
	entries := allEntries[stage.ServiceSM]

	// Filter by name if specified
	if opts.StagedName != "" {
		entry, err := r.Store.Get(stage.ServiceSM, opts.StagedName)
		if err == stage.ErrNotStaged {
			output.Warning(r.Stderr, "%s is not staged", opts.StagedName)
			return nil
		}
		if err != nil {
			return err
		}
		entries = map[string]stage.Entry{opts.StagedName: *entry}
	}

	if len(entries) == 0 {
		output.Warning(r.Stderr, "nothing staged")
		return nil
	}

	// Sort keys for consistent output
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	first := true
	for _, name := range names {
		entry := entries[name]

		if !first {
			_, _ = fmt.Fprintln(r.Stdout)
		}
		first = false

		if err := r.diffSingleStaged(ctx, opts, name, entry); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) diffSingleStaged(ctx context.Context, opts Options, name string, entry stage.Entry) error {
	// Get current AWS value
	spec := &smversion.Spec{Name: name}
	secret, err := smversion.GetSecretWithVersion(ctx, r.Client, spec)
	if err != nil {
		return fmt.Errorf("failed to get current version for %s: %w", name, err)
	}

	awsValue := lo.FromPtr(secret.SecretString)
	stagedValue := entry.Value

	// For delete operation, staged value is empty
	if entry.Operation == stage.OperationDelete {
		stagedValue = ""
	}

	// Format as JSON if enabled
	if opts.JSONFormat {
		formatted1, ok1 := jsonutil.TryFormat(awsValue)
		formatted2, ok2 := jsonutil.TryFormat(stagedValue)
		if ok1 && ok2 {
			awsValue = formatted1
			stagedValue = formatted2
		} else if ok1 || ok2 {
			output.Warning(r.Stderr, "--json has no effect for %s: some values are not valid JSON", name)
		}
	}

	if awsValue == stagedValue {
		output.Warning(r.Stderr, "staged value is identical to AWS current for %s", name)
		return nil
	}

	versionID := smutil.TruncateVersionID(lo.FromPtr(secret.VersionId))
	label1 := fmt.Sprintf("%s#%s (AWS)", name, versionID)
	label2 := fmt.Sprintf("%s (staged)", name)
	if entry.Operation == stage.OperationDelete {
		label2 = fmt.Sprintf("%s (staged for deletion)", name)
	}

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}

func (r *Runner) runNormal(ctx context.Context, opts Options) error {
	secret1, err := smversion.GetSecretWithVersion(ctx, r.Client, opts.Spec1)
	if err != nil {
		return fmt.Errorf("failed to get first version: %w", err)
	}

	secret2, err := smversion.GetSecretWithVersion(ctx, r.Client, opts.Spec2)
	if err != nil {
		return fmt.Errorf("failed to get second version: %w", err)
	}

	value1 := lo.FromPtr(secret1.SecretString)
	value2 := lo.FromPtr(secret2.SecretString)

	// Format as JSON if enabled
	if opts.JSONFormat {
		formatted1, ok1 := jsonutil.TryFormat(value1)
		formatted2, ok2 := jsonutil.TryFormat(value2)
		if ok1 && ok2 {
			value1 = formatted1
			value2 = formatted2
		} else {
			output.Warning(r.Stderr, "--json has no effect: some values are not valid JSON")
		}
	}

	if value1 == value2 {
		output.Warning(r.Stderr, "comparing identical versions")
		output.Hint(r.Stderr, "To compare with previous version, use: suve sm diff %s~1", opts.Spec1.Name)
		output.Hint(r.Stderr, "or: suve sm diff %s:AWSPREVIOUS", opts.Spec1.Name)
		return nil
	}

	v1 := smutil.TruncateVersionID(lo.FromPtr(secret1.VersionId))
	v2 := smutil.TruncateVersionID(lo.FromPtr(secret2.VersionId))

	diff := output.Diff(
		fmt.Sprintf("%s#%s", opts.Spec1.Name, v1),
		fmt.Sprintf("%s#%s", opts.Spec2.Name, v2),
		value1,
		value2,
	)
	_, _ = fmt.Fprint(r.Stdout, diff)

	return nil
}

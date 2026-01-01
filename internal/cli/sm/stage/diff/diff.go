// Package diff provides the SM stage diff command for comparing staged vs AWS values.
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
	Name       string // Secret name (empty = all staged)
	JSONFormat bool
	NoPager    bool
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between staged and AWS values",
		ArgsUsage: "[name]",
		Description: `Compare staged values against AWS current values.

If a secret name is specified, shows diff for that secret only.
Otherwise, shows diff for all staged SM secrets.

EXAMPLES:
   suve sm stage diff            Show diff for all staged SM secrets
   suve sm stage diff my-secret  Show diff for specific secret
   suve sm stage diff -j         Show diff with JSON formatting`,
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
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	var name string
	if cmd.Args().Len() > 1 {
		return fmt.Errorf("usage: suve sm stage diff [name]")
	}
	if cmd.Args().Len() == 1 {
		// Parse and validate the name (no version specifier allowed)
		spec, err := smversion.Parse(cmd.Args().First())
		if err != nil {
			return err
		}
		if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
			return fmt.Errorf("stage diff requires a secret name without version specifier")
		}
		name = spec.Name
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
		Name:       name,
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
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

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	// Get all staged entries for SM
	allEntries, err := r.Store.List(stage.ServiceSM)
	if err != nil {
		return err
	}
	entries := allEntries[stage.ServiceSM]

	// Filter by name if specified
	if opts.Name != "" {
		entry, err := r.Store.Get(stage.ServiceSM, opts.Name)
		if err == stage.ErrNotStaged {
			output.Warning(r.Stderr, "%s is not staged", opts.Name)
			return nil
		}
		if err != nil {
			return err
		}
		entries = map[string]stage.Entry{opts.Name: *entry}
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

		if err := r.diffSingle(ctx, opts, name, entry); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) diffSingle(ctx context.Context, opts Options, name string, entry stage.Entry) error {
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

// Package edit provides the SM edit command for staging secret changes.
package edit

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/editor"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Client is the interface for the edit command.
type Client interface {
	smapi.GetSecretValueAPI
	smapi.ListSecretVersionIdsAPI
}

// Runner executes the edit command.
type Runner struct {
	Client     Client
	Store      *stage.Store
	Stdout     io.Writer
	Stderr     io.Writer
	OpenEditor editor.OpenFunc // Optional: defaults to editor.Open if nil
}

// Options holds the options for the edit command.
type Options struct {
	Name string
}

// Command returns the edit command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     "Edit secret value and stage changes",
		ArgsUsage: "<name>",
		Description: `Open an editor to modify a secret value, then stage the change.

If the secret is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately push to AWS).

Use 'suve sm stage delete' to stage a secret for deletion.
Use 'suve sm stage push' to apply staged changes to AWS.
Use 'suve sm stage status' to view staged changes.

EXAMPLES:
   suve sm stage edit my-secret  Edit and stage secret`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm stage edit <name>")
	}

	name := cmd.Args().First()

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{Name: name})
}

// Run executes the edit command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	// Check if already staged
	stagedEntry, err := r.Store.Get(stage.ServiceSM, opts.Name)
	if err != nil && err != stage.ErrNotStaged {
		return err
	}

	var currentValue string
	if stagedEntry != nil && stagedEntry.Operation == stage.OperationSet {
		// Use staged value
		currentValue = stagedEntry.Value
	} else {
		// Fetch from AWS
		spec, err := smversion.Parse(opts.Name)
		if err != nil {
			return err
		}

		secret, err := smversion.GetSecretWithVersion(ctx, r.Client, spec)
		if err != nil {
			return err
		}
		currentValue = lo.FromPtr(secret.SecretString)
	}

	// Open editor
	editorFn := r.OpenEditor
	if editorFn == nil {
		editorFn = editor.Open
	}
	newValue, err := editorFn(currentValue)
	if err != nil {
		return fmt.Errorf("failed to edit: %w", err)
	}

	// Check if changed
	if newValue == currentValue {
		yellow := color.New(color.FgYellow).SprintFunc()
		_, _ = fmt.Fprintln(r.Stdout, yellow("No changes made."))
		return nil
	}

	// Stage the change
	if err := r.Store.Stage(stage.ServiceSM, opts.Name, stage.Entry{
		Operation: stage.OperationSet,
		Value:     newValue,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Staged: %s\n", green("✓"), opts.Name)
	return nil
}

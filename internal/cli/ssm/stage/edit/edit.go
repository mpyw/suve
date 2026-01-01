// Package edit provides the SSM edit command for staging parameter changes.
package edit

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Client is the interface for the edit command.
type Client interface {
	ssmapi.GetParameterAPI
	ssmapi.GetParameterHistoryAPI
}

// EditorFunc is a function that opens an editor with content and returns the edited result.
type EditorFunc func(content string) (string, error)

// Runner executes the edit command.
type Runner struct {
	Client     Client
	Store      *stage.Store
	Stdout     io.Writer
	Stderr     io.Writer
	OpenEditor EditorFunc // Optional: defaults to openEditor if nil
}

// Options holds the options for the edit command.
type Options struct {
	Name string
}

// Command returns the edit command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     "Edit parameter value and stage changes",
		ArgsUsage: "<name>",
		Description: `Open an editor to modify a parameter value, then stage the change.

If the parameter is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately push to AWS).

Use 'suve ssm stage delete' to stage a parameter for deletion.
Use 'suve ssm stage push' to apply staged changes to AWS.
Use 'suve ssm stage status' to view staged changes.

EXAMPLES:
   suve ssm stage edit /app/config/db-url  Edit and stage parameter`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm stage edit <name>")
	}

	name := cmd.Args().First()

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSSMClient(ctx)
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
	stagedEntry, err := r.Store.Get(stage.ServiceSSM, opts.Name)
	if err != nil && err != stage.ErrNotStaged {
		return err
	}

	var currentValue string
	if stagedEntry != nil && stagedEntry.Operation == stage.OperationSet {
		// Use staged value
		currentValue = stagedEntry.Value
	} else {
		// Fetch from AWS
		spec, err := ssmversion.Parse(opts.Name)
		if err != nil {
			return err
		}

		param, err := ssmversion.GetParameterWithVersion(ctx, r.Client, spec, true)
		if err != nil {
			return err
		}
		currentValue = lo.FromPtr(param.Value)
	}

	// Open editor
	editorFn := r.OpenEditor
	if editorFn == nil {
		editorFn = openEditor
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
	if err := r.Store.Stage(stage.ServiceSSM, opts.Name, stage.Entry{
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

func openEditor(content string) (string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}

	tmpFile, err := os.CreateTemp("", "suve-edit-*.txt")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	cmd := exec.Command(editor, tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return "", err
	}

	// Trim trailing newline that editors often add
	result := string(data)
	result = strings.TrimSuffix(result, "\n")

	return result, nil
}

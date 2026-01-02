// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/staging"
)

// VersionFetcher fetches values for specific versions from AWS.
type VersionFetcher interface {
	// FetchVersion fetches the value for a specific version.
	FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error)
}

// ResetRunner executes reset operations using a strategy.
type ResetRunner struct {
	Parser  staging.Parser
	Fetcher VersionFetcher // Optional: required only for version restore operations
	Store   *staging.Store
	Stdout  io.Writer
	Stderr  io.Writer
}

// ResetOptions holds options for the reset command.
type ResetOptions struct {
	Spec string // Name with optional version spec
	All  bool   // Reset all staged items for this service
}

// Run executes the reset command.
func (r *ResetRunner) Run(ctx context.Context, opts ResetOptions) error {
	if opts.All {
		return r.runUnstageAll()
	}

	name, hasVersion, err := r.Parser.ParseSpec(opts.Spec)
	if err != nil {
		return err
	}

	// If version specified, restore to that version
	if hasVersion {
		return r.runRestore(ctx, opts.Spec, name)
	}

	// Otherwise, just unstage
	return r.runUnstage(name)
}

func (r *ResetRunner) runUnstageAll() error {
	service := r.Parser.Service()
	serviceName := r.Parser.ServiceName()

	// Check if there are any staged changes
	staged, err := r.Store.List(service)
	if err != nil {
		return err
	}

	serviceStaged := staged[service]
	if len(serviceStaged) == 0 {
		_, _ = fmt.Fprintf(r.Stdout, "%s\n", colors.Warning(fmt.Sprintf("No %s changes staged.", serviceName)))
		return nil
	}

	if err := r.Store.UnstageAll(service); err != nil {
		return err
	}

	itemName := r.Parser.ItemName()
	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all %s %ss (%d)\n", colors.Success("✓"), serviceName, itemName, len(serviceStaged))
	return nil
}

func (r *ResetRunner) runUnstage(name string) error {
	service := r.Parser.Service()

	// Check if actually staged
	_, err := r.Store.Get(service, name)
	if errors.Is(err, staging.ErrNotStaged) {
		_, _ = fmt.Fprintf(r.Stdout, "%s %s is not staged\n", colors.Warning("!"), name)
		return nil
	}
	if err != nil {
		return err
	}

	if err := r.Store.Unstage(service, name); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged %s\n", colors.Success("✓"), name)
	return nil
}

func (r *ResetRunner) runRestore(ctx context.Context, spec, name string) error {
	service := r.Parser.Service()

	// Fetch the specific version (requires Fetcher)
	if r.Fetcher == nil {
		return fmt.Errorf("version fetcher required for restore operation")
	}
	value, versionLabel, err := r.Fetcher.FetchVersion(ctx, spec)
	if err != nil {
		return err
	}

	// Stage this value
	if err := r.Store.Stage(service, name, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     value,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Restored %s (staged from version %s)\n",
		colors.Success("✓"), name, versionLabel)
	return nil
}

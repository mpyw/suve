package staging

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// ApplyInput holds input for the apply use case.
type ApplyInput struct {
	Name            string // Optional: apply only this item
	IgnoreConflicts bool   // Skip conflict detection
}

// ApplyResultStatus represents the status of an apply operation.
type ApplyResultStatus int

const (
	ApplyResultCreated ApplyResultStatus = iota
	ApplyResultUpdated
	ApplyResultDeleted
	ApplyResultFailed
)

// ApplyResultEntry represents the result of applying a single entry.
type ApplyResultEntry struct {
	Name   string
	Status ApplyResultStatus
	Error  error
}

// ApplyOutput holds the result of the apply use case.
type ApplyOutput struct {
	ServiceName string
	ItemName    string
	Results     []ApplyResultEntry
	Conflicts   []string
	Succeeded   int
	Failed      int
}

// ApplyUseCase executes apply operations.
type ApplyUseCase struct {
	Strategy staging.ApplyStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the apply use case.
func (u *ApplyUseCase) Execute(ctx context.Context, input ApplyInput) (*ApplyOutput, error) {
	service := u.Strategy.Service()
	serviceName := u.Strategy.ServiceName()
	itemName := u.Strategy.ItemName()

	output := &ApplyOutput{
		ServiceName: serviceName,
		ItemName:    itemName,
	}

	staged, err := u.Store.List(service)
	if err != nil {
		return nil, err
	}

	entries := staged[service]

	// Filter by name if specified
	if input.Name != "" {
		entry, exists := entries[input.Name]
		if !exists {
			return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
		}
		entries = map[string]staging.Entry{input.Name: entry}
	}

	if len(entries) == 0 {
		return output, nil
	}

	// Check for conflicts
	if !input.IgnoreConflicts {
		conflicts := staging.CheckConflicts(ctx, u.Strategy, entries)
		if len(conflicts) > 0 {
			for name := range conflicts {
				output.Conflicts = append(output.Conflicts, name)
			}
			return output, fmt.Errorf("apply rejected: %d conflict(s) detected", len(conflicts))
		}
	}

	// Execute apply operations in parallel
	results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, name string, entry staging.Entry) (staging.Operation, error) {
		err := u.Strategy.Apply(ctx, name, entry)
		return entry.Operation, err
	})

	// Collect results
	for name := range entries {
		result := results[name]
		resultEntry := ApplyResultEntry{Name: name}

		if result.Err != nil {
			resultEntry.Status = ApplyResultFailed
			resultEntry.Error = result.Err
			output.Failed++
		} else {
			switch result.Value {
			case staging.OperationCreate:
				resultEntry.Status = ApplyResultCreated
			case staging.OperationUpdate:
				resultEntry.Status = ApplyResultUpdated
			case staging.OperationDelete:
				resultEntry.Status = ApplyResultDeleted
			}
			// Unstage successful operations
			_ = u.Store.Unstage(service, name)
			output.Succeeded++
		}
		output.Results = append(output.Results, resultEntry)
	}

	if output.Failed > 0 {
		return output, fmt.Errorf("applied %d, failed %d", output.Succeeded, output.Failed)
	}

	return output, nil
}

package stage_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/stage"
)

func TestEntryPrinter_PrintEntry(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name              string
		entryName         string
		entry             stage.Entry
		verbose           bool
		showDeleteOptions bool
		wantContains      []string
		wantNotContains   []string
	}{
		{
			name:      "set operation non-verbose",
			entryName: "/app/config",
			entry: stage.Entry{
				Operation: stage.OperationSet,
				Value:     "test-value",
				StagedAt:  fixedTime,
			},
			verbose:      false,
			wantContains: []string{"M", "/app/config"},
		},
		{
			name:      "delete operation non-verbose",
			entryName: "/app/secret",
			entry: stage.Entry{
				Operation: stage.OperationDelete,
				StagedAt:  fixedTime,
			},
			verbose:      false,
			wantContains: []string{"D", "/app/secret"},
		},
		{
			name:      "set operation verbose",
			entryName: "/app/config",
			entry: stage.Entry{
				Operation: stage.OperationSet,
				Value:     "test-value",
				StagedAt:  fixedTime,
			},
			verbose:      true,
			wantContains: []string{"M", "/app/config", "Staged:", "2024-01-15 10:30:00", "Value:", "test-value"},
		},
		{
			name:      "set operation verbose with long value",
			entryName: "/app/config",
			entry: stage.Entry{
				Operation: stage.OperationSet,
				Value:     strings.Repeat("x", 150),
				StagedAt:  fixedTime,
			},
			verbose:      true,
			wantContains: []string{"M", "/app/config", "Staged:", "Value:", "..."},
		},
		{
			name:      "delete operation verbose without options",
			entryName: "/app/secret",
			entry: stage.Entry{
				Operation: stage.OperationDelete,
				StagedAt:  fixedTime,
			},
			verbose:           true,
			showDeleteOptions: true,
			wantContains:      []string{"D", "/app/secret", "Staged:"},
			wantNotContains:   []string{"Delete:"},
		},
		{
			name:      "delete operation verbose with force option",
			entryName: "/app/secret",
			entry: stage.Entry{
				Operation:     stage.OperationDelete,
				StagedAt:      fixedTime,
				DeleteOptions: &stage.DeleteOptions{Force: true},
			},
			verbose:           true,
			showDeleteOptions: true,
			wantContains:      []string{"D", "/app/secret", "Staged:", "Delete:", "force", "immediate", "no recovery"},
		},
		{
			name:      "delete operation verbose with recovery window",
			entryName: "/app/secret",
			entry: stage.Entry{
				Operation:     stage.OperationDelete,
				StagedAt:      fixedTime,
				DeleteOptions: &stage.DeleteOptions{RecoveryWindow: 7},
			},
			verbose:           true,
			showDeleteOptions: true,
			wantContains:      []string{"D", "/app/secret", "Staged:", "Delete:", "7 days recovery window"},
		},
		{
			name:      "delete operation verbose with showDeleteOptions false",
			entryName: "/app/secret",
			entry: stage.Entry{
				Operation:     stage.OperationDelete,
				StagedAt:      fixedTime,
				DeleteOptions: &stage.DeleteOptions{Force: true},
			},
			verbose:           true,
			showDeleteOptions: false,
			wantContains:      []string{"D", "/app/secret", "Staged:"},
			wantNotContains:   []string{"Delete:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			printer := &stage.EntryPrinter{Writer: &buf}

			printer.PrintEntry(tt.entryName, tt.entry, tt.verbose, tt.showDeleteOptions)

			output := buf.String()
			for _, want := range tt.wantContains {
				assert.Contains(t, output, want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, output, notWant)
			}
		})
	}
}

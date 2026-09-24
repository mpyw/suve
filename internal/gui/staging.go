//go:build production || dev

package gui

import (
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StagingStatusResult represents the result of staging status.
type StagingStatusResult struct {
	Param      []StagingEntry    `json:"param"`
	Secret     []StagingEntry    `json:"secret"`
	ParamTags  []StagingTagEntry `json:"paramTags"`
	SecretTags []StagingTagEntry `json:"secretTags"`
}

// StagingEntry represents a staged entry change.
type StagingEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace the entry is staged under
	// (empty for the null/default namespace and every other provider). The
	// frontend shows it as a badge and passes it back to unstage/edit/delete.
	Namespace string  `json:"namespace"`
	Operation string  `json:"operation"`
	Value     *string `json:"value,omitempty"`
	StagedAt  string  `json:"stagedAt"`
}

// StagingTagEntry represents a staged tag change.
type StagingTagEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace of the tagged item (empty for
	// the null/default namespace and every other provider).
	Namespace  string            `json:"namespace"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags []string          `json:"removeTags,omitempty"`
	StagedAt   string            `json:"stagedAt"`
}

// StagingApplyEntryResult represents a single entry apply result.
type StagingApplyEntryResult struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace the entry was applied under
	// (empty for the null/default namespace and every other provider). The
	// frontend shows it as a badge so multi-namespace App Config results are
	// distinguishable.
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	// UnstageError is set when the cloud apply succeeded but clearing the entry
	// from the staging store afterwards failed. The entry is still staged, so a
	// later apply would re-run it; the frontend surfaces this as a warning row.
	UnstageError string `json:"unstageError,omitempty"`
}

// StagingApplyTagResult represents a single tag apply result.
type StagingApplyTagResult struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace the tags were applied under
	// (empty for the null/default namespace and every other provider).
	Namespace  string            `json:"namespace"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags []string          `json:"removeTags,omitempty"`
	Error      string            `json:"error,omitempty"`
	// UnstageError is set when the cloud tag apply succeeded but clearing the
	// staged tag afterwards failed (see StagingApplyEntryResult.UnstageError).
	UnstageError string `json:"unstageError,omitempty"`
}

// StagingApplyResult represents the result of applying staged changes.
type StagingApplyResult struct {
	ServiceName    string                    `json:"serviceName"`
	EntryResults   []StagingApplyEntryResult `json:"entryResults"`
	TagResults     []StagingApplyTagResult   `json:"tagResults"`
	Conflicts      []string                  `json:"conflicts,omitempty"`
	EntrySucceeded int                       `json:"entrySucceeded"`
	EntryFailed    int                       `json:"entryFailed"`
	TagSucceeded   int                       `json:"tagSucceeded"`
	TagFailed      int                       `json:"tagFailed"`
}

// StagingResetResult represents the result of resetting staged changes.
type StagingResetResult struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Count       int    `json:"count,omitempty"`
	ServiceName string `json:"serviceName"`
}

// StagingAddResult represents the result of staging an add operation.
type StagingAddResult struct {
	Name string `json:"name"`
}

// StagingEditResult represents the result of staging an edit operation.
type StagingEditResult struct {
	Name string `json:"name"`
}

// StagingDeleteResult represents the result of staging a delete operation.
type StagingDeleteResult struct {
	Name string `json:"name"`
}

// StagingUnstageResult represents the result of unstaging an item.
type StagingUnstageResult struct {
	Name string `json:"name"`
}

// StagingAddTagResult represents the result of staging a tag addition.
type StagingAddTagResult struct {
	Name string `json:"name"`
}

// StagingRemoveTagResult represents the result of staging a tag removal.
type StagingRemoveTagResult struct {
	Name string `json:"name"`
}

// StagingCancelAddTagResult represents the result of canceling a staged tag addition.
type StagingCancelAddTagResult struct {
	Name string `json:"name"`
}

// StagingCancelRemoveTagResult represents the result of canceling a staged tag removal.
type StagingCancelRemoveTagResult struct {
	Name string `json:"name"`
}

// StagingDiffResult represents the result of diffing staged changes.
type StagingDiffResult struct {
	ItemName   string                `json:"itemName"`
	Entries    []StagingDiffEntry    `json:"entries"`
	TagEntries []StagingDiffTagEntry `json:"tagEntries"`
}

// StagingDiffEntry represents a single diff entry. RemoteValue/RemoteIdentifier
// are the current value and version identifier in the remote store the staged
// value is compared against.
type StagingDiffEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace of the entry (empty for the
	// null/default namespace and every other provider).
	Namespace        string  `json:"namespace"`
	Type             string  `json:"type"` // "normal", "create", "autoUnstaged", "warning"
	Operation        string  `json:"operation,omitempty"`
	RemoteValue      string  `json:"remoteValue,omitempty"`
	RemoteIdentifier string  `json:"remoteIdentifier,omitempty"`
	StagedValue      string  `json:"stagedValue,omitempty"`
	Description      *string `json:"description,omitempty"`
	Warning          string  `json:"warning,omitempty"`
	// Secret reports whether this entry's values are secret material (a
	// SecureString param, or any secret-service entry) so the staging review
	// masks them instead of rendering cleartext. Mirrors the TUI's per-row flag
	// (staging/view.go); threaded from stagingusecase.DiffEntry.Secret (#715).
	Secret bool `json:"secret"`
}

// StagingDiffTagEntry represents a single diff tag entry.
type StagingDiffTagEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace of the tagged item (empty for
	// the null/default namespace and every other provider).
	Namespace  string            `json:"namespace"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags map[string]string `json:"removeTags,omitempty"` // key=current remote value
}

// Enum → frontend-string lookup tables. Kept as immutable package-level maps so
// the conversion sites stay a single lookup instead of a repeated switch.
//
//nolint:gochecknoglobals // immutable enum→string lookup tables
var (
	stagingApplyStatusNames = map[stagingusecase.ApplyResultStatus]string{
		stagingusecase.ApplyResultCreated: "created",
		stagingusecase.ApplyResultUpdated: "updated",
		stagingusecase.ApplyResultDeleted: "deleted",
		stagingusecase.ApplyResultFailed:  "failed",
	}

	stagingResetTypeNames = map[stagingusecase.ResetResultType]string{
		stagingusecase.ResetResultUnstaged:      "unstaged",
		stagingusecase.ResetResultUnstagedAll:   "unstagedAll",
		stagingusecase.ResetResultRestored:      "restored",
		stagingusecase.ResetResultNotStaged:     "notStaged",
		stagingusecase.ResetResultNothingStaged: "nothingStaged",
		stagingusecase.ResetResultSkipped:       "skipped",
		stagingusecase.ResetResultUnstagedTag:   "unstagedTag",
	}

	stagingDiffEntryTypeNames = map[stagingusecase.DiffEntryType]string{
		stagingusecase.DiffEntryNormal:       "normal",
		stagingusecase.DiffEntryCreate:       "create",
		stagingusecase.DiffEntryAutoUnstaged: "autoUnstaged",
		stagingusecase.DiffEntryWarning:      "warning",
	}
)

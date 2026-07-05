//go:build production || dev

package gui

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// modelsPath is the Wails-generated (hand-maintained) TypeScript mirror of the
// Go binding DTOs that the frontend consumes.
const modelsPath = "frontend/wailsjs/go/models.ts"

// dtoContract lists every exported binding DTO whose JSON shape the frontend
// depends on. Adding a new binding DTO (or a field to one) requires updating
// both the Go struct and models.ts; this test fails loudly when the two drift.
//
//nolint:exhaustruct // zero values are fine: the test only inspects field tags
func dtoContract() []any {
	return []any{
		// app.go
		AWSIdentityResult{},
		// param.go
		ParamListResult{}, ParamListEntry{}, ParamShowTag{}, ParamShowResult{},
		ParamLogResult{}, ParamLogEntry{}, ParamDiffResult{}, ParamSetResult{},
		ParamDeleteResult{},
		// secret.go
		SecretListResult{}, SecretListEntry{}, SecretShowTag{}, SecretShowResult{},
		SecretLogResult{}, SecretLogEntry{}, SecretCreateResult{}, SecretUpdateResult{},
		SecretDeleteResult{}, SecretDiffResult{}, SecretRestoreResult{},
		// staging.go
		StagingStatusResult{}, StagingEntry{}, StagingTagEntry{},
		StagingApplyEntryResult{}, StagingApplyTagResult{}, StagingApplyResult{},
		StagingResetResult{}, StagingAddResult{}, StagingEditResult{},
		StagingDeleteResult{}, StagingUnstageResult{}, StagingAddTagResult{},
		StagingRemoveTagResult{}, StagingCancelAddTagResult{}, StagingCancelRemoveTagResult{},
		StagingDiffResult{}, StagingDiffEntry{}, StagingDiffTagEntry{},
		StagingCheckStatusResult{}, StagingDrainResult{}, StagingPersistResult{},
		StagingFileStatusResult{}, StagingDropResult{},
	}
}

// jsonFieldNames returns the JSON object keys a struct type marshals to,
// honoring `json:"name"` tags (including ",omitempty"), skipping `json:"-"`,
// and defaulting to the Go field name when no tag is present.
func jsonFieldNames(t reflect.Type) []string {
	var names []string

	for i := range t.NumField() {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}

		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")

		switch name {
		case "-":
			continue
		case "":
			name = field.Name
		}

		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// tsClassFields parses models.ts and returns, for each exported class, the set
// of JSON keys its constructor reads via source["key"].
func tsClassFields(t *testing.T) map[string][]string {
	t.Helper()

	data, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatalf("read %s: %v", modelsPath, err)
	}

	classRe := regexp.MustCompile(`export class (\w+) \{`)
	sourceKeyRe := regexp.MustCompile(`source\["([^"]+)"\]`)

	result := make(map[string][]string)

	src := string(data)
	locs := classRe.FindAllStringSubmatchIndex(src, -1)

	for i, loc := range locs {
		name := src[loc[2]:loc[3]]
		start := loc[1]

		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}

		body := src[start:end]

		seen := make(map[string]struct{})

		var keys []string

		for _, m := range sourceKeyRe.FindAllStringSubmatch(body, -1) {
			key := m[1]
			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}

			keys = append(keys, key)
		}

		sort.Strings(keys)
		result[name] = keys
	}

	return result
}

// TestDTOContract guards against Go-binding ↔ frontend DTO drift: every Go DTO
// must have a matching TypeScript class in models.ts with the exact same set of
// JSON field names, and vice versa.
func TestDTOContract(t *testing.T) {
	t.Parallel()

	tsFields := tsClassFields(t)

	goFields := make(map[string][]string)

	for _, dto := range dtoContract() {
		typ := reflect.TypeOf(dto)
		goFields[typ.Name()] = jsonFieldNames(typ)
	}

	// Every Go DTO has a TS class with identical JSON field names.
	for name, want := range goFields {
		got, ok := tsFields[name]
		if !ok {
			t.Errorf("Go DTO %q has no matching class in %s", name, modelsPath)

			continue
		}

		if !reflect.DeepEqual(want, got) {
			t.Errorf("DTO %q field mismatch:\n  Go: %v\n  TS: %v", name, want, got)
		}
	}

	// Every TS class in the gui namespace maps back to a listed Go DTO, so a
	// removed/renamed Go DTO (or an untracked TS class) is caught too.
	for name := range tsFields {
		if _, ok := goFields[name]; !ok {
			t.Errorf("models.ts class %q has no matching Go DTO in dtoContract()", name)
		}
	}
}

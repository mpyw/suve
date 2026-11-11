//go:build production || dev

// The DTO contract test checks every Wails-bound result type of the package
// against the generated models. It tests no one binding file, so it is its own
// namespace; the directive drops the _internal suffix testpackage asks for.
//declscope:namespace dtoContract

package gui

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mpyw/suve/internal/capability"
)

// dtoContractModelsPaths are the Wails-generated TypeScript models of the Go
// binding DTOs that the frontend consumes, one file per Go package.
//
//nolint:gochecknoglobals // fixed list of generated files
var dtoContractModelsPaths = []string{
	"frontend/bindings/github.com/mpyw/suve/internal/gui/models.ts",
	"frontend/bindings/github.com/mpyw/suve/internal/capability/models.ts",
}

// dtoContract lists every exported binding DTO whose JSON shape the frontend
// depends on. Adding a new binding DTO (or a field to one) requires
// regenerating the bindings (mise generate-gui-bindings); this test fails
// loudly when the two drift.
//
//nolint:exhaustruct // zero values are fine: the test only inspects field tags
func dtoContract() []any {
	return []any{
		// app.go
		ScopeSelection{},
		// target.go
		ScopeTarget{}, ScopeTargetSegment{},
		// detect.go, capability.go
		DetectResult{}, capability.ServiceCapability{}, capability.ProviderCapability{},
		// param.go
		ParamListResult{}, ParamListEntry{}, ParamShowTag{}, ParamShowResult{},
		ParamLogResult{}, ParamLogEntry{}, ParamDiffResult{}, ParamSetResult{},
		ParamDeleteResult{},
		// secret.go
		SecretListResult{}, SecretListEntry{}, SecretShowTag{}, SecretExtraField{}, SecretShowResult{},
		SecretLogResult{}, SecretLogEntry{}, SecretCreateResult{}, SecretUpdateResult{},
		SecretDeleteResult{}, SecretDiffResult{}, SecretRestoreResult{},
		// staging.go
		StagingStatusResult{}, StagingEntry{}, StagingTagEntry{},
		StagingApplyEntryResult{}, StagingApplyTagResult{}, StagingApplyResult{},
		StagingResetResult{}, StagingAddResult{}, StagingEditResult{},
		StagingDeleteResult{}, StagingUnstageResult{}, StagingAddTagResult{},
		StagingRemoveTagResult{}, StagingCancelAddTagResult{}, StagingCancelRemoveTagResult{},
		StagingDiffResult{}, StagingDiffEntry{}, StagingDiffTagEntry{},
		StagingCheckStatusResult{}, StagingExportResult{}, StagingImportResult{},
		StagingEnvelopeInfoResult{},
	}
}

// dtoContractGoFields returns the JSON object keys a struct type marshals to,
// honoring `json:"name"` tags (including ",omitempty"), skipping `json:"-"`,
// and defaulting to the Go field name when no tag is present.
func dtoContractGoFields(t reflect.Type) []string {
	var names []string

	for field := range t.Fields() {
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

// dtoContractTSFields parses the generated models and returns, for each
// exported class, the set of JSON keys it declares as fields ("key": type or
// "key"?: type), read from the class body up to its constructor.
func dtoContractTSFields(t *testing.T) map[string][]string {
	t.Helper()

	classRe := regexp.MustCompile(`export class (\w+) \{`)
	fieldRe := regexp.MustCompile(`(?m)^\s+"([^"]+)"\??:`)

	result := make(map[string][]string)

	for _, path := range dtoContractModelsPaths {
		data, err := os.ReadFile(path) //nolint:gosec // path is from the fixed dtoContractModelsPaths list
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		src := string(data)

		for _, loc := range classRe.FindAllStringSubmatchIndex(src, -1) {
			name := src[loc[2]:loc[3]]
			body := src[loc[1]:]

			if i := strings.Index(body, "constructor("); i >= 0 {
				body = body[:i]
			}

			var keys []string
			for _, m := range fieldRe.FindAllStringSubmatch(body, -1) {
				keys = append(keys, m[1])
			}

			sort.Strings(keys)
			result[name] = keys
		}
	}

	return result
}

// TestDTOContract guards against Go-binding ↔ frontend DTO drift: every Go DTO
// must have a matching generated TypeScript class with the exact same set of
// JSON field names, and vice versa.
func TestDTOContract(t *testing.T) {
	t.Parallel()

	tsFields := dtoContractTSFields(t)

	goFields := make(map[string][]string)

	for _, dto := range dtoContract() {
		typ := reflect.TypeOf(dto)
		goFields[typ.Name()] = dtoContractGoFields(typ)
	}

	// Every Go DTO has a TS class with identical JSON field names.
	for name, want := range goFields {
		got, ok := tsFields[name]
		if !ok {
			t.Errorf("Go DTO %q has no matching class in %v", name, dtoContractModelsPaths)

			continue
		}

		if !reflect.DeepEqual(want, got) {
			t.Errorf("DTO %q field mismatch:\n  Go: %v\n  TS: %v", name, want, got)
		}
	}

	// Every generated TS class maps back to a listed Go DTO, so a
	// removed/renamed Go DTO (or an untracked TS class) is caught too.
	for name := range tsFields {
		if _, ok := goFields[name]; !ok {
			t.Errorf("generated class %q has no matching Go DTO in dtoContract()", name)
		}
	}
}

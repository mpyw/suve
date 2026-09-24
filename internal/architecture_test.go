// Package internal hosts a cross-cutting architecture guard test. It has no
// non-test Go files: it exists only to assert an import boundary across the
// internal tree.
//
//nolint:testpackage // must live in package internal so `go test ./internal` builds this guard
package internal

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestNoCloudSDKOutsideProvider enforces that only internal/provider (which
// includes internal/provider/aws, .../gcloud, .../azure) may import a cloud
// SDK. It walks the entire internal/ tree plus cmd/ and fails loudly if any
// other non-test package reintroduces a direct cloud-SDK dependency, which
// would break provider pluggability.
func TestNoCloudSDKOutsideProvider(t *testing.T) {
	t.Parallel()

	// guardedRoots are the trees (relative to this package dir) that are walked
	// in full. Everything under them must not import a cloud SDK directly,
	// except for the allowedRoots subtrees which are pruned from the walk.
	guardedRoots := []string{".", "../cmd"}

	// allowedRoots are the subtrees (relative to this package dir) that are
	// permitted to import a cloud SDK and are therefore skipped during the walk.
	allowedRoots := map[string]struct{}{
		"provider": {},
	}

	fset := token.NewFileSet()

	for _, root := range guardedRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if _, ok := allowedRoots[path]; ok {
					return filepath.SkipDir
				}

				return nil
			}

			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}

			for _, imp := range file.Imports {
				importPath, uerr := strconv.Unquote(imp.Path.Value)
				if uerr != nil {
					continue
				}

				if isCloudSDKImport(importPath) {
					t.Errorf(
						"%s imports %q: cloud SDKs must stay behind the provider seam "+
							"(only internal/provider/<cloud> may import its own SDK)",
						path, importPath,
					)
				}
			}

			return nil
		})
		if err != nil {
			t.Fatalf("walking %q: %v", root, err)
		}
	}
}

// isCloudSDKImport reports whether importPath is a cloud SDK package banned
// outside the provider seam. SDK service packages are matched by prefix so their
// subpackages (e.g. .../service/ssm/types, .../secretmanager/apiv1/...) are
// caught too.
func isCloudSDKImport(importPath string) bool {
	forbiddenPrefixes := []string{
		"github.com/aws/aws-sdk-go-v2/service/ssm",
		"github.com/aws/aws-sdk-go-v2/service/secretsmanager",
		"cloud.google.com/go/secretmanager",
		"github.com/Azure/azure-sdk-for-go",
	}

	return slices.ContainsFunc(forbiddenPrefixes, func(p string) bool {
		return importPath == p || strings.HasPrefix(importPath, p+"/")
	})
}

// TestSDKFreeProviderVocabulary enforces that the provider vocabulary packages
// the TUI and GUI import directly stay SDK-free even though they sit under
// internal/provider, where the SDK is otherwise allowed.
func TestSDKFreeProviderVocabulary(t *testing.T) {
	t.Parallel()

	sdkFreeDirs := []string{
		"provider/aws/paramtype",
	}

	fset := token.NewFileSet()

	for _, dir := range sdkFreeDirs {
		paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("listing %q: %v", dir, err)
		}

		if len(paths) == 0 {
			t.Fatalf("%q has no Go files; update sdkFreeDirs", dir)
		}

		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}

			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parsing %q: %v", path, err)
			}

			for _, imp := range file.Imports {
				if importPath, err := strconv.Unquote(imp.Path.Value); err == nil && isCloudSDKImport(importPath) {
					t.Errorf("%s imports %q: this package must stay SDK-free", path, importPath)
				}
			}
		}
	}
}

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
	"strconv"
	"strings"
	"testing"
)

// TestNoAWSSDKOutsideProviderAWS enforces that only internal/provider/aws (plus
// the allowed low-level package internal/infra, which is not under the guarded
// roots) imports the AWS service SDK, that only internal/provider/gcloud imports
// the Google Cloud Secret Manager SDK, and that only internal/provider/azure
// imports the Azure SDK. It fails loudly if a package under internal/cli,
// internal/usecase, internal/staging, or internal/gui reintroduces a direct
// cloud-SDK dependency, which would break provider pluggability.
func TestNoAWSSDKOutsideProviderAWS(t *testing.T) {
	t.Parallel()

	// guardedRoots are the internal subtrees (relative to this package dir) that
	// must not import a cloud SDK, or the (now-removed) paramapi/secretapi
	// aliases, directly.
	guardedRoots := []string{"cli", "usecase", "staging", "gui"}

	// forbiddenPrefixes are import paths banned in non-test packages under a
	// guarded root. SDK service packages are matched by prefix so their
	// subpackages (e.g. .../service/ssm/types, .../secretmanager/apiv1/...) are
	// caught too.
	forbiddenPrefixes := []string{
		"github.com/aws/aws-sdk-go-v2/service/ssm",
		"github.com/aws/aws-sdk-go-v2/service/secretsmanager",
		"cloud.google.com/go/secretmanager",
		"github.com/Azure/azure-sdk-for-go",
		"github.com/mpyw/suve/internal/api/paramapi",
		"github.com/mpyw/suve/internal/api/secretapi",
	}

	isForbidden := func(importPath string) bool {
		for _, p := range forbiddenPrefixes {
			if importPath == p || strings.HasPrefix(importPath, p+"/") {
				return true
			}
		}

		return false
	}

	fset := token.NewFileSet()

	for _, root := range guardedRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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

				if isForbidden(importPath) {
					t.Errorf(
						"%s imports %q: the AWS SDK must stay behind the provider seam "+
							"(only internal/provider/aws may import it)",
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

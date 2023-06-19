package actions

import (
	"context"
	"io"

	"github.com/mpyw/suve/pkg/api"
	"github.com/mpyw/suve/pkg/core/printing"
	"github.com/mpyw/suve/pkg/core/versioning"
)

type Dependencies struct {
	API           api.API
	Writer        io.Writer
	Printer       printing.PrettyPrinter
	VersionParser versioning.VersionParser
}

type contextKey struct{}

func GetDependencies(ctx context.Context) Dependencies {
	deps, _ := ctx.Value(contextKey{}).(Dependencies)
	return deps
}

func WithAPI(ctx context.Context, api api.API) context.Context {
	deps := GetDependencies(ctx)
	deps.API = api
	return context.WithValue(ctx, contextKey{}, deps)
}

func WithWriter(ctx context.Context, writer io.Writer) context.Context {
	deps := GetDependencies(ctx)
	deps.Writer = writer
	return context.WithValue(ctx, contextKey{}, deps)
}

func WithPrinter(ctx context.Context, printer printing.PrettyPrinter) context.Context {
	deps := GetDependencies(ctx)
	deps.Printer = printer
	return context.WithValue(ctx, contextKey{}, deps)
}

func WithVersionParser(ctx context.Context, parser versioning.VersionParser) context.Context {
	deps := GetDependencies(ctx)
	deps.VersionParser = parser
	return context.WithValue(ctx, contextKey{}, deps)
}

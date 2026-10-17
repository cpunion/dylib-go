package moduleexample

import (
	"context"

	"github.com/cpunion/dylib-go/module"
)

// Manifest imports both a function and data under consumer-specific names.
// Paths can name native objects, archives or complete OS libraries. Version
// contracts are explicit metadata; they do not infer binary signatures.
func Manifest(provider, consumer string) module.Manifest {
	return module.Manifest{Format: module.FormatVersion, Roots: []string{"app"}, Modules: []module.Module{
		{Name: "app", Version: "app/1", Files: []string{consumer}, Exports: []string{"module_run"}, Imports: []module.Import{
			{Provider: "math", Version: "math/1", Symbol: "module_add", As: "remote_add"},
			{Provider: "math", Version: "math/1", Symbol: "module_bias", As: "remote_bias"},
		}},
		{Name: "math", Version: "math/1", Files: []string{provider}, Exports: []string{"module_add", "module_bias"}},
	}}
}

// Discover resolves app/1 and its exact declared providers from *.dylib.json catalogs.
// It reads metadata only; pass the returned manifest to module.Open to load it.
func Discover(ctx context.Context, directories ...string) (module.Manifest, error) {
	return module.Discover(ctx, []module.Requirement{{Name: "app", Version: "app/1"}}, directories...)
}

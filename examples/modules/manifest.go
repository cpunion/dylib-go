package moduleexample

import (
	"context"

	"github.com/ramblelabs/dylib-go/module"
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

// DiscoverCompatible prefers app/2 but accepts app/1, while requiring math/1.
// The caller declares these ABI contracts; selection does not inspect binaries.
func DiscoverCompatible(ctx context.Context, directories ...string) (module.Manifest, error) {
	return module.DiscoverCompatible(ctx, []module.CompatibleRequirement{
		{Name: "app", Versions: []string{"app/2", "app/1"}},
		{Name: "math", Versions: []string{"math/1"}},
	}, directories...)
}

// DiscoverVersionRanges declares acceptance of stable app versions in [1, 2).
// Catalog dependencies still require their exact declared ABI contracts.
func DiscoverVersionRanges(ctx context.Context, directories ...string) (module.Manifest, error) {
	return module.DiscoverVersionRanges(ctx, []module.VersionRangeRequirement{
		{Name: "app", AtLeast: "1.0.0", Before: "2.0.0"},
	}, directories...)
}

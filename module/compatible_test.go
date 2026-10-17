package module_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/module"
)

func compatibleModule(name, version string, imports ...module.Import) module.Module {
	return module.Module{Name: name, Version: version, Files: []string{"absent.o"}, Exports: []string{"entry"}, Imports: imports}
}

func TestDiscoverCompatibleGraph(t *testing.T) {
	dir := t.TempDir()
	imp := func(version string) module.Import {
		return module.Import{Provider: "math", Version: version, Symbol: "entry"}
	}
	writeCatalog(t, dir, "versions",
		compatibleModule("app", "new", imp("two")), compatibleModule("app", "old", imp("one")),
		compatibleModule("side", "stable", imp("one")),
		compatibleModule("math", "one"), compatibleModule("math", "two"))
	roots := []module.CompatibleRequirement{{Name: "app", Versions: []string{"new", "old"}}, {Name: "side", Versions: []string{"stable"}}}
	manifest, err := module.DiscoverCompatible(context.Background(), roots, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest.Roots, []string{"app", "side"}) || len(manifest.Modules) != 3 || manifest.Modules[0].Version != "old" || manifest.Modules[1].Version != "one" || manifest.Modules[2].Name != "side" {
		t.Fatal("later root did not reconsider earlier dependency selection", manifest)
	}
	again, err := module.DiscoverCompatible(context.Background(), roots, dir)
	if err != nil || !reflect.DeepEqual(manifest, again) {
		t.Fatal("unstable selection", again, err)
	}
	// Removing the later root restores the earlier preferred graph. No numeric
	// ordering or semantic-version compatibility is inferred from these names.
	preferred, err := module.DiscoverCompatible(context.Background(), roots[:1], dir)
	if err != nil || preferred.Modules[0].Version != "new" || preferred.Modules[1].Version != "two" {
		t.Fatal(preferred, err)
	}
	// A dependency may also be a root, but cannot escape that root's allowlist.
	allowed := append(roots[:1:1], module.CompatibleRequirement{Name: "math", Versions: []string{"one"}})
	pinned, err := module.DiscoverCompatible(context.Background(), allowed, dir)
	if err != nil || pinned.Modules[0].Version != "old" || len(pinned.Modules) != 2 {
		t.Fatal(pinned, err)
	}
	// Exact discovery still refuses a graph requiring conflicting versions.
	if _, err := module.Discover(context.Background(), []module.Requirement{{Name: "app", Version: "new"}, {Name: "side", Version: "stable"}}, dir); err == nil {
		t.Fatal("exact discovery changed its contract")
	}
}

func TestDiscoverCompatibleInvalidCandidateFallback(t *testing.T) {
	for _, name := range []string{"missing_root", "missing_dependency", "missing_export", "cycle", "conflicting_imports"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			good := compatibleModule("app", "good")
			bad := compatibleModule("app", "bad", module.Import{Provider: "dep", Version: "1", Symbol: "entry"})
			dep := compatibleModule("dep", "1")
			switch name {
			case "missing_root":
				bad.Version = "not-requested"
			case "missing_dependency":
				dep.Name = "not-requested"
			case "missing_export":
				dep.Exports = nil
			case "cycle":
				dep.Imports = []module.Import{{Provider: "app", Version: "bad", Symbol: "entry"}}
			case "conflicting_imports":
				bad.Imports = append(bad.Imports, module.Import{Provider: "dep", Version: "2", Symbol: "entry", As: "other"})
			}
			writeCatalog(t, dir, "catalog", bad, dep, good)
			got, err := module.DiscoverCompatible(context.Background(), []module.CompatibleRequirement{{Name: "app", Versions: []string{"bad", "good"}}}, dir)
			if err != nil || len(got.Modules) != 1 || got.Modules[0].Version != "good" {
				t.Fatal(got, err)
			}
			got, err = module.DiscoverCompatible(context.Background(), []module.CompatibleRequirement{{Name: "app", Versions: []string{"bad"}}}, dir)
			if err == nil || got.Format != 0 || !strings.Contains(err.Error(), "no compatible provider graph") {
				t.Fatal("invalid graph returned", got, err)
			}
		})
	}
}

func TestDiscoverCompatibleValidation(t *testing.T) {
	dir := t.TempDir()
	for _, roots := range [][]module.CompatibleRequirement{
		nil, {{Name: "app"}}, {{Name: " app", Versions: []string{"1"}}},
		{{Name: "app", Versions: []string{""}}}, {{Name: "app", Versions: []string{"1", "1"}}},
		{{Name: "app", Versions: []string{"1", "bad\x00"}}}, {{Name: "app", Versions: []string{"1 "}}},
		{{Name: "app", Versions: []string{"1"}}, {Name: "app", Versions: []string{"2"}}}, make([]module.CompatibleRequirement, 257),
	} {
		if _, err := module.DiscoverCompatible(context.Background(), roots, dir); err == nil {
			t.Fatal("invalid roots accepted", roots)
		}
	}
	root := []module.CompatibleRequirement{{Name: "app", Versions: []string{"1"}}}
	for _, dirs := range [][]string{nil, {""}, make([]string, 17)} {
		if _, err := module.DiscoverCompatible(context.Background(), root, dirs...); err == nil {
			t.Fatal("invalid directories accepted")
		}
	}
	if _, err := module.DiscoverCompatible(nil, root, dir); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := module.DiscoverCompatible(ctx, root, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// A locally invalid or ambiguous catalog is never ignored as a fallback.
	bad := compatibleModule("app", "1")
	writeCatalog(t, dir, "one", bad, bad)
	if _, err := module.DiscoverCompatible(context.Background(), root, dir); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal(err)
	}
}

func TestDiscoverCompatibleSearchBudget(t *testing.T) {
	dir := t.TempDir()
	versions := make([]string, 4097)
	for i := range versions {
		versions[i] = fmt.Sprint(i)
	}
	if _, err := module.DiscoverCompatible(context.Background(), []module.CompatibleRequirement{{Name: "app", Versions: versions}}, dir); err == nil || !strings.Contains(err.Error(), "4096 version alternatives") {
		t.Fatal(err)
	}
	// Twelve independent binary choices followed by a missing root would
	// require an exponential search without a hard candidate budget.
	var mods []module.Module
	var roots []module.CompatibleRequirement
	for i := 0; i < 12; i++ {
		name := fmt.Sprint("root", i)
		mods = append(mods, compatibleModule(name, "one"), compatibleModule(name, "two"))
		roots = append(roots, module.CompatibleRequirement{Name: name, Versions: []string{"one", "two"}})
	}
	roots = append(roots, module.CompatibleRequirement{Name: "missing", Versions: []string{"one"}})
	writeCatalog(t, dir, "catalog", mods...)
	got, err := module.DiscoverCompatible(context.Background(), roots, dir)
	if err == nil || got.Format != 0 || !strings.Contains(err.Error(), "4096 version candidate attempts") {
		t.Fatal(got, err)
	}
}

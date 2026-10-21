package module_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/module"
)

func TestDiscoverVersionRangesSelection(t *testing.T) {
	dir := t.TempDir()
	var mods []module.Module
	for _, version := range []string{"1.9.0", "1.10.0+zeta", "1.10.0+alpha", "1.11.0-rc.1", "2.0.0", "opaque/99", "1.99.0+" + strings.Repeat("x", 250)} {
		mods = append(mods, compatibleModule("app", version))
	}
	writeCatalog(t, dir, "versions", mods...)
	root := module.VersionRangeRequirement{Name: "app", AtLeast: "1.0.0", Before: "2.0.0"}
	for _, test := range []struct {
		name string
		root module.VersionRangeRequirement
		want string
	}{
		{"stable", root, "1.10.0+alpha"},
		{"prerelease", module.VersionRangeRequirement{Name: "app", AtLeast: "1.0.0", Before: "2.0.0", IncludePrerelease: true}, "1.11.0-rc.1"},
		{"prerelease_interval", module.VersionRangeRequirement{Name: "app", AtLeast: "1.11.0-rc.1", Before: "1.11.0", IncludePrerelease: true}, "1.11.0-rc.1"},
		{"lower_inclusive", module.VersionRangeRequirement{Name: "app", AtLeast: "1.10.0+other", Before: "1.11.0"}, "1.10.0+alpha"},
		{"upper_exclusive", module.VersionRangeRequirement{Name: "app", AtLeast: "1.0.0", Before: "1.10.0+other"}, "1.9.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var previous module.Manifest
			for i := 0; i < 5; i++ {
				got, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{test.root}, dir)
				if err != nil || len(got.Modules) != 1 || got.Modules[0].Version != test.want {
					t.Fatal(got, err)
				}
				if _, err := os.Stat(got.Modules[0].Files[0]); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("fixture unexpectedly has a native input", err)
				}
				if i != 0 && !reflect.DeepEqual(previous, got) {
					t.Fatal("unstable equal-precedence selection")
				}
				previous = got
			}
		})
	}
}

func TestDiscoverVersionRangesGraph(t *testing.T) {
	dir := t.TempDir()
	imp := func(v string) module.Import { return module.Import{Provider: "math", Version: v, Symbol: "entry"} }
	writeCatalog(t, dir, "versions",
		compatibleModule("app", "1.2.0", imp("two")), compatibleModule("app", "1.1.0", imp("one")),
		compatibleModule("side", "1.0.0", imp("one")), compatibleModule("math", "one"), compatibleModule("math", "two"))
	root := func(name string) module.VersionRangeRequirement {
		return module.VersionRangeRequirement{Name: name, AtLeast: "1.0.0", Before: "2.0.0"}
	}
	got, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{root("app"), root("side")}, dir)
	if err != nil || len(got.Modules) != 3 || got.Modules[0].Version != "1.1.0" || got.Modules[1].Version != "one" || got.Modules[2].Name != "side" {
		t.Fatal("later root did not reconsider the newest graph", got, err)
	}
	preferred, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{root("app")}, dir)
	if err != nil || preferred.Modules[0].Version != "1.2.0" {
		t.Fatal(preferred, err)
	}
}

func TestDiscoverVersionRangesExactBuildDependencies(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, "versions",
		compatibleModule("app", "1.0.0", module.Import{Provider: "math", Version: "1.0.0+zeta", Symbol: "entry"}),
		compatibleModule("math", "1.0.0+alpha"), compatibleModule("math", "1.0.0+zeta"))
	got, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{
		{Name: "app", AtLeast: "1.0.0", Before: "2.0.0"},
		{Name: "math", AtLeast: "1.0.0", Before: "2.0.0"},
	}, dir)
	if err != nil || len(got.Modules) != 2 || got.Modules[1].Version != "1.0.0+zeta" {
		t.Fatal("equal precedence relaxed an exact dependency", got, err)
	}
}

func TestDiscoverVersionRangesInvalidCandidateFallback(t *testing.T) {
	for _, name := range []string{"missing_dependency", "missing_export", "cycle", "conflicting_imports", "root_dependency"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			bad := compatibleModule("app", "1.2.0", module.Import{Provider: "dep", Version: "1.0.0", Symbol: "entry"})
			dep := compatibleModule("dep", "1.0.0")
			roots := []module.VersionRangeRequirement{{Name: "app", AtLeast: "1.0.0", Before: "2.0.0"}}
			switch name {
			case "missing_dependency":
				dep.Name = "other"
			case "missing_export":
				dep.Exports = nil
			case "cycle":
				dep.Imports = []module.Import{{Provider: "app", Version: "1.2.0", Symbol: "entry"}}
			case "conflicting_imports":
				bad.Imports = append(bad.Imports, module.Import{Provider: "dep", Version: "1.1.0", Symbol: "entry", As: "other"})
			case "root_dependency":
				roots = append(roots, module.VersionRangeRequirement{Name: "dep", AtLeast: "1.1.0", Before: "2.0.0"})
			}
			writeCatalog(t, dir, "catalog", bad, dep, compatibleModule("app", "1.1.0"), compatibleModule("dep", "1.1.0"))
			got, err := module.DiscoverVersionRanges(context.Background(), roots, dir)
			if err != nil || got.Modules[0].Version != "1.1.0" || name != "root_dependency" && len(got.Modules) != 1 {
				t.Fatal(got, err)
			}
			if name == "root_dependency" && (len(got.Modules) != 2 || got.Modules[1].Version != "1.1.0") {
				t.Fatal("dependency escaped its root interval", got)
			}
		})
	}
}

func TestDiscoverVersionRangesValidation(t *testing.T) {
	dir := t.TempDir()
	valid := module.VersionRangeRequirement{Name: "app", AtLeast: "1.0.0", Before: "2.0.0"}
	for _, roots := range [][]module.VersionRangeRequirement{
		nil, {{}}, {{Name: " app", AtLeast: "1.0.0", Before: "2.0.0"}}, {valid, valid}, make([]module.VersionRangeRequirement, 257),
		{{Name: "app", AtLeast: "1", Before: "2.0.0"}}, {{Name: "app", AtLeast: "1.0.0", Before: ""}},
		{{Name: "app", AtLeast: "2.0.0", Before: "1.0.0"}}, {{Name: "app", AtLeast: "1.0.0+one", Before: "1.0.0+two"}},
		{{Name: "app", AtLeast: "1.0.0+" + strings.Repeat("x", 251), Before: "2.0.0"}},
	} {
		if got, err := module.DiscoverVersionRanges(context.Background(), roots, dir); err == nil || got.Format != 0 {
			t.Fatal("invalid roots returned a manifest", roots, got, err)
		}
	}
	for _, dirs := range [][]string{nil, {""}, make([]string, 17)} {
		if _, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{valid}, dirs...); err == nil {
			t.Fatal("invalid directories accepted")
		}
	}
	if _, err := module.DiscoverVersionRanges(nil, []module.VersionRangeRequirement{valid}, dir); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := module.DiscoverVersionRanges(ctx, []module.VersionRangeRequirement{valid}, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{valid}, dir); err == nil || !strings.Contains(err.Error(), "no provider") {
		t.Fatal("empty range returned a manifest", err)
	}
	mod := compatibleModule("app", "1.0.0")
	writeCatalog(t, dir, "ambiguous", mod, mod)
	if _, err := module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{valid}, dir); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal(err)
	}
}

func TestDiscoverVersionRangesSearchBudget(t *testing.T) {
	dir := t.TempDir()
	var mods []module.Module
	var roots []module.VersionRangeRequirement
	for i := 0; i < 12; i++ {
		name := fmt.Sprint("root", i)
		mods = append(mods, compatibleModule(name, "1.0.0"), compatibleModule(name, "1.1.0"))
		roots = append(roots, module.VersionRangeRequirement{Name: name, AtLeast: "1.0.0", Before: "2.0.0"})
	}
	mods = append(mods, compatibleModule("bad", "1.0.0", module.Import{Provider: "missing", Version: "exact", Symbol: "entry"}))
	roots = append(roots, module.VersionRangeRequirement{Name: "bad", AtLeast: "1.0.0", Before: "2.0.0"})
	writeCatalog(t, dir, "catalog", mods...)
	got, err := module.DiscoverVersionRanges(context.Background(), roots, dir)
	if err == nil || got.Format != 0 || !strings.Contains(err.Error(), "4096 version candidate attempts") {
		t.Fatal(got, err)
	}
}

package module_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	example "github.com/cpunion/dylib-go/examples/modules"
	"github.com/cpunion/dylib-go/module"
)

func writeCatalog(t *testing.T, directory, name string, modules ...module.Module) string {
	t.Helper()
	data, err := json.Marshal(module.Catalog{Format: module.FormatVersion, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name+".dylib.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverCatalogGraph(t *testing.T) {
	appDir, providerDir := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(appDir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(appDir, "nested", "ignored.dylib.json"), filepath.Join(appDir, "ignored.json")} {
		if err := os.WriteFile(path, []byte("invalid ignored JSON"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	declarations := example.Manifest("provider.o", "consumer.o").Modules
	second := declarations[0]
	second.Name = "second"
	writeCatalog(t, appDir, "apps", declarations[0], second)
	other := declarations[1]
	other.Version = "math/2"
	other.Files = []string{"other.o"}
	writeCatalog(t, providerDir, "math", declarations[1], other, module.Module{Name: "unused", Version: "1", Files: []string{"missing.o"}, Imports: []module.Import{{Provider: "not-installed", Version: "1", Symbol: "unused"}}})
	roots := []module.Requirement{{Name: "app", Version: "app/1"}, {Name: "second", Version: "app/1"}}
	got, err := module.Discover(context.Background(), roots, appDir, providerDir, appDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Modules) != 3 || !reflect.DeepEqual(got.Roots, []string{"app", "second"}) {
		t.Fatal(got)
	}
	if got.Modules[0].Name != "app" || got.Modules[1].Name != "math" || got.Modules[2].Name != "second" {
		t.Fatal("unstable dependency traversal", got)
	}
	if got.Modules[0].Files[0] != filepath.Join(appDir, "consumer.o") || got.Modules[1].Files[0] != filepath.Join(providerDir, "provider.o") {
		t.Fatal("catalog-relative paths", got)
	}
	again, err := module.Discover(context.Background(), roots, appDir, providerDir)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("discovery is not deterministic", again, err)
	}
	// Only metadata has been read: every declared native file is absent.
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	selected, err := module.Discover(context.Background(), []module.Requirement{{Name: "math", Version: "math/2"}}, providerDir)
	if err != nil || len(selected.Modules) != 1 || selected.Modules[0].Version != "math/2" {
		t.Fatal(selected, err)
	}
}

func TestDiscoverCatalogContracts(t *testing.T) {
	for name, patch := range map[string]func([]module.Module) []module.Module{
		"missing_provider":   func(ms []module.Module) []module.Module { return ms[:1] },
		"wrong_version":      func(ms []module.Module) []module.Module { ms[1].Version = "math/2"; return ms },
		"missing_export":     func(ms []module.Module) []module.Module { ms[1].Exports = []string{"other"}; return ms },
		"duplicate_provider": func(ms []module.Module) []module.Module { return append(ms, ms[1]) },
		"duplicate_alias":    func(ms []module.Module) []module.Module { ms[0].Imports[0].As = "remote_bias"; return ms },
		"bad_path":           func(ms []module.Module) []module.Module { ms[1].Files = []string{"bad\x00path"}; return ms },
		"bad_unused": func(ms []module.Module) []module.Module {
			return append(ms, module.Module{Name: "unused", Version: "1"})
		},
		"cycle": func(ms []module.Module) []module.Module {
			ms[1].Imports = []module.Import{{Provider: "app", Version: "app/1", Symbol: "module_run"}}
			return ms
		},
		"conflicting_versions": func(ms []module.Module) []module.Module {
			other := ms[1]
			other.Version = "math/2"
			ms[0].Imports[1].Version = "math/2"
			return append(ms, other)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeCatalog(t, dir, "catalog", patch(example.Manifest("provider.o", "consumer.o").Modules)...)
			if got, err := module.Discover(context.Background(), []module.Requirement{{Name: "app", Version: "app/1"}}, dir); err == nil || got.Format != 0 {
				t.Fatal("invalid catalog graph returned", got, err)
			}
		})
	}
	t.Run("cross_directory_ambiguity", func(t *testing.T) {
		a, b := t.TempDir(), t.TempDir()
		mod := example.Manifest("provider.o", "consumer.o").Modules[1]
		writeCatalog(t, a, "one", mod)
		writeCatalog(t, b, "two", mod)
		if _, err := module.Discover(context.Background(), []module.Requirement{{Name: "math", Version: "math/1"}}, a, b); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatal(err)
		}
	})
}

func TestDiscoverJSONAndInputs(t *testing.T) {
	for _, input := range []string{"", "null", `{}`, `{"format":2,"modules":[]}`, `{"format":1,"roots":[],"modules":[]}`, `{"format":1,"modules":[]} {}`, strings.Repeat(" ", (1<<20)+1)} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "bad.dylib.json"), []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := module.Discover(context.Background(), []module.Requirement{{Name: "app", Version: "1"}}, dir); err == nil {
			t.Fatal("invalid catalog JSON accepted")
		}
	}
	dir := t.TempDir()
	for _, roots := range [][]module.Requirement{nil, {{Name: "app"}}, {{Name: "app", Version: "1"}, {Name: "app", Version: "1"}}, {{Name: " bad", Version: "1"}}, make([]module.Requirement, 257)} {
		if _, err := module.Discover(context.Background(), roots, dir); err == nil {
			t.Fatal("invalid roots accepted")
		}
	}
	root := []module.Requirement{{Name: "app", Version: "1"}}
	for _, dirs := range [][]string{nil, {""}, {filepath.Join(dir, "missing")}, make([]string, 17)} {
		if _, err := module.Discover(context.Background(), root, dirs...); err == nil {
			t.Fatal("invalid directories accepted")
		}
	}
	if _, err := module.Discover(nil, root, dir); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := module.Discover(ctx, root, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := module.Discover(context.Background(), root, dir); err == nil {
		t.Fatal("empty catalog accepted")
	}
}

func TestDiscoverCatalogBudgets(t *testing.T) {
	root := []module.Requirement{{Name: "app", Version: "1"}}
	t.Run("inputs", func(t *testing.T) {
		dir := t.TempDir()
		files := make([]string, 4097)
		for i := range files {
			files[i] = "missing.o"
		}
		writeCatalog(t, dir, "inputs", module.Module{Name: "app", Version: "1", Files: files})
		if _, err := module.Discover(context.Background(), root, dir); err == nil || !strings.Contains(err.Error(), "4096 native input") {
			t.Fatal(err)
		}
	})
	t.Run("modules", func(t *testing.T) {
		dir := t.TempDir()
		mods := make([]module.Module, 257)
		for i := range mods {
			mods[i] = module.Module{Name: fmt.Sprint("module", i), Version: "1", Files: []string{"missing.o"}}
		}
		writeCatalog(t, dir, "modules", mods...)
		if _, err := module.Discover(context.Background(), root, dir); err == nil || !strings.Contains(err.Error(), "256 module") {
			t.Fatal(err)
		}
	})
	t.Run("catalogs", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < 257; i++ {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.dylib.json", i)), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := module.Discover(context.Background(), root, dir); err == nil || !strings.Contains(err.Error(), "256 catalogs") {
			t.Fatal(err)
		}
	})
	t.Run("entries", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < 4097; i++ {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprint(i)), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := module.Discover(context.Background(), root, dir); err == nil || !strings.Contains(err.Error(), "4096 directory") {
			t.Fatal(err)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < 9; i++ {
			data, err := json.Marshal(module.Catalog{Format: 1, Modules: []module.Module{{Name: fmt.Sprint("module", i), Version: "1", Files: []string{"missing.o"}}}})
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, []byte(strings.Repeat(" ", (1<<20)-len(data)))...)
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.dylib.json", i)), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := module.Discover(context.Background(), root, dir); err == nil || !strings.Contains(err.Error(), "8 MiB") {
			t.Fatal(err)
		}
	})
}

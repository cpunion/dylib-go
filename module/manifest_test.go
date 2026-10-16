package module_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	example "github.com/cpunion/dylib-go/examples/modules"
	"github.com/cpunion/dylib-go/module"
)

func TestManifestContracts(t *testing.T) {
	manifest := example.Manifest("missing-provider.o", "missing-consumer.o")
	manifest.Modules = append(manifest.Modules, module.Module{Name: "unused", Version: "1", Files: []string{"also-missing.o"}})
	if err := manifest.Validate(); err != nil {
		t.Fatal("validation must not read native files", err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := module.Read(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(got, manifest) {
		t.Fatal(got, err)
	}
	for name, patch := range map[string]func(*module.Manifest){
		"format":           func(m *module.Manifest) { m.Format++ },
		"empty":            func(m *module.Manifest) { m.Modules = nil },
		"roots":            func(m *module.Manifest) { m.Roots = nil },
		"missing_root":     func(m *module.Manifest) { m.Roots = []string{"other"} },
		"duplicate_root":   func(m *module.Manifest) { m.Roots = []string{"app", "app"} },
		"duplicate_module": func(m *module.Manifest) { m.Modules[1].Name = "app" },
		"invalid_name":     func(m *module.Manifest) { m.Modules[1].Name = "bad\x00name" },
		"empty_files":      func(m *module.Manifest) { m.Modules[1].Files = nil },
		"nul_path":         func(m *module.Manifest) { m.Modules[1].Files[0] = "bad\x00path" },
		"duplicate_export": func(m *module.Manifest) { m.Modules[1].Exports = []string{"module_add", "module_add"} },
		"invalid_export":   func(m *module.Manifest) { m.Modules[1].Exports[0] = "bad\x00name" },
		"missing_provider": func(m *module.Manifest) { m.Modules[0].Imports[0].Provider = "other" },
		"wrong_version":    func(m *module.Manifest) { m.Modules[0].Imports[0].Version = "math/2" },
		"empty_version":    func(m *module.Manifest) { m.Modules[1].Version = "" },
		"missing_export":   func(m *module.Manifest) { m.Modules[0].Imports[0].Symbol = "other" },
		"duplicate_alias":  func(m *module.Manifest) { m.Modules[0].Imports[0].As = "remote_bias" },
		"invalid_alias":    func(m *module.Manifest) { m.Modules[0].Imports[0].As = "bad\x00alias" },
		"cycle": func(m *module.Manifest) {
			m.Modules[1].Imports = []module.Import{{Provider: "app", Version: "app/1", Symbol: "module_run"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := example.Manifest("missing-provider.o", "missing-consumer.o")
			patch(&m)
			if err := m.Validate(); err == nil {
				t.Fatal("invalid manifest accepted")
			}
			if graph, err := module.Open(t.TempDir(), m, dylib.Options{}); err == nil || graph != nil {
				t.Fatal("invalid graph opened", graph, err)
			}
		})
	}
	manifest.Modules = append(manifest.Modules, module.Module{Name: "unused-cycle", Version: "1", Files: []string{"missing.o"}, Exports: []string{"f"}, Imports: []module.Import{{Provider: "unused-cycle", Version: "1", Symbol: "f"}}})
	if err := manifest.Validate(); err == nil {
		t.Fatal("an unused cycle was accepted")
	}
}

func TestManifestJSONBoundary(t *testing.T) {
	for _, input := range []string{"", "null", `{}`, `{"format":1,"unexpected":true}`, `{ "format":1 } {}`, strings.Repeat(" ", (1<<20)+1)} {
		if _, err := module.Read(strings.NewReader(input)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	if _, err := module.Read(nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	m := example.Manifest("provider.o", "consumer.o")
	data, _ := json.Marshal(m)
	if _, err := module.Read(strings.NewReader(string(data) + " true")); err == nil {
		t.Fatal("trailing document accepted")
	}
	if _, err := module.Read(strings.NewReader(string(data) + " !")); err == nil {
		t.Fatal("trailing garbage accepted")
	}
	if err := (*module.Graph)(nil).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := (*module.Graph)(nil).Session("app"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	var graph module.Graph
	if _, err := graph.Session("absent"); err == nil {
		t.Fatal("unknown session accepted")
	}
	if err := graph.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Session("app"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	if err := graph.Close(); err != nil {
		t.Fatal(err)
	}
}

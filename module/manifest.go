// Package module loads an explicit, versioned dependency graph using retained
// dylib sessions. It does not discover ABI signatures or search for providers.
package module

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const FormatVersion = 1
const maxManifestSize = 1 << 20

// Manifest selects roots and their transitive providers. Only that reachable
// subgraph is loaded. Validation checks all definitions, including unused ones.
type Manifest struct {
	Format  int      `json:"format"`
	Roots   []string `json:"roots"`
	Modules []Module `json:"modules"`
}

// Module names native inputs and an explicit export contract. Version is an
// opaque, exact-match ABI version declared by the manifest, not inferred from
// binary bytes. Files are resolved relative to Open's base directory (or the
// JSON manifest's directory for Load); absolute paths are accepted as supplied.
type Module struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Files   []string `json:"files"`
	Exports []string `json:"exports,omitempty"`
	Imports []Import `json:"imports,omitempty"`
}

// Import selects one provider export and its exact required version. As defaults
// to Symbol and selects the consumer's linkage name, without ABI adaptation.
type Import struct {
	Provider string `json:"provider"`
	Version  string `json:"version"`
	Symbol   string `json:"symbol"`
	As       string `json:"as,omitempty"`
}

func (i Import) name() string {
	if i.As != "" {
		return i.As
	}
	return i.Symbol
}

func textName(s string) bool     { return s != "" && !strings.ContainsRune(s, 0) }
func contractName(s string) bool { return textName(s) && strings.TrimSpace(s) == s }

// Read decodes one JSON document of at most 1 MiB and validates its dependency
// contracts. Unknown fields and trailing JSON documents are rejected. It reads
// no native files and invokes no native code. The reader remains caller-owned.
func Read(reader io.Reader) (Manifest, error) {
	var manifest Manifest
	if reader == nil {
		return manifest, fmt.Errorf("module: nil manifest reader")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maxManifestSize {
		return manifest, fmt.Errorf("module: manifest exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("module: decode manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Manifest{}, fmt.Errorf("module: expected one JSON document")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Validate checks structural contracts, provider versions, declared exports,
// duplicate names and dependency cycles before native loading. Actual binary
// exports, host compatibility and core definition rules are checked on loading.
func (m Manifest) Validate() error {
	_, err := m.order()
	return err
}

func (m Manifest) order() ([]int, error) {
	if m.Format != FormatVersion || len(m.Modules) == 0 || len(m.Roots) == 0 {
		return nil, fmt.Errorf("module: expected format %d, modules and roots", FormatVersion)
	}
	indices := make(map[string]int)
	exports := make(map[string]map[string]bool)
	for index, mod := range m.Modules {
		if !contractName(mod.Name) || !contractName(mod.Version) || len(mod.Files) == 0 {
			return nil, fmt.Errorf("module: invalid name/version or empty files for %q", mod.Name)
		}
		if _, exists := indices[mod.Name]; exists {
			return nil, fmt.Errorf("module: duplicate module %q", mod.Name)
		}
		indices[mod.Name] = index
		exports[mod.Name] = make(map[string]bool)
		for _, path := range mod.Files {
			if !textName(path) {
				return nil, fmt.Errorf("module: invalid input path for %q", mod.Name)
			}
		}
		for _, symbol := range mod.Exports {
			if !textName(symbol) || exports[mod.Name][symbol] {
				return nil, fmt.Errorf("module: invalid or duplicate export %q in %q", symbol, mod.Name)
			}
			exports[mod.Name][symbol] = true
		}
	}
	for _, mod := range m.Modules {
		aliases := make(map[string]bool)
		for _, imp := range mod.Imports {
			provider, ok := indices[imp.Provider]
			if !ok || !contractName(imp.Version) || imp.Version != m.Modules[provider].Version {
				return nil, fmt.Errorf("module: %q requires provider %q version %q", mod.Name, imp.Provider, imp.Version)
			}
			if !exports[imp.Provider][imp.Symbol] {
				return nil, fmt.Errorf("module: provider %q does not declare export %q", imp.Provider, imp.Symbol)
			}
			if !textName(imp.name()) || aliases[imp.name()] {
				return nil, fmt.Errorf("module: invalid or duplicate import name %q in %q", imp.name(), mod.Name)
			}
			aliases[imp.name()] = true
		}
	}
	states := make([]uint8, len(m.Modules))
	var result []int
	var visit func(int) error
	visit = func(index int) error {
		if states[index] == 1 {
			return fmt.Errorf("module: dependency cycle at %q", m.Modules[index].Name)
		}
		if states[index] == 2 {
			return nil
		}
		states[index] = 1
		for _, imp := range m.Modules[index].Imports {
			if err := visit(indices[imp.Provider]); err != nil {
				return err
			}
		}
		states[index] = 2
		result = append(result, index)
		return nil
	}
	seen := make(map[string]bool)
	for _, root := range m.Roots {
		index, ok := indices[root]
		if !ok || seen[root] {
			return nil, fmt.Errorf("module: missing or duplicate root %q", root)
		}
		seen[root] = true
		if err := visit(index); err != nil {
			return nil, err
		}
	}
	selected := append([]int(nil), result...)
	// Reject invalid unused subgraphs without opening their input paths.
	for index := range m.Modules {
		if err := visit(index); err != nil {
			return nil, err
		}
	}
	return selected, nil
}

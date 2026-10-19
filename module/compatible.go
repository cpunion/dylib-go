package module

import (
	"context"
	"fmt"
)

// CompatibleRequirement lists acceptable opaque ABI versions in preference
// order. Acceptance is declared by the caller, not inferred from version text
// or native binaries. Imports inside each catalog version remain exact.
type CompatibleRequirement struct {
	Name     string
	Versions []string
}

const maxVersionChoices = 4096

// DiscoverCompatible selects the first complete, valid graph in root/version
// preference order. It can reconsider an earlier root when a later root needs
// a conflicting provider. Each module name has one selected version, and a root
// reached as a dependency must use one of that root's acceptable versions.
//
// It uses Discover's catalog limits and metadata-only filesystem behavior, with
// at most 4096 declared alternatives and 4096 candidate attempts. Exhausting the
// search limit returns an error rather than an incomplete or unvalidated graph.
// There is no numeric ordering, range syntax, implicit ABI compatibility or
// fallback after native loading fails. The returned manifest pins exact versions.
func DiscoverCompatible(ctx context.Context, roots []CompatibleRequirement, directories ...string) (Manifest, error) {
	if err := discoveryInputs(ctx, len(roots), len(directories)); err != nil {
		return Manifest{}, err
	}
	accepted := make(map[string]map[string]bool)
	choices := 0
	for _, root := range roots {
		if !contractName(root.Name) || len(root.Versions) == 0 || accepted[root.Name] != nil {
			return Manifest{}, fmt.Errorf("module: invalid or duplicate compatible root %q", root.Name)
		}
		versions := make(map[string]bool)
		for _, version := range root.Versions {
			choices++
			if choices > maxVersionChoices {
				return Manifest{}, fmt.Errorf("module: discovery exceeds 4096 version alternatives")
			}
			if !contractName(version) || versions[version] {
				return Manifest{}, fmt.Errorf("module: invalid or duplicate acceptable version for %q", root.Name)
			}
			versions[version] = true
		}
		accepted[root.Name] = versions
	}
	available, err := discoverCatalogs(ctx, directories)
	if err != nil {
		return Manifest{}, err
	}
	return selectCompatible(ctx, roots, accepted, available)
}

func selectCompatible(ctx context.Context, roots []CompatibleRequirement, accepted map[string]map[string]bool, available map[Requirement]Module) (Manifest, error) {
	attempts := 0
	var last error
	var search func(int, Manifest, map[string]string) (Manifest, bool, error)
	search = func(index int, manifest Manifest, selected map[string]string) (Manifest, bool, error) {
		if err := ctx.Err(); err != nil {
			return Manifest{}, false, err
		}
		if index == len(roots) {
			return manifest, true, nil
		}
		root := roots[index]
		for _, version := range root.Versions {
			if err := ctx.Err(); err != nil {
				return Manifest{}, false, err
			}
			attempts++
			if attempts > maxVersionChoices {
				return Manifest{}, false, fmt.Errorf("module: discovery exceeds 4096 version candidate attempts")
			}
			// Branches own their selections and slices. Failed candidates cannot
			// leave dependencies in another candidate's graph.
			next := Manifest{Format: FormatVersion, Roots: append([]string(nil), manifest.Roots...), Modules: append([]Module(nil), manifest.Modules...)}
			versions := make(map[string]string, len(selected))
			for name, v := range selected {
				versions[name] = v
			}
			var visit func(Requirement) error
			visit = func(required Requirement) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if allowed := accepted[required.Name]; allowed != nil && !allowed[required.Version] {
					return fmt.Errorf("module: provider %q version %q is not acceptable to its root", required.Name, required.Version)
				}
				if v, ok := versions[required.Name]; ok {
					if v != required.Version {
						return fmt.Errorf("module: conflicting requested versions for %q: %q and %q", required.Name, v, required.Version)
					}
					return nil
				}
				mod, ok := available[required]
				if !ok {
					return fmt.Errorf("module: missing provider %q version %q", required.Name, required.Version)
				}
				versions[required.Name] = required.Version
				next.Modules = append(next.Modules, mod)
				for _, imp := range mod.Imports {
					if err := visit(Requirement{imp.Provider, imp.Version}); err != nil {
						return err
					}
				}
				return nil
			}
			if err := visit(Requirement{root.Name, version}); err != nil {
				last = err
				continue
			}
			next.Roots = append(next.Roots, root.Name)
			if err := next.Validate(); err != nil {
				last = err
				continue
			}
			if found, ok, err := search(index+1, next, versions); ok || err != nil {
				return found, ok, err
			}
		}
		return Manifest{}, false, nil
	}
	manifest, ok, err := search(0, Manifest{Format: FormatVersion}, nil)
	if err != nil {
		return Manifest{}, err
	}
	if !ok {
		return Manifest{}, fmt.Errorf("module: no compatible provider graph: %w", last)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

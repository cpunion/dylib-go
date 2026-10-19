package module

import (
	"context"
	"fmt"
	"sort"
)

// VersionRangeRequirement declares acceptable root ABI versions using the
// half-open SemVer 2.0.0 interval [AtLeast, Before). Both bounds are required,
// use complete MAJOR.MINOR.PATCH versions without a v prefix, and may contain
// prerelease/build identifiers. Each version is limited to 256 ASCII bytes.
// Prerelease candidates are excluded unless IncludePrerelease is true.
// Compatibility is declared by the caller; version text cannot prove it.
type VersionRangeRequirement struct {
	Name, AtLeast, Before string
	IncludePrerelease     bool
}

// DiscoverVersionRanges reads Discover's catalogs once, expanding each root
// interval into its available strict SemVer versions, newest precedence first.
// Equal precedence uses ascending literal version text as a stable tie-break;
// build metadata does not affect precedence. Opaque or overlong catalog versions
// do not match. Other catalog validation, limits and cancellation remain intact.
//
// The existing bounded compatible-graph selector can backtrack across roots.
// Imports remain exact, including a dependency that is also a root, and the
// returned manifest pins exact versions. Native inputs are never inspected or
// loaded here. A subsequent native loading failure does not select a fallback.
func DiscoverVersionRanges(ctx context.Context, roots []VersionRangeRequirement, directories ...string) (Manifest, error) {
	if err := discoveryInputs(ctx, len(roots), len(directories)); err != nil {
		return Manifest{}, err
	}
	type interval struct{ low, high semanticVersion }
	intervals := make([]interval, len(roots))
	seen := make(map[string]bool)
	for i, root := range roots {
		low, lo := parseSemanticVersion(root.AtLeast)
		high, hi := parseSemanticVersion(root.Before)
		if !contractName(root.Name) || seen[root.Name] || !lo || !hi || compareSemanticVersions(low, high) >= 0 {
			return Manifest{}, fmt.Errorf("module: invalid or duplicate version range for %q", root.Name)
		}
		seen[root.Name] = true
		intervals[i] = interval{low, high}
	}
	available, err := discoverCatalogs(ctx, directories)
	if err != nil {
		return Manifest{}, err
	}
	type candidate struct {
		literal string
		version semanticVersion
	}
	choices := make([]CompatibleRequirement, len(roots))
	accepted := make(map[string]map[string]bool)
	for i, root := range roots {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		var candidates []candidate
		for key := range available {
			if err := ctx.Err(); err != nil {
				return Manifest{}, err
			}
			if key.Name != root.Name {
				continue
			}
			v, valid := parseSemanticVersion(key.Version)
			if !valid || len(v.pre) != 0 && !root.IncludePrerelease || compareSemanticVersions(v, intervals[i].low) < 0 || compareSemanticVersions(v, intervals[i].high) >= 0 {
				continue
			}
			candidates = append(candidates, candidate{key.Version, v})
		}
		if len(candidates) == 0 {
			return Manifest{}, fmt.Errorf("module: no provider for %q in declared version range", root.Name)
		}
		sort.Slice(candidates, func(a, b int) bool {
			if order := compareSemanticVersions(candidates[a].version, candidates[b].version); order != 0 {
				return order > 0
			}
			return candidates[a].literal < candidates[b].literal
		})
		choice := CompatibleRequirement{Name: root.Name}
		versions := make(map[string]bool)
		for _, c := range candidates {
			choice.Versions = append(choice.Versions, c.literal)
			versions[c.literal] = true
		}
		choices[i], accepted[root.Name] = choice, versions
	}
	// Distinct roots partition the at-most-256 catalog declarations, so expansion
	// cannot exceed the selector's 4096-alternative budget.
	return selectCompatible(ctx, choices, accepted, available)
}

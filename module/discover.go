package module

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Requirement selects one exact, case-sensitive name and opaque ABI version.
// Versions are declared contracts, not semantic-version ranges or ABI probes.
type Requirement struct {
	Name, Version string
}

// Catalog is a JSON declaration document discovered in a *.dylib.json file.
// Imports may refer to declarations in other catalogs. Multiple versions of a
// name may be available, but a selected graph can use only one version per name.
// Files are relative to this catalog's directory. No native input is read here.
type Catalog struct {
	Format  int      `json:"format"`
	Modules []Module `json:"modules"`
}

const (
	maxCatalogDirectories = 16
	maxCatalogEntries     = 4096
	maxCatalogFiles       = 256
	maxCatalogModules     = 256
	maxCatalogInputs      = 4096
	maxCatalogBytes       = 8 << 20
)

// Discover scans 1-16 explicit directories, without recursion, for regular
// *.dylib.json files. It reads at most 4096 directory entries, 256 catalogs,
// 256 module declarations, 4096 native input paths and 8 MiB total JSON
// (1 MiB per file). Directory order and sorted filenames determine stable
// diagnostics and traversal.
// Duplicate name/version definitions are ambiguous and rejected. All local
// declarations are validated; only roots and reachable dependencies enter the
// returned complete manifest, whose native file paths are absolute.
//
// Discover neither opens native inputs nor executes code. Pass its result to
// Open to load it. Native files are opened later and are not snapshotted here.
// Cancellation is checked between filesystem operations, reads and graph visits;
// an in-progress operating-system filesystem operation cannot be interrupted.
func Discover(ctx context.Context, roots []Requirement, directories ...string) (Manifest, error) {
	if err := discoveryInputs(ctx, len(roots), len(directories)); err != nil {
		return Manifest{}, err
	}
	seenRoots := make(map[string]bool)
	for _, root := range roots {
		if !contractName(root.Name) || !contractName(root.Version) || seenRoots[root.Name] {
			return Manifest{}, fmt.Errorf("module: invalid or duplicate discovery root %q", root.Name)
		}
		seenRoots[root.Name] = true
	}
	available, err := discoverCatalogs(ctx, directories)
	if err != nil {
		return Manifest{}, err
	}
	candidates := make([]CompatibleRequirement, len(roots))
	for i, root := range roots {
		candidates[i] = CompatibleRequirement{Name: root.Name, Versions: []string{root.Version}}
	}
	return selectCompatible(ctx, candidates, nil, available)
}

func discoveryInputs(ctx context.Context, roots, directories int) error {
	if ctx == nil {
		return fmt.Errorf("module: nil discovery context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if roots == 0 || roots > maxCatalogModules || directories == 0 || directories > maxCatalogDirectories {
		return fmt.Errorf("module: discovery requires 1-256 roots and 1-16 directories")
	}
	return nil
}

// discoverCatalogs reads metadata once for all discovery policies.
func discoverCatalogs(ctx context.Context, directories []string) (map[Requirement]Module, error) {
	var paths []string
	seenDirs := make(map[string]bool)
	entries := 0
	for _, dir := range directories {
		if !textName(dir) {
			return nil, fmt.Errorf("module: invalid catalog directory")
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if seenDirs[abs] {
			continue
		}
		seenDirs[abs] = true
		files, err := catalogPaths(ctx, abs, &entries)
		if err != nil {
			return nil, err
		}
		paths = append(paths, files...)
		if len(paths) > maxCatalogFiles {
			return nil, fmt.Errorf("module: discovery exceeds 256 catalogs")
		}
	}
	available := make(map[Requirement]Module)
	totalBytes := 0
	inputs := 0
	for _, path := range paths {
		catalog, n, err := readCatalog(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("module: catalog %s: %w", path, err)
		}
		totalBytes += n
		if totalBytes > maxCatalogBytes {
			return nil, fmt.Errorf("module: discovery exceeds 8 MiB of catalog JSON")
		}
		if catalog.Format != FormatVersion || len(catalog.Modules) == 0 {
			return nil, fmt.Errorf("module: catalog %s requires format %d and modules", path, FormatVersion)
		}
		for _, mod := range catalog.Modules {
			inputs += len(mod.Files)
			if inputs > maxCatalogInputs {
				return nil, fmt.Errorf("module: discovery exceeds 4096 native input paths")
			}
			if _, err := mod.validate(); err != nil {
				return nil, fmt.Errorf("module: catalog %s: %w", path, err)
			}
			key := Requirement{mod.Name, mod.Version}
			if _, exists := available[key]; exists {
				return nil, fmt.Errorf("module: ambiguous provider %q version %q", mod.Name, mod.Version)
			}
			for i, file := range mod.Files {
				if !filepath.IsAbs(file) {
					file = filepath.Join(filepath.Dir(path), file)
				}
				mod.Files[i] = filepath.Clean(file)
			}
			available[key] = mod
			if len(available) > maxCatalogModules {
				return nil, fmt.Errorf("module: discovery exceeds 256 module declarations")
			}
		}
	}
	return available, nil
}

func catalogPaths(ctx context.Context, directory string, count *int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	var paths []string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := dir.ReadDir(64)
		*count += len(batch)
		if *count > maxCatalogEntries {
			return nil, fmt.Errorf("module: discovery exceeds 4096 directory entries")
		}
		for _, entry := range batch {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dylib.json") {
				continue
			}
			paths = append(paths, filepath.Join(directory, entry.Name()))
			if len(paths) > maxCatalogFiles {
				return nil, fmt.Errorf("module: discovery exceeds 256 catalogs")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("module: catalog %s must be a regular file", path)
		}
	}
	return paths, nil
}

type discoveryReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r discoveryReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
func readCatalog(ctx context.Context, path string) (Catalog, int, error) {
	var catalog Catalog
	if err := ctx.Err(); err != nil {
		return catalog, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return catalog, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return catalog, 0, err
	}
	if !info.Mode().IsRegular() {
		return catalog, 0, fmt.Errorf("expected a regular catalog file")
	}
	n, err := readDocument(discoveryReader{ctx, file}, &catalog)
	return catalog, n, err
}

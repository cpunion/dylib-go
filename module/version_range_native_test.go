//go:build cgo && (linux || darwin || windows)

package module_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	example "github.com/cpunion/dylib-go/examples/modules"
	"github.com/cpunion/dylib-go/module"
)

func TestNativeVersionRangeModuleCalls(t *testing.T) {
	for _, kind := range []string{"object", "archive", "library"} {
		for _, opt := range []string{"-O0", "-O2"} {
			t.Run(kind+"/"+opt, func(t *testing.T) {
				dir := t.TempDir()
				old := produce(t, dir, "old", `API int module_add(int a,int b){return a+b+10;}`, kind, opt)
				current := produce(t, dir, "current", `API int module_add(int a,int b){return a+b;}`, kind, opt)
				future := produce(t, dir, "future", `API int module_add(int a,int b){return a+b+100;}`, kind, opt)
				makeModule := func(version, file string) module.Module {
					return module.Module{Name: "app", Version: version, Files: []string{filepath.Base(file)}, Exports: []string{"module_add"}}
				}
				mods := []module.Module{makeModule("1.0.0", old), makeModule("1.1.0", current), makeModule("1.2.0-rc.1", future), makeModule("2.0.0", future)}
				bad := makeModule("1.3.0", current)
				bad.Imports = []module.Import{{Provider: "missing", Version: "exact", Symbol: "absent"}}
				mods = append(mods, bad)
				writeCatalog(t, dir, "versions", mods...)
				for _, mode := range []string{"stable", "prerelease", "native_failure_no_fallback"} {
					t.Run(mode, func(t *testing.T) {
						want, version := int32(42), "1.1.0"
						var manifest module.Manifest
						var err error
						if mode == "native_failure_no_fallback" {
							writeCatalog(t, dir, "versions", append(mods, makeModule("1.4.0", "not-installed.o"))...)
							version = "1.4.0"
						}
						if mode == "prerelease" {
							want, version = 142, "1.2.0-rc.1"
							manifest, err = module.DiscoverVersionRanges(context.Background(), []module.VersionRangeRequirement{
								{Name: "app", AtLeast: "1.0.0", Before: "2.0.0", IncludePrerelease: true},
							}, dir)
						} else {
							manifest, err = example.DiscoverVersionRanges(context.Background(), dir)
						}
						if err != nil || len(manifest.Modules) != 1 || manifest.Modules[0].Version != version {
							t.Fatal(manifest, err)
						}
						graph, err := module.Open("", manifest, dylib.Options{})
						if mode == "native_failure_no_fallback" {
							if graph != nil {
								graph.Close()
								t.Fatal("native failure loaded a fallback graph")
							}
							if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "not-installed.o") {
								t.Fatal(err)
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() {
							if err := graph.Close(); err != nil {
								t.Error(err)
							}
						})
						session, err := graph.Session("app")
						if err != nil {
							t.Fatal(err)
						}
						lease, read := nativeRead(t, session, "module_add")
						for i := int32(0); i < 100; i++ {
							if got := read(20+i, 22); got != want+i {
								t.Fatal("wrong native provider selected", got, want+i)
							}
						}
						lease.Close()
						if err := graph.Close(); err != nil {
							t.Fatal(err)
						}
						if _, err := graph.Session("app"); !errors.Is(err, dylib.ErrClosed) {
							t.Fatal(err)
						}
					})
				}
			})
		}
	}
}

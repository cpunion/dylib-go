//go:build linux || darwin

package dylib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	examplecall "github.com/cpunion/dylib-go/examples/call"
)

func testNativeCXXExceptions(t *testing.T) {
	needNative(t)
	cpp := os.Getenv("DYLIB_CXX")
	if cpp == "" {
		cpp = "clang++"
	}
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			dir := t.TempDir()
			flags := []string{"-std=c++17", optimization, "-fPIC", "-fexceptions", "-fno-stack-protector"}
			libraryName, libraryFlag := "cxx_host.so", "-shared"
			if runtime.GOOS == "linux" {
				flags = append(flags, "-stdlib=libstdc++", "-rtlib=libgcc", "-unwindlib=libgcc")
			} else {
				libraryName, libraryFlag = "cxx_host.dylib", "-dynamiclib"
				flags = append(flags, "-stdlib=libc++", "-femit-dwarf-unwind=always")
				if runtime.GOARCH == "amd64" {
					flags = append(flags, "--target=x86_64-apple-macosx11")
				}
			}
			if runtime.GOARCH == "386" {
				flags = append(flags, "-m32")
			}
			host := filepath.Join(dir, libraryName)
			command(t, cpp, append(append([]string(nil), flags...), libraryFlag, "testdata/unwind_cxx_host.cpp", "-o", host)...)
			var objects []string
			for _, kind := range []string{"throw", "catch"} {
				path := filepath.Join(dir, kind+".o")
				command(t, cpp, append(append([]string(nil), flags...), "-c", "testdata/unwind_cxx_"+kind+".cpp", "-o", path)...)
				if runtime.GOOS == "darwin" {
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("CXX diagnostic object %s: %x", kind, data)
				}
				objects = append(objects, path)
			}
			observer := New(Options{})
			defer observer.Close()
			load(t, observer, host)
			for _, input := range []string{"objects", "reversed_objects", "archive"} {
				t.Run(input, func(t *testing.T) {
					paths := append([]string(nil), objects...)
					if input == "reversed_objects" {
						paths[0], paths[1] = paths[1], paths[0]
					}
					if input == "archive" {
						archive := filepath.Join(dir, "cxx.a")
						command(t, "ar", append([]string{"rcs", archive}, objects...)...)
						paths = []string{archive}
					}
					session := New(Options{RegisterUnwind: true})
					defer session.Close()
					load(t, session, append([]string{host}, paths...)...)
					if err := session.Link("missing_cxx_root"); err == nil {
						t.Fatal("missing root accepted")
					}
					for _, test := range []struct {
						name         string
						a, b, events int32
					}{
						{"integer_rethrow", 20, 22, 111}, {"double", -20, 62, 11}, {"library_throw", 99, 0, 10},
					} {
						t.Run(test.name, func(t *testing.T) {
							if runtime.GOOS == "darwin" && session.image == nil {
								if err := session.Link("cxx_catch"); err != nil {
									t.Fatal(err)
								}
								im := session.image
								for _, object := range im.objects {
									for _, section := range object.sections {
										if section != nil {
											t.Logf("CXX diagnostic section %s base=%x original=%x size=%x data=%x", section.name, im.base+uintptr(section.offset), section.original, section.size, im.mem[section.offset:section.offset+section.size])
										}
									}
								}
								t.Logf("CXX diagnostic GOT base=%x data=%x personality=%x int=%x double=%x", im.base+uintptr(im.gotStart), im.mem[im.gotStart:im.gotNext], session.external("__gxx_personality_v0"), session.external("_ZTIi"), session.external("_ZTId"))
							}
							call(t, observer, "cxx_events", 1, 0, 0)
							call(t, session, "cxx_catch", test.a, test.b, 42)
							call(t, observer, "cxx_events", 1, 0, test.events)
						})
					}
					call(t, observer, "cxx_has_frame", 0, 0, 1)
					// Native exceptions and frame registration remain valid while a
					// retained entry finishes work during session retirement.
					symbol, err := session.Resolve("cxx_catch")
					if err != nil {
						t.Fatal(err)
					}
					lease, err := symbol.Acquire()
					if err != nil {
						t.Fatal(err)
					}
					defer lease.Close()
					closed := make(chan error, 1)
					go func() { closed <- session.Close() }()
					waitSessionRetired(t, session)
					if err := lease.WithAddress(func(address uintptr) error {
						result, err := examplecall.Int32(address, 20, 22)
						if result != 42 {
							t.Errorf("retiring C++ call: %d", result)
						}
						return err
					}); err != nil {
						t.Fatal(err)
					}
					call(t, observer, "cxx_has_frame", 0, 0, 1)
					call(t, observer, "cxx_events", 1, 0, 111)
					if err := lease.Close(); err != nil {
						t.Fatal(err)
					}
					if err := <-closed; err != nil {
						t.Fatal(err)
					}
					call(t, observer, "cxx_has_frame", 0, 0, 0)
				})
			}
			plain := New(Options{})
			defer plain.Close()
			load(t, plain, append([]string{host}, objects...)...)
			call(t, plain, "cxx_plain", 20, 22, 42)
			if plain.image.unwind != nil {
				t.Fatal("default loading registered language frames")
			}
			for _, object := range plain.image.objects {
				for _, section := range object.sections {
					if section != nil && (elfUnwindSection(section.name) || section.name == "__eh_frame" || lsdaSection(object.info.Format, section.name)) {
						t.Fatal("default loading retained exception data")
					}
				}
			}
		})
	}
}

//go:build cgo && (linux || darwin || windows)

package module_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	dylib "github.com/cpunion/dylib-go"
	call "github.com/cpunion/dylib-go/examples/call"
	example "github.com/cpunion/dylib-go/examples/modules"
	"github.com/cpunion/dylib-go/module"
)

const exportMacro = `
#if defined(_WIN32)
#define API __declspec(dllexport)
#else
#define API __attribute__((visibility("default")))
#endif
`

func nativeTool(t *testing.T, name string) string {
	t.Helper()
	tool, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	return tool
}

func produce(t *testing.T, dir, name, code, kind, optimization string) string {
	t.Helper()
	compiler := os.Getenv("CLANG")
	if compiler == "" {
		compiler = "clang"
	}
	source, output := filepath.Join(dir, name+".c"), filepath.Join(dir, name+".o")
	if err := os.WriteFile(source, []byte(exportMacro+code), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{optimization, "-fno-stack-protector"}
	if runtime.GOARCH == "386" {
		args = append(args, "-m32")
	}
	if runtime.GOOS != "windows" {
		args = append(args, "-fPIC")
	}
	if kind == "library" {
		if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
			args = append(args, "--target=i686-w64-windows-gnu")
			if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
				args = append(args, "--sysroot="+root, "--rtlib=libgcc")
			}
		}
		flag, suffix := "-shared", ".so"
		if runtime.GOOS == "darwin" {
			flag, suffix = "-dynamiclib", ".dylib"
		}
		if runtime.GOOS == "windows" {
			suffix = ".dll"
		}
		output = filepath.Join(dir, name+suffix)
		args = append(args, flag)
	} else {
		args = append(args, "-c")
		if runtime.GOOS == "windows" {
			arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
			args = append(args, "--target="+arch+"-pc-windows-msvc", "-ffreestanding")
		}
	}
	args = append(args, source, "-o", output)
	if data, err := exec.Command(nativeTool(t, compiler), args...).CombinedOutput(); err != nil {
		t.Fatalf("native module producer: %v\n%s", err, data)
	}
	if kind == "archive" {
		archive := filepath.Join(dir, name+".a")
		if data, err := exec.Command(nativeTool(t, "ar"), "rcs", archive, output).CombinedOutput(); err != nil {
			t.Fatalf("module archive: %v\n%s", err, data)
		}
		output = archive
	}
	return output
}

func nativeRead(t *testing.T, session *dylib.Session, name string) (*dylib.SymbolLease, func(int32, int32) int32) {
	t.Helper()
	symbol, err := session.Resolve(name)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := symbol.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lease.Close() })
	return lease, func(a, b int32) int32 {
		t.Helper()
		var value int32
		if err := lease.WithAddress(func(address uintptr) error { var err error; value, err = call.Int32(address, a, b); return err }); err != nil {
			t.Fatal(err)
		}
		return value
	}
}

func TestNativeModuleGraphCalls(t *testing.T) {
	for _, kind := range []string{"object", "archive", "library"} {
		for _, optimization := range []string{"-O0", "-O2"} {
			t.Run(kind+"/"+optimization, func(t *testing.T) {
				dir := t.TempDir()
				provider := produce(t, dir, "provider", `API int module_bias=2; API int module_add(int a,int b){return a+b;}`, kind, optimization)
				if kind == "archive" {
					unused := produce(t, dir, "unused", `extern int absent(int,int); __attribute__((constructor)) static void bad(void){absent(0,0);} int unused(int a,int b){return absent(a,b);}`, "object", optimization)
					if data, err := exec.Command(nativeTool(t, "ar"), "rcs", provider, unused).CombinedOutput(); err != nil {
						t.Fatalf("unused archive member: %v\n%s", err, data)
					}
				}
				consumer := produce(t, dir, "consumer", `extern int remote_add(int,int); extern int remote_bias; API int module_run(int a,int b){return remote_add(a,b)+remote_bias;}`, "object", optimization)
				manifest := example.Manifest(filepath.Base(provider), filepath.Base(consumer))
				second := manifest.Modules[0]
				second.Name = "second"
				manifest.Roots = append(manifest.Roots, "second")
				manifest.Modules = append(manifest.Modules, second, module.Module{Name: "unused", Version: "1", Files: []string{"not-present.o"}})
				data, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "modules.json")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				graph, err := module.Load(path, dylib.Options{})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := graph.Close(); err != nil {
						t.Error(err)
					}
				})
				if _, err := graph.Session("unused"); err == nil {
					t.Fatal("unreachable module loaded")
				}
				for _, name := range []string{"app", "second"} {
					session, err := graph.Session(name)
					if err != nil {
						t.Fatal(err)
					}
					lease, read := nativeRead(t, session, "module_run")
					if got := read(20, 20); got != 42 {
						t.Fatal(name, got)
					}
					lease.Close()
				}
				if err := graph.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := graph.Session("app"); !errors.Is(err, dylib.ErrClosed) {
					t.Fatal(err)
				}
			})
		}
	}
}

func observerManifest(observer, provider, consumer string) module.Manifest {
	return module.Manifest{Format: module.FormatVersion, Roots: []string{"app"}, Modules: []module.Module{
		{Name: "app", Version: "1", Files: []string{consumer}, Exports: []string{"module_run"}, Imports: []module.Import{{Provider: "provider", Version: "1", Symbol: "module_add"}}},
		{Name: "provider", Version: "1", Files: []string{provider}, Exports: []string{"module_add"}, Imports: []module.Import{{Provider: "observer", Version: "1", Symbol: "module_event"}}},
		{Name: "observer", Version: "1", Files: []string{observer}, Exports: []string{"module_event", "module_count"}},
	}}
}

const observerCode = `static int count; API int module_event(int a,int b){count+=a+b;return count;} API int module_count(int a,int b){return count;}`
const providerCode = `extern int module_event(int,int); __attribute__((constructor)) static void start(void){module_event(1,0);} __attribute__((destructor)) static void stop(void){module_event(1,0);} API int module_add(int a,int b){return a+b;}`

func TestNativeModuleGraphRetirement(t *testing.T) {
	dir := t.TempDir()
	observer := produce(t, dir, "observer", observerCode, "object", "-O2")
	provider := produce(t, dir, "provider", providerCode, "archive", "-O2")
	consumer := produce(t, dir, "consumer", `extern int module_add(int,int); API int module_run(int a,int b){return module_add(a,b);}`, "object", "-O2")
	graph, err := module.Open(dir, observerManifest(observer, provider, consumer), dylib.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { graph.Close() })
	session, err := graph.Session("observer")
	if err != nil {
		t.Fatal(err)
	}
	lease, read := nativeRead(t, session, "module_count")
	if got := read(0, 0); got != 1 {
		t.Fatal("initializer order", got)
	}
	closed := make(chan error, 2)
	go func() { closed <- graph.Close() }()
	go func() { closed <- graph.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := session.Resolve("module_count"); errors.Is(err, dylib.ErrClosed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("observer did not retire after consumer cleanup")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := graph.Session("app"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("graph lookup accepted during retirement", err)
	}
	if got := read(0, 0); got != 2 {
		t.Fatal("provider finalizer lost its observer", got)
	}
	select {
	case err := <-closed:
		t.Fatal("graph closed over an external lease", err)
	default:
	}
	lease.Close()
	for range 2 {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent graph close did not finish")
		}
	}
}

func TestNativeModuleGraphRollback(t *testing.T) {
	dir := t.TempDir()
	observer := produce(t, dir, "observer", observerCode, "library", "-O2")
	external := dylib.New(dylib.Options{})
	if err := external.Load(observer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { external.Close() })
	lease, read := nativeRead(t, external, "module_count")
	defer lease.Close()
	provider := produce(t, dir, "provider", providerCode, "object", "-O2")
	consumer := produce(t, dir, "consumer", `extern int missing_native(int,int); API int module_run(int a,int b){return missing_native(a,b);}`, "object", "-O2")
	manifest := observerManifest(observer, provider, consumer)
	manifest.Modules[0].Imports[0].Version = "wrong"
	if graph, err := module.Open(dir, manifest, dylib.Options{}); graph != nil || err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatal("version preflight", graph, err)
	}
	if got := read(0, 0); got != 0 {
		t.Fatal("contract failure initialized native code", got)
	}
	manifest.Modules[0].Imports[0].Version = "1"
	if graph, err := module.Open(dir, manifest, dylib.Options{}); graph != nil || err == nil {
		t.Fatal("native link failure accepted", graph, err)
	}
	if got := read(0, 0); got != 2 {
		t.Fatal("rollback omitted initialization/finalization", got)
	}
	// Raw missing export roots fail before initialization, and release sessions.
	manifest.Modules[0].Files = []string{provider}
	manifest.Modules[0].Imports = manifest.Modules[1].Imports
	manifest.Modules[0].Exports = []string{"missing_export"}
	if graph, err := module.Open(dir, manifest, dylib.Options{}); graph != nil || err == nil {
		t.Fatal("stale export contract accepted", graph, err)
	}
	if got := read(0, 0); got != 2 {
		t.Fatal("stale raw export contract ran initialization", got)
	}
	for index, tc := range []struct {
		name  string
		patch func(*module.Manifest)
	}{
		{"reserved_import", func(m *module.Manifest) { m.Modules[0].Imports[0].As = "atexit" }},
		{"missing_input", func(m *module.Manifest) { m.Modules[0].Files = []string{"missing-consumer.o"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := observerManifest(observer, provider, consumer)
			tc.patch(&m)
			if graph, err := module.Open(dir, m, dylib.Options{}); graph != nil || err == nil {
				t.Fatal("invalid native graph opened", graph, err)
			}
			if got := read(0, 0); got != int32(4+index*2) {
				t.Fatal("consumer setup failure leaked provider cleanup", got)
			}
		})
	}
}

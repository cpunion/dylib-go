package dylib

import (
	"path/filepath"
	"testing"
)

// Windows i386 mixes compiler thiscall destructors and manual cdecl callbacks
// behind the same C registration ABI. Other hosts exercise ordinary C entries.
func TestNativeMixedDestructorConventions(t *testing.T) {
	needNative(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			object := compile(t, "testdata/cxa_mixed_abi.c", filepath.Join(dir, "mixed.o"), opt)
			archive := filepath.Join(dir, "mixed.a")
			command(t, "ar", "rcs", archive, object)
			for _, kind := range []string{"object", "archive"} {
				t.Run(kind, func(t *testing.T) {
					host := nativeThreadLibrary(t, "testdata/emutls_dtor_host.c")
					s := New(Options{})
					defer s.Close()
					event, err := host.Resolve("record_event")
					if err != nil {
						t.Fatal(err)
					}
					if err := s.DefineSymbol("record_event", event); err != nil {
						t.Fatal(err)
					}
					input := object
					if kind == "archive" {
						input = archive
					}
					load(t, s, input)
					call(t, s, "mixed_ready", 20, 22, 42)
					call(t, host, "recorded_event", -1, 0, 0)
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					expectEvents(t, func(a, b int32) (int32, error) { return callInt32(host, "recorded_event", a, b) }, 33, 22, 11)
				})
			}
		})
	}
}

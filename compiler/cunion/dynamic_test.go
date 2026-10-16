//go:build libffi && cgo && (linux || darwin || windows)

package cunion_test

import (
	"errors"
	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNativeDynamicUnionFacade(t *testing.T) {
	facade, src, metadata := render(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			for kind, path := range producer(t, src, opt) {
				t.Run(kind, func(t *testing.T) {
					session := dylib.New(dylib.Options{})
					t.Cleanup(func() {
						if err := session.Close(); err != nil {
							t.Error(err)
						}
					})
					for _, file := range strings.Split(path, "\n") {
						if err := session.Load(file); err != nil {
							t.Fatal(err)
						}
					}
					entries := map[string]*dylib.Function{}
					for _, name := range facade.Functions {
						d, err := metadata.ForHost(name)
						if err != nil {
							t.Fatal(err)
						}
						entry, err := session.Bind(d.Symbol, d.Signature)
						if err != nil {
							t.Fatal(err)
						}
						entries[name] = entry
					}
					call := func(t *testing.T, name string, want abi.Value, args ...abi.Value) {
						t.Helper()
						got, err := entries[name].Call(args...)
						if err != nil || !reflect.DeepEqual(got, want) {
							t.Fatalf("%s: %v, %v; want %v", name, got, err, want)
						}
					}
					call(t, "union_add", abi.Int32(42), abi.Int32(20), abi.Int32(22))
					for _, test := range []struct {
						name  string
						value abi.Value
					}{
						{"i8", abi.Int8(-128)}, {"u8", abi.Uint8(255)}, {"i16", abi.Int16(-32768)}, {"u16", abi.Uint16(65535)}, {"i32", abi.Int32(-2147483648)}, {"u32", abi.Uint32(4294967295)}, {"i64", abi.Int64(-9223372036854775808)}, {"u64", abi.Uint64(18446744073709551615)}, {"f32", abi.Float32(-42.5)}, {"double", abi.Float64(42.5)}, {"double", abi.Float64(math.Float64frombits(0x7ff8000000000042))}, {"double", abi.Float64(math.Copysign(0, -1))}, {"bool", abi.Boolean(true)}, {"bool", abi.Boolean(false)}, {"pointer", abi.Ptr(0)},
					} {
						t.Run(test.name, func(t *testing.T) { call(t, "union_"+test.name, test.value, test.value) })
					}
					call(t, "union_small", abi.Float32(42), abi.Float32(20), abi.Float32(22))
					call(t, "union_large", abi.Int32(42), abi.Int32(20), abi.Int32(22))
					call(t, "union_packed", abi.Float64(42), abi.Float64(20.5), abi.Float64(21.5))
					call(t, "union_aligned", abi.Int32(42), abi.Int32(20), abi.Int32(22))
					call(t, "union_store", abi.Value{Type: abi.Void}, abi.Int32(42))
					call(t, "union_stored", abi.Int32(42))
					call(t, "union_read", abi.Int32(42), abi.Int32(42))
					address, err := entries["union_address"].Call()
					if err != nil || address.Type != abi.Pointer || address.Bits == 0 {
						t.Fatal(address, err)
					}
					call(t, "union_pointer", address, address)
					call(t, "union_pointer_read", abi.Int32(42), address)
					t.Run("record_views", func(t *testing.T) {
						pair, err := entries["union_pair_make"].Call(abi.Int32(20), abi.Int32(22))
						if err != nil {
							t.Fatal(err)
						}
						zero, err := entries["union_pair_make"].Call(abi.Int32(0), abi.Int32(0))
						if err != nil {
							t.Fatal(err)
						}
						call(t, "union_pair", pair, pair, zero)
						bundle, err := entries["union_bundle_make"].Call(abi.Int32(20), abi.Int32(22))
						if err != nil {
							t.Fatal(err)
						}
						call(t, "union_bundle", bundle, bundle)
						hfa, err := entries["union_hfa_make"].Call(abi.Float64(20.5), abi.Float64(21.5))
						if err != nil {
							t.Fatal(err)
						}
						call(t, "union_hfa", hfa, hfa)
						fields := bundle.Aggregate.Fields
						if fields[0].Aggregate.Fields[0] != abi.Int32(20) || fields[1].Aggregate.Fields[1].Aggregate.Fields[0] != abi.Int32(22) || fields[2].Aggregate.Fields[1] != abi.Float64(21.5) || fields[3] != address {
							t.Fatal("nested record factory", bundle)
						}
					})
					// A pointer projection remains borrowed from this provider. A retained code
					// entry keeps the native global alive while the session retires.
					lease, err := entries["union_pointer_read"].Acquire()
					if err != nil {
						t.Fatal(err)
					}

					t.Cleanup(func() { lease.Close() })
					closed := make(chan error, 1)
					go func() { closed <- session.Close() }()
					deadline := time.Now().Add(5 * time.Second)
					for {
						if _, err := session.Resolve("union_pointer_read"); errors.Is(err, dylib.ErrClosed) {
							break
						}
						if time.Now().After(deadline) {
							t.Fatal("provider did not retire")
						}
						time.Sleep(time.Millisecond)
					}
					select {
					case err := <-closed:
						t.Fatal("pointer provider closed before lease release", err)
					default:
					}
					got, err := lease.Call(address)
					if err != nil || got != abi.Int32(42) {
						t.Fatal("retiring pointer view", got, err)
					}
					if _, err := entries["union_pointer_read"].Call(address); !errors.Is(err, dylib.ErrClosed) {
						t.Fatal("closed provider", err)
					}
					if err := lease.Close(); err != nil {
						t.Fatal(err)
					}
					select {
					case err := <-closed:
						if err != nil {
							t.Fatal(err)
						}
					case <-time.After(5 * time.Second):
						t.Fatal("provider did not close")
					}

				})
			}
		})
	}
}

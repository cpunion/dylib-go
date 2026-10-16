//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
	cxxexample "github.com/cpunion/dylib-go/examples/cxx"
)

func TestNativeDynamicDiscoveredBaseFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/bases.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := cxxexample.Discover(context.Background(), filepath.ToSlash(header), cxx.DiscoveryOptions{Compiler: cppTool(t), Flags: cppTargetFlags()})
	if err != nil {
		t.Fatal(err)
	}
	source, metadata := prepare(t, facade)
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			library := shared(t, source, optimization)
			session := dylib.New(dylib.Options{})
			t.Cleanup(func() { session.Close() })
			if err := session.Load(library); err != nil {
				t.Fatal(err)
			}
			functions := map[string]*dylib.Function{}
			for _, name := range facade.Functions {
				declaration, err := metadata.ForHost(name)
				if err != nil {
					t.Fatal(err)
				}
				functions[name], err = session.Bind(name, declaration.Signature)
				if err != nil {
					t.Fatal(err)
				}
			}
			output := func(name string, initial abi.Value, args ...abi.Value) abi.Value {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: initial.Type}, initial)
				if err != nil {
					t.Fatal(err)
				}
				defer v.Close()
				err = v.WithAddress(func(address uintptr) error {
					status, err := functions[name].Call(append(args, abi.Ptr(address))...)
					if err == nil && status != abi.Int32(cxx.Success) {
						return fmt.Errorf("%s status: %v", name, status)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				got, err := v.Read()
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			for _, kind := range []string{"derived", "diamond"} {
				t.Run(kind, func(t *testing.T) {
					owner := output("discovered_"+kind+"_new", abi.Ptr(0), abi.Int32(20))
					defer func() {
						if got := output("discovered_"+kind+"_delete", owner); got != abi.Ptr(0) {
							t.Error("owner was not consumed", got)
						}
					}()
					view := output("discovered_"+kind+"_counter", abi.Ptr(0), owner)
					if view.Bits == 0 || view == owner {
						t.Fatal("missing native base adjustment", owner, view)
					}
					if got := output("discovered_score", abi.Int32(0), view, abi.Int32(20)); got != abi.Int32(42) {
						t.Fatal("native virtual dispatch", got)
					}
					if kind == "diamond" {
						left := output("discovered_diamond_virtualleft", abi.Ptr(0), owner)
						if indirect := output("discovered_virtualleft_counter", abi.Ptr(0), left); indirect != view {
							t.Fatal("discovered paths disagree on shared virtual base", view, indirect)
						}
					}
				})
			}
		})
	}
}

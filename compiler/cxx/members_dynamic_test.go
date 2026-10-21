//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	dylib "github.com/ramblelabs/dylib-go"
	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
	cxxexample "github.com/ramblelabs/dylib-go/examples/cxx"
)

func TestNativeDynamicDiscoveredMemberFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/members.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := cxxexample.DiscoverMembers(context.Background(), filepath.ToSlash(header), cxx.DiscoveryOptions{Compiler: cppTool(t), Flags: cppTargetFlags()})
	if err != nil {
		t.Fatal(err)
	}
	source, metadata := prepare(t, facade)
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			library := shared(t, source, optimization)
			session := dylib.New(dylib.Options{})
			t.Cleanup(func() {
				if err := session.Close(); err != nil {
					t.Error(err)
				}
			})
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
				value, err := abi.NewNativeValue(abi.TypeDesc{Type: initial.Type}, initial)
				if err != nil {
					t.Fatal(err)
				}
				defer value.Close()
				err = value.WithAddress(func(address uintptr) error {
					got, err := functions[name].Call(append(args, abi.Ptr(address))...)
					if err == nil && got != abi.Int32(cxx.Success) {
						return fmt.Errorf("%s status: %v", name, got)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				got, err := value.Read()
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			owner := output("member_new", abi.Ptr(0), abi.Int32(20))
			defer func() {
				if got := output("member_delete", owner); got != abi.Ptr(0) {
					t.Error("native owner was not consumed", got)
				}
			}()
			if owner.Bits == 0 {
				t.Fatal("missing native owner")
			}
			if got := output("member_add", abi.Int32(0), owner, abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal("discovered const overload", got)
			}
			if got := output("member_twice", abi.Int32(0), abi.Int32(21)); got != abi.Int32(42) {
				t.Fatal("discovered static member", got)
			}
		})
	}
}

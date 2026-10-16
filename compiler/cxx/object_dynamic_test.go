//go:build libffi && cgo && (linux || darwin || windows)

package cxx_test

import (
	"path/filepath"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

func TestNativeDynamicObjectFacade(t *testing.T) {
	header, err := filepath.Abs("testdata/counter.hpp")
	if err != nil {
		t.Fatal(err)
	}
	facade, err := generateObjects(filepath.ToSlash(header))
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
				functions[name], err = session.Bind(declaration.Symbol, declaration.Signature)
				if err != nil {
					t.Fatal(err)
				}
			}
			storage := func(initial abi.Value) *abi.NativeValue {
				t.Helper()
				v, err := abi.NewNativeValue(abi.TypeDesc{Type: initial.Type}, initial)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := v.Close(); err != nil {
						t.Error(err)
					}
				})
				return v
			}
			call := func(name string, status int32, args ...abi.Value) {
				t.Helper()
				got, err := functions[name].Call(args...)
				if err != nil || got != abi.Int32(status) {
					t.Fatalf("%s: %v, %v; want %d", name, got, err, status)
				}
			}
			output := func(name string, initial abi.Value, status int32, args ...abi.Value) abi.Value {
				t.Helper()
				v := storage(initial)
				if err := v.WithAddress(func(address uintptr) error {
					call(name, status, append(args, abi.Ptr(address))...)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				got, err := v.Read()
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			owned := func(name string, args ...abi.Value) abi.Value { return output(name, abi.Ptr(0), cxx.Success, args...) }
			integer := func(name string, status int32, args ...abi.Value) abi.Value {
				return output(name, abi.Int32(999), status, args...)
			}
			destroy := func(name string, owner *abi.Value) {
				t.Helper()
				*owner = output(name, *owner, cxx.Success)
				if *owner != abi.Ptr(0) {
					t.Fatal("destruction did not clear the owner")
				}
			}
			first, second, bad := owned("object_new", abi.Int32(20)), owned("object_new", abi.Int32(22)), owned("object_new", abi.Int32(13))
			clone, copied, token := abi.Ptr(0), abi.Ptr(0), abi.Ptr(0)
			readonly := owned("readonly_new", abi.Int32(22))
			t.Cleanup(func() {
				destroy("token_delete", &token)
				destroy("readonly_delete", &readonly)
				for _, owner := range []*abi.Value{&first, &second, &bad, &clone, &copied} {
					destroy("object_delete", owner)
				}
			})
			for _, name := range []string{"object_sum", "object_copied", "object_sum"} {
				if got := integer(name, cxx.Success, first, second); got != abi.Int32(42) {
					t.Fatal(name, got)
				}
			}
			if got := integer("object_sum", cxx.Success, first, readonly); got != abi.Int32(42) {
				t.Fatal("constructed const-qualified alias:", got)
			}
			if got := integer("object_mixed", cxx.Success, first, abi.Int32(0), second, abi.Ptr(0)); got != abi.Int32(42) {
				t.Fatal("multiple object arguments:", got)
			}
			copied, clone = owned("object_copy", second), owned("object_clone", first)
			destroy("object_delete", &copied)
			copied = owned("object_value_copy", second)
			if copied.Bits == 0 || copied == second || clone.Bits == 0 || clone == first {
				t.Fatal("missing independent copies")
			}
			if got := integer("object_mutate", cxx.Success, clone, clone, abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal("reference mutation:", got)
			}
			if got := integer("object_nullable", cxx.Success, first); got != abi.Int32(20) {
				t.Fatal("clone changed original:", got)
			}
			for _, name := range []string{"object_self", "object_const_self", "object_pointer", "object_const_pointer", "object_const_alias"} {
				if view := owned(name, first); view != first {
					t.Fatal("borrowed address/overloaded operator&:", name, view)
				}
			}
			if got := output("object_self", first, cxx.InvalidArgument, abi.Ptr(0)); got != abi.Ptr(0) {
				t.Fatal("stale borrowed output:", got)
			}
			if got := owned("object_pointer", abi.Ptr(0)); got != abi.Ptr(0) {
				t.Fatal("nullable pointer result:", got)
			}
			if got := integer("object_nullable", cxx.Success, abi.Ptr(0)); got != abi.Int32(42) {
				t.Fatal("nullable pointer argument:", got)
			}
			if got := integer("object_sum", cxx.InvalidArgument, first, abi.Ptr(0)); got != abi.Int32(0) {
				t.Fatal("null reference output:", got)
			}
			if got := integer("object_copied", cxx.Exception, first, bad); got != abi.Int32(0) {
				t.Fatal("throwing copy output:", got)
			}
			if got := output("object_clone", clone, cxx.InvalidArgument, first); got != clone {
				t.Fatal("owned result overwritten:", got)
			}
			for _, name := range []string{"object_clone", "object_copy", "object_value_copy"} {
				if got := output(name, abi.Ptr(0), cxx.Exception, bad); got != abi.Ptr(0) {
					t.Fatal("throwing copy published an owner:", name, got)
				}
			}
			if got := output("object_copy", abi.Ptr(0), cxx.InvalidArgument, abi.Ptr(0)); got != abi.Ptr(0) {
				t.Fatal("null constructor reference:", got)
			}
			call("object_clone", cxx.InvalidArgument, first, abi.Ptr(0))
			token = owned("token_make", abi.Int32(42))
			if got := integer("token_read", cxx.Success, token); got != abi.Int32(42) {
				t.Fatal("noncopyable C++17 result:", got)
			}
			if got := integer("static_answer", cxx.Success); got != abi.Int32(42) {
				t.Fatal("private destructor static class:", got)
			}
			for _, owner := range []*abi.Value{&first, &second, &bad, &clone, &copied} {
				destroy("object_delete", owner)
			}
			destroy("readonly_delete", &readonly)
			if got := integer("object_live", cxx.Success); got != abi.Int32(0) {
				t.Fatal("native copies leaked:", got)
			}
		})
	}
}

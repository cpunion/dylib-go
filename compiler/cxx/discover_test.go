package cxx_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/compiler/cxx"
)

func TestNativeDiscoverBasesAcrossTargets(t *testing.T) {
	header, err := filepath.Abs("testdata/bases.hpp")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"example::Counter", "example::CounterAlias", "example::Prefix", "example::Derived", "example::DerivedAlias", "example::Diamond", "example::VirtualLeft", "example::PrivateCounter", "example::ProtectedCounter", "example::AmbiguousCounter", "example::Token", "example::TemplateInt", "example::TemplateDerived", "example::TargetCounter", "Answer", "valid_0"}
	expected := []cxx.BaseRelation{
		{"example::Derived", "example::Counter"}, {"example::Derived", "example::CounterAlias"}, {"example::Derived", "example::Prefix"},
		{"example::DerivedAlias", "example::Counter"}, {"example::DerivedAlias", "example::CounterAlias"}, {"example::DerivedAlias", "example::Prefix"},
		{"example::Diamond", "example::Counter"}, {"example::Diamond", "example::CounterAlias"}, {"example::Diamond", "example::VirtualLeft"},
		{"example::VirtualLeft", "example::Counter"}, {"example::VirtualLeft", "example::CounterAlias"},
		{"example::TemplateDerived", "example::TemplateInt"},
	}
	for _, target := range []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu", "i686-unknown-linux-gnu", "x86_64-apple-macosx11", "arm64-apple-macosx11", "x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			got, err := cxx.DiscoverBases(context.Background(), header, names, cxx.DiscoveryOptions{Compiler: cppTool(t), Target: target, Flags: []string{"-ffreestanding"}})
			want := append([]cxx.BaseRelation(nil), expected...)
			if strings.HasPrefix(target, "i686-") {
				want = append(want, cxx.BaseRelation{"example::TargetCounter", "example::Counter"}, cxx.BaseRelation{"example::TargetCounter", "example::CounterAlias"})
			} else {
				want = append(want, cxx.BaseRelation{"example::TargetCounter", "example::Token"})
			}
			want = append(want, cxx.BaseRelation{"valid_0", "Answer"})
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("base discovery: %v\n got: %v\nwant: %v", err, got, want)
			}
		})
	}
	for _, private := range []bool{false, true} {
		opts := cxx.DiscoveryOptions{Compiler: cppTool(t)}
		if private {
			opts.Flags = []string{"-DDYLIB_QUERY_PRIVATE"}
		}
		got, err := cxx.DiscoverBases(context.Background(), header, []string{"example::Counter", "example::ConditionalCounter"}, opts)
		if err != nil || private && len(got) != 0 || !private && !reflect.DeepEqual(got, []cxx.BaseRelation{{"example::ConditionalCounter", "example::Counter"}}) {
			t.Fatal("preprocessing flags were ignored", private, got, err)
		}
	}
}

func TestDiscoverBasesRejectsInvalidInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "header.hpp")
	write(t, path, []byte("class A {};\n"))
	opts := cxx.DiscoveryOptions{Compiler: filepath.Join(t.TempDir(), "missing compiler")}
	for _, names := range [][]string{nil, make([]string, 33), {"A", "A"}, {"class"}, {"A; injected"}, {"A*"}, {strings.Repeat("a", 257)}} {
		if got, err := cxx.DiscoverBases(context.Background(), path, names, opts); err == nil || got != nil {
			t.Fatal("invalid class names accepted", got, err)
		}
	}
	if got, err := cxx.DiscoverBases(nil, path, []string{"A"}, opts); err == nil || got != nil {
		t.Fatal("nil context accepted", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := cxx.DiscoverBases(ctx, path, []string{"A"}, opts); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatal("canceled context", got, err)
	}
	for _, invalid := range []string{filepath.Dir(path), path + ".missing"} {
		if got, err := cxx.DiscoverBases(context.Background(), invalid, []string{"A"}, opts); err == nil || got != nil {
			t.Fatal("invalid header accepted", got, err)
		}
	}
	if got, err := cxx.DiscoverBases(context.Background(), path, []string{"A"}, opts); err == nil || got != nil {
		t.Fatal("missing compiler accepted", got, err)
	}
	huge := filepath.Join(t.TempDir(), "huge.hpp")
	file, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(16<<20 + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cxx.DiscoverBases(context.Background(), huge, []string{"A"}, opts); err == nil || got != nil || !strings.Contains(err.Error(), "regular header") {
		t.Fatal("oversize header accepted", got, err)
	}
}

func TestNativeDiscoverBasesMaximum(t *testing.T) {
	var source strings.Builder
	names := make([]string, 32)
	var want []cxx.BaseRelation
	for i := range names {
		names[i] = fmt.Sprintf("C%d", i)
		if i == 0 {
			fmt.Fprintln(&source, "struct C0 {};")
		} else {
			fmt.Fprintf(&source, "struct %s : %s {};\n", names[i], names[i-1])
		}
		for j := 0; j < i; j++ {
			want = append(want, cxx.BaseRelation{Derived: names[i], Base: names[j]})
		}
	}
	header := filepath.Join(t.TempDir(), "maximum.hpp")
	write(t, header, []byte(source.String()))
	got, err := cxx.DiscoverBases(context.Background(), header, names, cxx.DiscoveryOptions{Compiler: cppTool(t)})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("maximum selected types: got %d relations, want %d: %v", len(got), len(want), err)
	}
}

func TestNativeDiscoverBasesRejectsUnsupportedTypes(t *testing.T) {
	header, err := filepath.Abs("testdata/bases.hpp")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ConstCounter", "CounterPointer", "Scalar", "Incomplete", "Missing"} {
		t.Run(name, func(t *testing.T) {
			if got, err := cxx.DiscoverBases(context.Background(), header, []string{"example::" + name}, cxx.DiscoveryOptions{Compiler: cppTool(t)}); err == nil || got != nil {
				t.Fatal("unsupported or incomplete selected type accepted", got, err)
			}
		})
	}
}

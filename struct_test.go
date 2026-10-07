package dylib

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/abi/signature"
)

func TestStructABI(t *testing.T) {
	if !abi.Available() {
		t.Skip("requires libffi")
	}
	needShared(t)
	dir := t.TempDir()
	library := filepath.Join(dir, "records.so")
	switch runtime.GOOS {
	case "darwin":
		library = filepath.Join(dir, "records.dylib")
		command(t, compiler(), "-dynamiclib", "testdata/cli.c", "-o", library)
	case "windows":
		library = filepath.Join(dir, "records.dll")
		command(t, compiler(), "-shared", "testdata/cli.c", "-Wl,--export-all-symbols", "-o", library)
	default:
		command(t, compiler(), "-shared", "-fPIC", "testdata/cli.c", "-o", library)
	}
	s := New(Options{})
	defer s.Close()
	load(t, s, library)
	for _, tc := range []struct{ text, expected string }{
		{"sum_pair({a:20,b:22}:struct{a,b int32})int32", "42"},
		{"sum_pair_ptr({a:20,b:22}:*struct{a,b int32})int32", "42"},
		{"echo_pair({a:20,b:22}:struct{a,b int32})struct{a,b int32}", "{a:20,b:22}"},
		{"mutate_pair(&{a:20,b:22}:*struct{a,b int32})*struct{a,b int32}", "&{a:22,b:20}"},
		{"scalar_ptr(&40:*int32)int32", "42"},
		{"sum_nested({tag:1,p:{a:20,b:20},extra:1}:struct{tag int8;p struct{a,b int32};extra float64})float64", "42"},
		{"echo_nested({tag:1,p:{a:20,b:20},extra:1}:struct{tag int8;p struct{a,b int32};extra float64})struct{tag int8;p struct{a,b int32};extra float64}", "{tag:1,p:{a:20,b:20},extra:1}"},
		{"echo_float_pair({x:20.5,y:21.5}:struct{x,y float32})struct{x,y float32}", "{x:20.5,y:21.5}"},
		{"echo_pair_ref({p:&{a:20,b:20}}:struct{p *struct{a,b int32};bonus int32})struct{p *struct{a,b int32};bonus int32}", "{p:&{a:22,b:20},bonus:0}"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			invocation, err := signature.ParseCall(tc.text)
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.Bind(invocation.Name, invocation.Signature)
			if err != nil {
				t.Fatal(err)
			}
			v, err := f.Call(invocation.Args...)
			if err != nil {
				t.Fatal(err)
			}
			printed, err := signature.FormatValue(v)
			if err != nil || printed != tc.expected {
				t.Fatalf("got %s %v; want %s", printed, err, tc.expected)
			}
			if invocation.Name == "scalar_ptr" && *invocation.Args[0].Pointee != abi.Int32(42) {
				t.Fatal("pointee update lost")
			}
		})
	}
	t.Run("temporary aliases", func(t *testing.T) {
		invocation, err := signature.ParseCall("same_pair({20,20}:*struct{a,b int32},nil:*struct{a,b int32})bool")
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Bind(invocation.Name, invocation.Signature)
		if err != nil {
			t.Fatal(err)
		}
		invocation.Args[1] = invocation.Args[0]
		v, err := f.Call(invocation.Args...)
		if err != nil || v != abi.Boolean(true) || invocation.Args[0].Pointee.Aggregate.Fields[0] != abi.Int32(22) {
			t.Fatalf("alias identity or update lost: %+v %v", v, err)
		}
	})
	t.Run("reject interior temporary pointer", func(t *testing.T) {
		invocation, err := signature.ParseCall("interior_pair({20,22}:*struct{a,b int32})*int32")
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Bind(invocation.Name, invocation.Signature)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Call(invocation.Args...); err == nil || !strings.Contains(err.Error(), "interior pointer") {
			t.Fatalf("expected lifetime error, got %v", err)
		}
	})
	t.Run("bound signature owns descriptors", func(t *testing.T) {
		invocation, err := signature.ParseCall("sum_pair({20,22}:struct{a,b int32})int32")
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Bind(invocation.Name, invocation.Signature)
		if err != nil {
			t.Fatal(err)
		}
		invocation.Signature.ArgTypes[0].Fields[0].Type.Type = abi.F64
		v, err := f.Call(invocation.Args...)
		if err != nil || v != abi.Int32(42) {
			t.Fatalf("caller mutation changed bound signature: %+v %v", v, err)
		}
	})
}

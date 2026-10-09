//go:build libffi && cgo && (linux || darwin || windows)

package clang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func TestNativeDeclaredFunctionPointers(t *testing.T) {
	compiler := compilerTool(t)
	headerPath := headerFile(t, functionPointerHeader+`void swap_pair(Pair *);
typedef struct {double values[4];} Large;
Large large_apply(Large (*)(Large),Large);
#if defined(_WIN32) && defined(__i386__)
int std_apply(int (__attribute__((stdcall)) *)(int,int),int,int);
int fast_apply(int (__attribute__((fastcall)) *)(int,int),int,int);
#endif
`)
	names := []string{"apply", "direct", "decayed", "factory", "anonymous_factory", "pair_apply", "pointer_apply", "variable_factory", "swap_pair", "large_apply"}
	if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		names = append(names, "std_apply", "fast_apply")
	}
	h, err := Parse(context.Background(), headerPath, Options{Compiler: compiler, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source, object := filepath.Join(dir, "callbacks.c"), filepath.Join(dir, "callbacks.o")
	code := `static int add(int a,int b){return a+b;}
Adder factory(void){return add;}
int (*anonymous_factory(void))(int,int){return add;}
int apply(Adder callback,int a,int b){return callback(a,b);}
int direct(int (*callback)(int,int),int a,int b){return callback(a,b);}
int decayed(int callback(int,int),int a,int b){return callback(a,b);}
int pair_apply(PairFn callback,Pair p){Pair r=callback(p);return r.a+r.b;}
void pointer_apply(void (*callback)(Pair *),Pair *p){callback(p);}
void swap_pair(Pair *p){int a=p->a;p->a=p->b;p->b=a;}
static int variable(int first,...){__builtin_va_list ap;__builtin_va_start(ap,first);int a=__builtin_va_arg(ap,int);double b=__builtin_va_arg(ap,double);__builtin_va_end(ap);return first+a+(int)b;}
Variable variable_factory(void){return variable;}
Large large_apply(Large (*callback)(Large),Large value){return callback(value);}
#if defined(_WIN32) && defined(__i386__)
int std_apply(int (__attribute__((stdcall)) *callback)(int,int),int a,int b){return callback(a,b);}
int fast_apply(int (__attribute__((fastcall)) *callback)(int,int),int a,int b){return callback(a,b);}
#endif
`
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--target=" + nativeTriple(), "-O0", "-c", "-fno-stack-protector", "-include", headerPath, source, "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("function-pointer producer: %v\n%s", err, data)
	}
	s := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	defer s.Close()
	if runtime.GOOS == "windows" {
		directory := "System32"
		if runtime.GOARCH == "386" {
			directory = "SysWOW64"
		}
		if err := s.Load(filepath.Join(os.Getenv("SystemRoot"), directory, "msvcrt.dll")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Load(object); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(); err != nil {
		t.Fatal(err)
	}
	declaration := func(name string) Declaration {
		t.Helper()
		d, err := h.ForHost(name)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	callbackCall := func(name string, handler abi.CallbackFunc, args []abi.Value, want abi.Value) {
		t.Helper()
		d := declaration(name)
		pointer, err := d.LookupFunctionPointer(0)
		if err != nil {
			t.Fatal(err)
		}
		callback, err := abi.NewCallback(pointer.Signature, handler)
		if err != nil {
			t.Fatal(err)
		}
		defer callback.Close()
		fn, err := s.Bind(d.Symbol, d.Signature)
		if err != nil {
			t.Fatal(err)
		}
		err = callback.WithAddress(func(address uintptr) error {
			got, err := fn.Call(append([]abi.Value{abi.Ptr(address)}, args...)...)
			if err == nil && !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: %+v; want %+v", name, got, want)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := callback.Err(); err != nil {
			t.Fatal(err)
		}
	}
	offset := int32(2)
	for _, name := range []string{"apply", "direct", "decayed"} {
		callbackCall(name, func(args []abi.Value) (abi.Value, error) {
			return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits) + offset), nil
		}, []abi.Value{abi.Int32(20), abi.Int32(20)}, abi.Int32(42))
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		for _, name := range []string{"std_apply", "fast_apply"} {
			callbackCall(name, func(args []abi.Value) (abi.Value, error) {
				return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
			}, []abi.Value{abi.Int32(20), abi.Int32(22)}, abi.Int32(42))
		}
	}
	pair, err := h.LookupRecord("Pair")
	if err != nil {
		t.Fatal(err)
	}
	value, err := abi.StructValue(pair.Description, abi.Int32(20), abi.Int32(22))
	if err != nil {
		t.Fatal(err)
	}
	callbackCall("pair_apply", func(args []abi.Value) (abi.Value, error) { return args[0], nil }, []abi.Value{value}, abi.Int32(42))
	large, err := h.LookupRecord("Large")
	if err != nil {
		t.Fatal(err)
	}
	largeValue := abi.Zero(large.Description)
	for i := range largeValue.Aggregate.Fields[0].Aggregate.Fields {
		largeValue.Aggregate.Fields[0].Aggregate.Fields[i] = abi.Float64(10.5)
	}
	callbackCall("large_apply", func(args []abi.Value) (abi.Value, error) { return args[0], nil }, []abi.Value{largeValue}, largeValue)
	swap := declaration("swap_pair")
	swapFn, err := s.Bind(swap.Symbol, swap.Signature)
	if err != nil {
		t.Fatal(err)
	}
	callbackCall("pointer_apply", func(args []abi.Value) (abi.Value, error) { return swapFn.Call(args...) }, []abi.Value{abi.AddressOf(&value)}, abi.Value{Type: abi.Void})
	if value.Aggregate.Fields[0] != abi.Int32(22) || value.Aggregate.Fields[1] != abi.Int32(20) {
		t.Fatal("callback pointer mutation was lost", value)
	}
	for _, name := range []string{"factory", "anonymous_factory", "variable_factory"} {
		d := declaration(name)
		p, err := d.LookupFunctionPointer(-1)
		if err != nil {
			t.Fatal(err)
		}
		args := []abi.Value{abi.Int32(20), abi.Int32(22)}
		if name == "variable_factory" {
			p, err = p.WithTail(abi.I8, abi.F32)
			if err != nil {
				t.Fatal(err)
			}
			args = []abi.Value{abi.Int32(20), abi.Int8(10), abi.Float32(12)}
		}
		symbol, err := s.Resolve(d.Symbol)
		if err != nil {
			t.Fatal(err)
		}
		// Retain the image while using an address returned by its factory.
		err = symbol.WithAddress(func(address uintptr) error {
			result, err := abi.Call(address, d.Signature)
			if err != nil {
				return err
			}
			got, err := abi.Call(uintptr(result.Bits), p.Signature, args...)
			if err == nil && got != abi.Int32(42) {
				t.Fatalf("%s returned %+v", name, got)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeChecksFunctionPointerRecordLayout(t *testing.T) {
	h, err := Parse(context.Background(), headerFile(t, "struct S {char a;int b;}; void f(struct S (*)(struct S));"), Options{Compiler: compilerTool(t), Flags: []string{"-fpack-struct=1"}, Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ForHost("f"); err == nil {
		t.Fatal("callback layout mismatch accepted")
	}
}

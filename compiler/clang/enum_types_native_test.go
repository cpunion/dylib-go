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

func TestNativeDeclaredEnums(t *testing.T) {
	compiler := compilerTool(t)
	path := headerFile(t, enumHeader+"Wide variable(enum Small,...);\n")
	names := []string{"signed_echo", "unsigned_echo", "anonymous_echo", "small_echo", "wide_echo", "bool_echo", "forward_echo", "factory", "apply", "packet_echo", "variable"}
	code := `enum Signed signed_echo(enum Signed x){return x;}
enum Unsigned unsigned_echo(enum Unsigned x){return x;}
Alias anonymous_echo(Alias x){return x;}
enum Small small_echo(enum Small x){return x;}
Wide wide_echo(Wide x){return x;}
Boolean bool_echo(Boolean x){return x;}
enum Forward forward_echo(enum Forward x){return x;}
Entry factory(void){return small_echo;}
enum Small apply(Entry fn,enum Small x){return fn(x);}
Packet packet_echo(Packet x){return x;}
Wide variable(enum Small x,...){__builtin_va_list ap;__builtin_va_start(ap,x);int a=__builtin_va_arg(ap,int);double b=__builtin_va_arg(ap,double);__builtin_va_end(ap);return x+a+(int)b;}
`
	for _, short := range []bool{false, true} {
		var enumFlags []string
		if short {
			enumFlags = []string{"-fshort-enums"}
		}
		h, err := Parse(context.Background(), path, Options{Compiler: compiler, Functions: names, Flags: enumFlags})
		if err != nil {
			t.Fatal(err)
		}
		for _, optimization := range []string{"-O0", "-O2"} {
			t.Run(map[bool]string{false: "default", true: "short"}[short]+"/"+optimization, func(t *testing.T) {
				dir := t.TempDir()
				source, object := filepath.Join(dir, "enums.c"), filepath.Join(dir, "enums.o")
				if err := os.WriteFile(source, []byte(code), 0600); err != nil {
					t.Fatal(err)
				}
				flags := append([]string{"--target=" + h.Target.Triple, "-std=c17", optimization, "-c", "-fno-stack-protector", "-include", path, source, "-o", object}, enumFlags...)
				if runtime.GOOS != "windows" {
					flags = append(flags, "-fPIC")
				}
				if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
					t.Fatalf("enum producer: %v\n%s", err, data)
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
				invoke := func(name string, args ...abi.Value) abi.Value {
					t.Helper()
					d, err := h.ForHost(name)
					if name == "variable" && err == nil {
						d, err = d.WithTail(abi.I8, abi.F32)
					}
					if err != nil {
						t.Fatal(err)
					}
					fn, err := s.Bind(d.Symbol, d.Signature)
					if err != nil {
						t.Fatal(err)
					}
					got, err := fn.Call(args...)
					if err != nil {
						t.Fatal(name, err)
					}
					return got
				}
				integer := func(typ abi.Type, bits uint64) abi.Value { return abi.Value{Type: typ, Bits: bits} }
				inputs := []uint64{uint64(1) << 31, uint64(1) << 31, 255, 128, ^uint64(0), 1, 65535}
				for i, bits := range inputs {
					argument := integer(h.Functions[i].Signature.Result, bits)
					if i == 0 && argument.Type == abi.I8 {
						argument.Bits = 128
					}
					if got := invoke(names[i], argument); got != argument {
						t.Fatalf("enum %s: %+v, want %+v", names[i], got, argument)
					}
				}
				entry, err := h.Functions[7].LookupFunctionPointer(-1)
				if err != nil {
					t.Fatal(err)
				}
				callback, err := abi.NewCallback(entry.Signature, func(args []abi.Value) (abi.Value, error) { return args[0], nil })
				if err != nil {
					t.Fatal(err)
				}
				defer callback.Close()
				argument := integer(entry.Signature.Result, 128)
				factory, err := s.Resolve(h.Functions[7].Symbol)
				if err != nil {
					t.Fatal(err)
				}
				if err := factory.WithAddress(func(_ uintptr) error {
					address := invoke("factory")
					plan, err := abi.Prepare(entry.Signature)
					if err != nil {
						return err
					}
					defer plan.Close()
					got, err := plan.Call(uintptr(address.Bits), argument)
					if err != nil || got != argument {
						t.Fatal("enum native factory:", got, err)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if err := callback.WithAddress(func(address uintptr) error {
					if got := invoke("apply", abi.Ptr(address), argument); got != argument {
						t.Fatal("enum callback:", got)
					}
					packet, err := h.LookupRecord("Packet")
					if err != nil {
						return err
					}
					value := abi.Zero(packet.Description)
					value.Aggregate.Fields[0] = argument
					value.Aggregate.Fields[1].Aggregate.Fields[0] = abi.Uint64(^uint64(0))
					value.Aggregate.Fields[1].Aggregate.Fields[1] = abi.Uint64(uint64(1) << 63)
					value.Aggregate.Fields[2] = abi.Boolean(true)
					value.Aggregate.Fields[3] = abi.Ptr(address)
					if got := invoke("packet_echo", value); !reflect.DeepEqual(got, value) {
						t.Fatal("enum record/array/callback fields:", got)
					}
					return callback.Err()
				}); err != nil {
					t.Fatal(err)
				}
				if got := invoke("variable", integer(entry.Signature.Result, 20), abi.Int8(10), abi.Float32(12)); got != abi.Uint64(42) {
					t.Fatal("enum variadic prefix:", got)
				}
			})
		}
	}
}

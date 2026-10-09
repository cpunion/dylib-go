//go:build libffi && cgo && (linux || darwin || windows)

package clang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
)

func TestNativeDeclaredRecordsAndPointers(t *testing.T) {
	compiler := compilerTool(t)
	headerPath := headerFile(t, recordHeader+"typedef struct {float values[4];} FloatVector; FloatVector float_echo(FloatVector);\n")
	header, err := Parse(context.Background(), headerPath, Options{Compiler: compiler, Functions: []string{"echo", "nest", "mutate", "var_record", "float_echo"}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source, object := filepath.Join(dir, "records.c"), filepath.Join(dir, "records.o")
	code := `Pair echo(Pair p){return p;}
struct Outer nest(struct Outer p){return p;}
Pair *mutate(Pair *p){p->tag++;p->value++;p->tail++;return p;}
int var_record(Pair p,...){__builtin_va_list ap;__builtin_va_start(ap,p);int a=__builtin_va_arg(ap,int);double b=__builtin_va_arg(ap,double);__builtin_va_end(ap);return p.tag+(int)p.value+p.tail+a+(int)b;}
FloatVector float_echo(FloatVector value){return value;}
`
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--target=" + nativeTriple(), "-O0", "-c", "-fno-stack-protector", "-include", headerPath, source, "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("record producer: %v\n%s", err, data)
	}
	session := dylib.New(dylib.Options{ProcessSymbols: runtime.GOOS != "windows"})
	defer session.Close()
	if runtime.GOOS == "windows" {
		directory := "System32"
		if runtime.GOARCH == "386" {
			directory = "SysWOW64"
		}
		if err := session.Load(filepath.Join(os.Getenv("SystemRoot"), directory, "msvcrt.dll")); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.Load(object); err != nil {
		t.Fatal(err)
	}
	if err := session.Link(); err != nil {
		t.Fatal(err)
	}
	call := func(name string, args []abi.Value, tail ...abi.Type) abi.Value {
		t.Helper()
		declaration, err := header.ForHost(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(tail) != 0 {
			declaration, err = declaration.WithTail(tail...)
			if err != nil {
				t.Fatal(err)
			}
		}
		function, err := session.Bind(declaration.Symbol, declaration.Signature)
		if err != nil {
			t.Fatal(err)
		}
		value, err := function.Call(args...)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	pair, _ := header.LookupRecord("Pair")
	value, err := abi.StructValue(pair.Description, abi.Int8(1), abi.Float64(20.5), abi.Int16(2))
	if err != nil {
		t.Fatal(err)
	}
	if got := call("echo", []abi.Value{value}); !reflect.DeepEqual(got, value) {
		t.Fatalf("record return: %+v", got)
	}
	vector, _ := header.LookupRecord("FloatVector")
	floats := abi.Zero(vector.Description)
	for i := range floats.Aggregate.Fields[0].Aggregate.Fields {
		floats.Aggregate.Fields[0].Aggregate.Fields[i] = abi.Float32(10.5)
	}
	if got := call("float_echo", []abi.Value{floats}); !reflect.DeepEqual(got, floats) {
		t.Fatalf("floating-point aggregate return: %+v", got)
	}
	if got := call("var_record", []abi.Value{value, abi.Int8(3), abi.Float32(16.5)}, abi.I8, abi.F32); got != abi.Int32(42) {
		t.Fatalf("record variadic prefix: %+v", got)
	}
	outer, _ := header.LookupRecord("struct Outer")
	record := abi.Zero(outer.Description)
	record.Aggregate.Fields[0].Aggregate.Fields[0] = value
	record.Aggregate.Fields[0].Aggregate.Fields[1] = value
	record.Aggregate.Fields[1].Aggregate.Fields[1].Aggregate.Fields[2] = abi.Int32(42)
	if got := call("nest", []abi.Value{record}); !reflect.DeepEqual(got, record) {
		t.Fatalf("nested array/large return: %+v", got)
	}
	got := call("mutate", []abi.Value{abi.AddressOf(&value)})
	want, err := abi.StructValue(pair.Description, abi.Int8(2), abi.Float64(21.5), abi.Int16(3))
	if err != nil {
		t.Fatal(err)
	}
	if got.Pointee != &value || !reflect.DeepEqual(value, want) {
		t.Fatalf("typed record pointer mutation/identity: %+v / %+v", got, value)
	}
}

func TestNativeRejectsCompilerPackingMismatch(t *testing.T) {
	path := headerFile(t, "struct S {char tag;int value;}; struct S f(struct S);")
	h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Flags: []string{"-fpack-struct=1"}, Functions: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ForHost("f"); err == nil || !strings.Contains(err.Error(), "layout mismatch") {
		t.Fatal("compiler packing bypassed backend validation:", err)
	}
}

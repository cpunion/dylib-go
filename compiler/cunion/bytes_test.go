package cunion_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cunion"
)

func TestUnionByteManifestValidation(t *testing.T) {
	valid := cunion.Value{Type: abi.Array, Union: "union U", Bytes: 8}
	for name, patch := range map[string]func(*cunion.Value){
		"negative":   func(v *cunion.Value) { v.Bytes = -1 },
		"zero":       func(v *cunion.Value) { v.Bytes = 0 },
		"too_large":  func(v *cunion.Value) { v.Bytes = 65537 },
		"no_union":   func(v *cunion.Value) { v.Union = "" },
		"scalar":     func(v *cunion.Value) { v.Type = abi.U64 },
		"struct":     func(v *cunion.Value) { v.Type = abi.Struct },
		"record":     func(v *cunion.Value) { v.Record = "struct S" },
		"array":      func(v *cunion.Value) { v.Array = "Bytes" },
		"expression": func(v *cunion.Value) { v.Member = "*pointer" },
		"index":      func(v *cunion.Value) { v.Member = "inner[0]" },
	} {
		t.Run(name, func(t *testing.T) {
			v := valid
			patch(&v)
			for _, result := range []bool{false, true} {
				f := cunion.Function{Name: "target", Symbol: "bridge"}
				if result {
					f.Result = v
				} else {
					f.Args = []cunion.Value{v}
				}
				if _, err := cunion.Generate("values.h", []cunion.Function{f}); err == nil {
					t.Fatal("invalid byte view accepted", v)
				}
			}
		})
	}
	for _, size := range []int{1, 65536} {
		v := valid
		v.Bytes = size
		if _, err := cunion.Generate("values.h", []cunion.Function{{Name: "target", Symbol: "bridge", Args: []cunion.Value{v}, Result: v}}); err != nil {
			t.Fatal("byte size boundary", size, err)
		}
	}
}

func TestUnionCompilerRejectsByteContracts(t *testing.T) {
	for _, tc := range []struct {
		name, header, member, message string
		size                          int
	}{
		{"not_union", "typedef struct{unsigned char bytes[8];} U;U target(U);", "", "selected type must be a union", 8},
		{"too_small", "typedef union{unsigned char bytes[8];} U;U target(U);", "", "byte view requires exact union storage", 7},
		{"too_large", "typedef union{unsigned char bytes[8];} U;U target(U);", "", "byte view requires exact union storage", 9},
		{"scalar_leaf", "typedef union{unsigned long long scalar;} U;U target(U);", "scalar", "byte view must select a union", 8},
		{"record_leaf", "typedef union{struct{unsigned char bytes[8];} record;} U;U target(U);", "record", "byte view must select a union", 8},
		{"array_leaf", "typedef union{unsigned char bytes[8];} U;U target(U);", "bytes", "byte view must select a union", 8},
		{"pointer_leaf", "typedef union{void *pointer;} U;U target(U);", "pointer", "byte view must select a union", 8},
		{"wrong_prototype", "typedef union{unsigned char bytes[8];} U;U target(int);", "", "incompatible native function prototype", 8},
		{"missing_leaf", "typedef union{unsigned char bytes[8];} U;U target(U);", "missing", "", 8},
		{"bitfield_leaf", "typedef union{unsigned bit:3;} U;U target(U);", "bit", "", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			header := filepath.Join(dir, "values.h")
			write(t, header, []byte(tc.header))
			v := cunion.Value{Type: abi.Array, Union: "U", Member: tc.member, Bytes: tc.size}
			f, err := cunion.Generate(filepath.ToSlash(header), []cunion.Function{{Name: "target", Symbol: "bridge", Args: []cunion.Value{v}, Result: v}})
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "facade.c")
			write(t, source, f.CSource)
			out, err := exec.Command(clangTool(t), "-std=c17", "-ffreestanding", "-fsyntax-only", source).CombinedOutput()
			if err == nil || tc.message != "" && !strings.Contains(string(out), tc.message) {
				t.Fatalf("byte contract rejection: %v\n%s", err, out)
			}
		})
	}
}

func TestUnionByteHeadersComposeWithoutNativeHeader(t *testing.T) {
	dir := t.TempDir()
	v := cunion.Value{Type: abi.Array, Union: "union U", Bytes: 8}
	for _, symbol := range []string{"first", "second"} {
		f, err := cunion.Generate("missing-native-header.h", []cunion.Function{{Name: "target", Symbol: symbol, Args: []cunion.Value{v}, Result: v}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(f.CHeader), "missing-native-header.h") {
			t.Fatal("byte-only transport requires native header")
		}
		write(t, filepath.Join(dir, symbol+".h"), f.CHeader)
	}
	source := filepath.Join(dir, "compose.c")
	write(t, source, []byte("#include \"first.h\"\n#include \"second.h\"\n_Static_assert(__builtin_types_compatible_p(__typeof__(first),__typeof__(second)),\"byte wrapper identity\");\n"))
	if out, err := exec.Command(clangTool(t), "-std=c17", "-fsyntax-only", source).CombinedOutput(); err != nil {
		t.Fatalf("byte headers: %v\n%s", err, out)
	}
}

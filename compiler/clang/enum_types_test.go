package clang

import (
	"context"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

const enumHeader = `enum Signed { negative=-1, positive=42 };
enum Unsigned { high=0x80000000U };
typedef enum { tiny=255 } Anonymous;
typedef Anonymous Alias;
enum __attribute__((packed)) Small { minimum=-128, maximum=127 };
typedef enum : unsigned long long { limit=0xffffffffffffffffULL } Wide;
typedef enum : _Bool { off=0, on=1 } Boolean;
enum Forward : unsigned short;
enum Signed signed_echo(enum Signed);
enum Unsigned unsigned_echo(enum Unsigned);
Alias anonymous_echo(Alias);
enum Small small_echo(enum Small);
Wide wide_echo(Wide);
Boolean bool_echo(Boolean);
enum Forward forward_echo(enum Forward);
typedef enum Small (*Entry)(enum Small);
Entry factory(void);
enum Small apply(Entry,enum Small);
typedef struct { enum Small tag; Wide values[2]; Boolean truth; Entry entry; } Packet;
Packet packet_echo(Packet);
`

func TestCompilerQualifiedEnums(t *testing.T) {
	path, compiler := headerFile(t, enumHeader), compilerTool(t)
	names := []string{"signed_echo", "unsigned_echo", "anonymous_echo", "small_echo", "wide_echo", "bool_echo", "forward_echo", "factory", "apply", "packet_echo"}
	for _, target := range targets {
		for _, short := range []bool{false, true} {
			t.Run(target.triple+map[bool]string{true: "/short", false: "/default"}[short], func(t *testing.T) {
				t.Parallel()
				var flags []string
				if short {
					flags = []string{"-fshort-enums"}
				}
				h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Flags: flags, Functions: names})
				if err != nil {
					t.Fatal(err)
				}
				want := []abi.Type{abi.I32, abi.U32, abi.U32, abi.I8, abi.U64, abi.Bool, abi.U16}
				if target.os == "windows" {
					want[1], want[2], want[3] = abi.I32, abi.I32, abi.I32
				} else if short {
					want[0], want[2] = abi.I8, abi.U8
				}
				for i, typ := range want {
					signature := abi.Signature{Convention: abi.CDecl, Result: typ, Args: []abi.Type{typ}}
					if !reflect.DeepEqual(h.Functions[i].Signature, signature) {
						t.Fatalf("%s: %+v, want %+v", names[i], h.Functions[i].Signature, signature)
					}
				}
				entry, err := h.Functions[7].LookupFunctionPointer(-1)
				if err != nil || entry.Signature.Result != want[3] || !reflect.DeepEqual(entry.Signature.Args, []abi.Type{want[3]}) {
					t.Fatal("enum function pointer:", entry, err)
				}
				packet, err := h.LookupRecord("Packet")
				if err != nil || packet.Description.Fields[0].Type.Type != want[3] || packet.Description.Fields[1].Type.Elem.Type != abi.U64 || packet.Description.Fields[2].Type.Type != abi.Bool || len(packet.FunctionPointers) != 1 {
					t.Fatal("enum fields/arrays/callbacks:", packet, err)
				}
				generators := []func(string, string) ([]byte, error){h.GoSource, h.CgoSource}
				if target.arch != "386" {
					generators = append(generators, h.LLGoSource)
				}
				for _, generate := range generators {
					source, err := generate("bindings", "Bindings")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestUnsupportedEnums(t *testing.T) {
	for _, declaration := range []string{
		"enum Kind; enum Kind echo(enum Kind);",
		"enum __attribute__((aligned(16))) Kind { one=1 }; enum Kind echo(enum Kind);",
		"enum Kind : unsigned __int128 { one=1 }; enum Kind echo(enum Kind);",
	} {
		_, err := Parse(context.Background(), headerFile(t, declaration), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
		if err == nil {
			t.Fatal("unsupported enum accepted:", declaration, err)
		}
	}
	h, err := Parse(context.Background(), headerFile(t, "enum Kind; enum Kind *echo(enum Kind *);"), Options{Compiler: compilerTool(t), Functions: []string{"echo"}})
	if err != nil || h.Functions[0].Signature.Result != abi.Pointer || h.Functions[0].Signature.Args[0] != abi.Pointer {
		t.Fatal("opaque incomplete enum pointer:", h, err)
	}
}

func TestEnumTagAndTypedefNamespaces(t *testing.T) {
	path := headerFile(t, `enum Same { negative=-1 };
typedef enum : unsigned char { positive=255 } Same;
enum Same tagged(enum Same); Same aliased(Same);`)
	h, err := Parse(context.Background(), path, Options{Compiler: compilerTool(t), Functions: []string{"tagged", "aliased"}})
	if err != nil || h.Functions[0].Signature.Result != abi.I32 || h.Functions[1].Signature.Result != abi.U8 {
		t.Fatal("separate enum tag/typedef namespaces:", h, err)
	}
}

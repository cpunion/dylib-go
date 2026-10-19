package clang

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const nominalRecordsHeader = `
typedef struct { float x; int y; } Plain;
typedef struct __attribute__((packed)) { float x; int y; } Packed;
typedef Plain PlainAlias;
PlainAlias alias_plain(PlainAlias);
typedef struct { Plain a[2]; Packed b[2]; } Envelope;
Plain plain(Plain); Packed packed(Packed); Envelope envelope(Envelope);
Plain *plain_pointer(Plain *); Packed *packed_pointer(Packed *);
Packed (*factory(void))(Packed); Packed apply(Packed (*)(Packed),Packed);
`

func TestNominalRecordMetadata(t *testing.T) {
	path, compiler := headerFile(t, nominalRecordsHeader), compilerTool(t)
	functions := []string{"plain", "packed", "envelope", "plain_pointer", "packed_pointer", "factory", "apply", "alias_plain"}
	for _, target := range targets {
		t.Run(target.triple, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(context.Background(), path, Options{Compiler: compiler, Target: target.triple, Functions: functions, NominalRecords: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(h.Records) != 3 {
				t.Fatal("lost distinct nominal records", len(h.Records))
			}
			for _, r := range h.Records {
				if r.Description.RecordID != r.Name {
					t.Fatal("missing declaration identity", r.Name, r.Description.RecordID)
				}
			}
			plain, _ := h.LookupRecord("Plain")
			packed, _ := h.LookupRecord("Packed")
			alias, _ := h.Lookup("alias_plain")
			if alias.Signature.ArgumentType(0).RecordID != plain.Description.RecordID {
				t.Fatal("typedef alias created a new record identity")
			}
			if !reflect.DeepEqual(plain.Description.Fields, packed.Description.Fields) || plain.Description.RecordID == packed.Description.RecordID {
				t.Fatal("field shape substituted for nominal identity")
			}
			envelope, _ := h.LookupRecord("Envelope")
			if envelope.Description.Fields[0].Type.Elem.RecordID != plain.Description.RecordID || envelope.Description.Fields[1].Type.Elem.RecordID != packed.Description.RecordID {
				t.Fatal("nested arrays lost declaration identity")
			}
			pointer, _ := h.Lookup("packed_pointer")
			if pointer.Signature.ArgumentType(0).Elem.RecordID != packed.Description.RecordID || pointer.Signature.ReturnType().Elem.RecordID != packed.Description.RecordID {
				t.Fatal("pointee lost declaration identity")
			}
			factory, _ := h.Lookup("factory")
			if factory.FunctionPointers[0].Signature.ArgumentType(0).RecordID != packed.Description.RecordID {
				t.Fatal("native function pointer lost declaration identity")
			}
			encoded, err := json.Marshal(h)
			if err != nil {
				t.Fatal("marshal nominal header:", err)
			}
			var restored Header
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal("unmarshal nominal header:", err)
			}
			if !reflect.DeepEqual(*h, restored) {
				t.Fatal("nominal header metadata changed after JSON round trip")
			}
			code, err := h.GoSource("bindings", "Declarations")
			if err != nil || !strings.Contains(string(code), `RecordID: "Packed"`) {
				t.Fatal("portable source lost record identity", err)
			}
			if _, err := restored.CgoSource("bindings", "Bindings"); err != nil {
				t.Fatal("distinct layouts cannot be generated", err)
			}
			if target.os != "windows" || target.arch != "386" {
				testCompilerRecordCallingABI(t, compiler, path, &restored)
			}
		})
	}
	t.Run("darwin_unnamed", func(t *testing.T) {
		path := headerFile(t, `typedef struct { float x; unsigned long long :33; signed char y; } Wide;
   typedef struct { float x; unsigned long long :3; signed char y; } Small;
   Wide echo_wide(Wide); Small echo_small(Small);`)
		options := Options{Compiler: compiler, Target: "arm64-apple-macosx11", Functions: []string{"echo_wide", "echo_small"}}
		legacy, err := Parse(context.Background(), path, options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = legacy.CgoSource("bindings", "Bindings"); err == nil || !strings.Contains(err.Error(), "ambiguous native layouts") {
			t.Fatal("ambiguous legacy metadata accepted", err)
		}
		options.NominalRecords = true
		h, err := Parse(context.Background(), path, options)
		if err != nil {
			t.Fatal(err)
		}
		wide, _ := h.LookupRecord("Wide")
		small, _ := h.LookupRecord("Small")
		if !reflect.DeepEqual(wide.Description.Fields, small.Description.Fields) || wide.Layout.Size != 16 || small.Layout.Size != 8 || wide.Layout.Offsets[1] != 13 || small.Layout.Offsets[1] != 5 {
			t.Fatal("reproducer layout changed", wide.Layout, small.Layout)
		}
		if _, err = h.CgoSource("bindings", "Bindings"); err != nil {
			t.Fatal(err)
		}
		testCompilerRecordCallingABI(t, compiler, path, h)
	})

}

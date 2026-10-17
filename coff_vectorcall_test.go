package dylib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCOFFVectorcallNames(t *testing.T) {
	for _, target := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "names.c")
			if err := os.WriteFile(src, []byte(`int __vectorcall _vector_add(int a,int b){return a+b;}
int _cdecl_add(int a,int b){return a+b;}
__declspec(dllimport) int __vectorcall _external(int,int);
int invoke(int a,int b){return _external(a,b);}`), 0600); err != nil {
				t.Fatal(err)
			}
			path := compile(t, src, filepath.Join(dir, "names.o"), "--target="+target, "-ffreestanding")
			data, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, data)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, symbol := range f.obj.symbols {
				names[symbol.name] = true
			}
			for _, name := range []string{"_vector_add@@8", "_cdecl_add", "__imp__external@@8"} {
				if !names[name] {
					t.Fatalf("missing exact COFF name %q: %v", name, names)
				}
			}
		})
	}
}

func TestCOFFVectorcallShortImportNames(t *testing.T) {
	for _, name := range []string{"vector@@8", "_vector@@8", "__vector@@8"} {
		f, err := parse("vector.lib", shortImport(0x14c, 0, 1, name, "provider.dll", "", 0))
		if err != nil {
			t.Fatal(err)
		}
		entry := f.obj.symbols[0].imported.entry
		if entry.public != name || entry.name != name || f.obj.symbols[0].name != "__imp_"+name {
			t.Fatalf("vectorcall import decoration lost: %+v", entry)
		}
	}
}

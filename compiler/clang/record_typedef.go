package clang

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// RecordTypedefAlignment describes a written alias of a Record's body. Aligned
// is the alias's alignment, including inherited and lowered requests. The body
// Layout retains its own alignment and size; typedefs never add tail padding.
// Native pointers still require producer-owned, suitably aligned storage.
type RecordTypedefAlignment struct {
	Name    string
	Aligned uint64
}

// A plain spelling used in layout probes must also be free of inherited
// typedef attributes. Index all aliases before selecting accessible body names.
func (r *typeResolver) recordTypedefAttributes(node astNode) bool {
	for depth := 0; depth <= 16; depth++ {
		for _, child := range node.Inner {
			if strings.HasSuffix(child.Kind, "Attr") {
				return true
			}
		}
		var ok bool
		node, ok = r.aliasNodes[unqualified(node.Type.QualType)]
		if !ok {
			return false
		}
	}
	return true
}

func (r *typeResolver) rememberRecordTypedef(id, name string, alignment uint64) {
	for i := range r.records {
		if r.records[i].Name != r.spellings[id] {
			continue
		}
		aliases := &r.records[i].TypedefAlignments
		for _, alias := range *aliases {
			if alias.Name == name {
				return
			}
		}
		*aliases = append(*aliases, RecordTypedefAlignment{Name: name, Aligned: alignment})
		sort.Slice(*aliases, func(i, j int) bool { return (*aliases)[i].Name < (*aliases)[j].Name })
		return
	}
}

func validateRecordTypedefs(r Record) error {
	if len(r.TypedefAlignments) > maxRecords {
		return fmt.Errorf("clang: too many aligned record typedefs in %s", r.Name)
	}
	previous := ""
	for _, alias := range r.TypedefAlignments {
		if !cIdentifier(alias.Name) || alias.Name <= previous || alias.Aligned == 0 || !validAlignment(alias.Aligned) {
			return fmt.Errorf("clang: invalid record typedef alignment in %s", r.Name)
		}
		previous = alias.Name
	}
	return nil
}

// Canonical body adapters are safe only when alias alignment leaves the actual
// calling ABI unchanged. Check the producer's LLVM lowering, including MSVC
// i386 conventions, rather than using alias alignment as argument alignment.
func (h *Header) probeRecordTypedefABI(ctx context.Context, compiler, path string, flags []string) error {
	var source strings.Builder
	var pairs [][2]string
	conventions := []string{""}
	if h.Target.OS == "windows" && h.Target.Arch == "386" {
		conventions = append(conventions, "__attribute__((stdcall)) ", "__attribute__((fastcall)) ")
	}
	for _, r := range h.Records {
		for _, alias := range r.TypedefAlignments {
			for _, cc := range conventions {
				body, name := fmt.Sprintf("dylib_record_typedef_body_%d", len(pairs)), fmt.Sprintf("dylib_record_typedef_alias_%d", len(pairs))
				// Explicit assembler names keep i386 stdcall/fastcall decoration
				// out of the comparison without changing their calling convention.
				fmt.Fprintf(&source, "%s%s %s(int,%s) __asm__(%q);\n", cc, r.Name, body, r.Name, body)
				fmt.Fprintf(&source, "%s%s %s(int,%s) __asm__(%q);\n", cc, alias.Name, name, alias.Name, name)
				fmt.Fprintf(&source, "%s%s %s(int marker,%s value) { (void)marker; return value; }\n", cc, r.Name, body, r.Name)
				fmt.Fprintf(&source, "%s%s %s(int marker,%s value) { (void)marker; return value; }\n", cc, alias.Name, name, alias.Name)
				pairs = append(pairs, [2]string{body, name})
			}
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	args := append(append([]string(nil), flags...), "-x", "c", "-std=c17", "-include", path, "-O0", "-S", "-emit-llvm", "-o", "-", "-")
	data, err := runCompiler(ctx, compiler, args, source.String(), maxASTSize)
	if err != nil {
		return err
	}
	entry := func(name string) string {
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "define ") {
				continue
			}
			for _, marker := range []string{"@" + name + "(", "@\"\\01" + name + "\"("} {
				if strings.Contains(line, marker) {
					// Attribute-group numbers describe function bodies, not the
					// physical argument/result ABI being qualified here.
					line, _, _ = strings.Cut(strings.TrimSuffix(line, " {"), " #")
					return strings.Replace(line, marker, "@entry(", 1)
				}
			}
		}
		return ""
	}
	for _, pair := range pairs {
		if body, alias := entry(pair[0]), entry(pair[1]); body == "" || body != alias {
			return fmt.Errorf("clang: aligned record typedef has a different calling ABI from its body: %s; %s", body, alias)
		}
	}
	return nil
}

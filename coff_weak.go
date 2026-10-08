package dylib

import (
	"fmt"
	"strings"
)

func weakAliases(objects []*object) (map[string]definition, error) {
	aliases := make(map[string]definition)
	for _, o := range objects {
		for i, v := range o.symbols {
			if v.alias == nil {
				continue
			}
			if v.alias.target < 0 || v.alias.target >= len(o.symbols) {
				return nil, fmt.Errorf("invalid COFF weak fallback index")
			}
			if old, ok := aliases[v.name]; ok {
				a := old.sym().alias
				if a.search != v.alias.search || old.o.symbols[a.target].name != o.symbols[v.alias.target].name {
					return nil, fmt.Errorf("conflicting COFF weak fallbacks for %s", v.name)
				}
				continue
			}
			aliases[v.name] = definition{o, i}
		}
	}
	return aliases, nil
}

func (im *image) externalSymbol(o *object, name string) uintptr {
	if o.info.Format == "COFF" {
		name = strings.TrimPrefix(name, "__imp_")
	}
	if p := im.hooks[name]; p != 0 {
		return p
	}
	if im.external != nil {
		return im.external(name)
	}
	return 0
}

// resolveDefinition is shared by address and section-relative relocations.
// A strong definition or explicitly provided native symbol wins over a COFF
// weak fallback. Cycles are detected after archive selection has stabilized.
func (im *image) resolveDefinition(o *object, index int) (definition, error) {
	seen := make(map[definition]bool)
	for {
		if index < 0 || index >= len(o.symbols) {
			return definition{}, fmt.Errorf("invalid symbol index %d", index)
		}
		d := definition{o, index}
		v := d.sym()
		if v.global {
			if strong, ok := im.defs[v.name]; ok {
				return strong, nil
			}
		}
		if v.section != 0 || im.externalSymbol(o, v.name) != 0 {
			return d, nil
		}
		alias, ok := im.aliases[v.name]
		if !ok {
			return d, nil
		}
		if seen[alias] {
			return definition{}, fmt.Errorf("COFF weak alias cycle at %s", v.name)
		}
		seen[alias] = true
		o, index = alias.o, alias.sym().alias.target
	}
}

// SEARCH_LIBRARY tries archive definitions of the weak name before its
// fallback. NOLIBRARY and ALIAS may use an already selected strong definition,
// but do not extract archive members just to define that weak name.
func (im *image) weakLibraryNames(names []string) []string {
	var wanted []string
	seen := make(map[string]bool)
	for _, name := range names {
		for !seen[name] {
			seen[name] = true
			d, ok := im.aliases[name]
			if !ok {
				break
			}
			if _, defined := im.defs[name]; defined || im.externalSymbol(d.o, name) != 0 {
				break
			}
			a := d.sym().alias
			if a.search == 2 {
				wanted = append(wanted, name)
			}
			name = d.o.symbols[a.target].name
		}
	}
	return wanted
}

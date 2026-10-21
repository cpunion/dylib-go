package cxxexample

import (
	"context"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
)

// Discover uses compiler-confirmed inheritance among selected class names.
// The application still selects members, native ownership and C export names.
func Discover(ctx context.Context, header string, opts cxx.DiscoveryOptions) (*cxx.Facade, error) {
	classes := []cxx.Class{{
		Name: "example::Counter",
		Methods: []cxx.Method{{Name: "score", Symbol: "discovered_score", Const: true,
			Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}}}},
	}, {
		Name: "example::Derived", Delete: "discovered_derived_delete",
		Constructors: []cxx.Constructor{{Symbol: "discovered_derived_new", Args: []abi.Type{abi.I32}}},
	}, {
		Name: "example::Diamond", Delete: "discovered_diamond_delete",
		Constructors: []cxx.Constructor{{Symbol: "discovered_diamond_new", Args: []abi.Type{abi.I32}}},
	}, {Name: "example::VirtualLeft"}}
	names := make([]string, len(classes))
	for i := range classes {
		names[i] = classes[i].Name
	}
	relations, err := cxx.DiscoverBases(ctx, header, names, opts)
	if err != nil {
		return nil, err
	}
	short := func(name string) string { return strings.ToLower(strings.TrimPrefix(name, "example::")) }
	for _, relation := range relations {
		for i := range classes {
			if classes[i].Name == relation.Derived {
				classes[i].Bases = append(classes[i].Bases, cxx.Base{
					Name: relation.Base, Symbol: "discovered_" + short(relation.Derived) + "_" + short(relation.Base),
				})
			}
		}
	}
	return cxx.Generate(header, classes)
}

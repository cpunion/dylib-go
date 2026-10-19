package cxxexample

import (
	"context"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/compiler/cxx"
)

// DiscoverMembers selects public scalar operations from the executed members.hpp
// fixture. ABI types, ownership and export names remain application decisions.
func DiscoverMembers(ctx context.Context, header string, opts cxx.DiscoveryOptions) (*cxx.Facade, error) {
	declaration, err := cxx.DiscoverMembers(ctx, header, "members::Alias", opts)
	if err != nil {
		return nil, err
	}
	class := cxx.Class{Name: declaration.Name}
	for _, call := range declaration.Callables {
		if call.Deleted || call.Variadic {
			continue
		}
		if call.Kind == cxx.DestructorDeclaration {
			class.Delete = "member_delete"
			continue
		}
		if len(call.Parameters) != 1 || call.Parameters[0].NativeType != "int" {
			continue
		}
		switch {
		case call.Kind == cxx.ConstructorDeclaration:
			class.Constructors = append(class.Constructors, cxx.Constructor{Symbol: "member_new", Args: []abi.Type{abi.I32}})
		case call.Kind == cxx.MethodDeclaration && call.Name == "add" && strings.HasSuffix(call.NativeType, " const"):
			class.Methods = append(class.Methods, cxx.Method{Name: call.Name, Symbol: "member_add", Const: true,
				Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}}})
		case call.Kind == cxx.MethodDeclaration && call.Name == "twice" && call.Static:
			class.Methods = append(class.Methods, cxx.Method{Name: call.Name, Symbol: "member_twice", Static: true,
				Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}}})
		}
	}
	if class.Delete == "" || len(class.Constructors) != 1 || len(class.Methods) != 2 {
		return nil, fmt.Errorf("expected one public int constructor, destructor, const add(int) and static twice(int)")
	}
	return cxx.Generate(header, []cxx.Class{class})
}

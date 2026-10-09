package clang

import (
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// FunctionPointer describes an address at a function boundary. Position is a
// zero-based argument index, or -1 for the result. It does not own an address,
// callback lease or native registration.
type FunctionPointer struct {
	Position  int
	Signature abi.Signature
}

// LookupFunctionPointer returns an independent signature snapshot. Use a
// declaration returned by ForHost before creating callbacks or calling addresses.
func (d Declaration) LookupFunctionPointer(position int) (FunctionPointer, error) {
	if err := (&Header{Functions: []Declaration{d}}).validateDescriptions(); err != nil {
		return FunctionPointer{}, err
	}
	for _, pointer := range d.FunctionPointers {
		if pointer.Position == position {
			pointer.Signature = pointer.Signature.Clone()
			return pointer, nil
		}
	}
	return FunctionPointer{}, fmt.Errorf("clang: no function pointer at position %d", position)
}

// WithTail describes a concrete call to a variadic native function pointer.
// Variadic Go callback entries remain unsupported by abi.NewCallback.
func (p FunctionPointer) WithTail(types ...abi.Type) (FunctionPointer, error) {
	d, err := (Declaration{Signature: p.Signature}).WithTail(types...)
	if err != nil {
		return FunctionPointer{}, err
	}
	p.Signature = d.Signature
	return p, nil
}

func (d Declaration) clone() Declaration {
	d.Signature = d.Signature.Clone()
	d.FunctionPointers = append([]FunctionPointer(nil), d.FunctionPointers...)
	for i := range d.FunctionPointers {
		d.FunctionPointers[i].Signature = d.FunctionPointers[i].Signature.Clone()
	}
	return d
}

func functionProbes(names []string) string {
	var source strings.Builder
	source.WriteString(sizeProbe)
	for i, name := range names {
		fmt.Fprintf(&source, "typedef __typeof__(%s) dylib_function_probe_%d;\n", name, i)
	}
	return source.String()
}

// Clang emits both the written and effective types for attributes and decayed
// parameters. The final type child preserves the actual convention/adjustment.
func (r *typeResolver) effectiveType(node astNode) (astNode, error) {
	for depth := 0; depth <= 16; depth++ {
		if attr := r.aliasAttributes[unqualified(node.Type.QualType)]; attr != "" {
			return astNode{}, fmt.Errorf("unsupported typedef attribute %s", attr)
		}
		switch node.Kind {
		case "TypedefDecl", "TypedefType", "ParenType", "AttributedType", "ElaboratedType", "TypeOfExprType", "QualType", "DecayedType", "AdjustedType":
			found := false
			for i := len(node.Inner) - 1; i >= 0; i-- {
				if strings.HasSuffix(node.Inner[i].Kind, "Type") {
					node, found = node.Inner[i], true
					break
				}
			}
			if !found {
				return astNode{}, fmt.Errorf("missing compiler type node")
			}
		default:
			return node, nil
		}
	}
	return astNode{}, fmt.Errorf("compiler type nesting exceeds 16 levels")
}

func callingConvention(cc string, target Target) (abi.Convention, error) {
	var convention abi.Convention
	switch cc {
	case "cdecl":
		convention = abi.CDecl
	case "stdcall":
		convention = abi.StdCall
	case "fastcall":
		convention = abi.FastCall
	default:
		return 0, fmt.Errorf("unsupported function calling convention %q", cc)
	}
	if convention != abi.CDecl && (target.OS != "windows" || target.Arch != "386") {
		return 0, fmt.Errorf("calling convention requires Windows 386")
	}
	return convention, nil
}

func (r *typeResolver) prototype(node astNode, target Target, functionPointers bool) (abi.Signature, []FunctionPointer, error) {
	if node.Kind != "FunctionProtoType" || len(node.Inner) == 0 {
		return abi.Signature{}, nil, fmt.Errorf("C17 calls require an explicit prototype")
	}
	convention, err := callingConvention(node.CC, target)
	if err != nil {
		return abi.Signature{}, nil, err
	}
	if len(node.Inner) > 257 {
		return abi.Signature{}, nil, fmt.Errorf("at most 256 declared parameters are supported")
	}
	signature := abi.Signature{Convention: convention, Variadic: node.Variadic}
	var pointers []FunctionPointer
	var arguments []abi.TypeDesc
	complex := false
	for i, child := range node.Inner {
		description, pointer, err := r.boundaryType(child, target, functionPointers)
		if err != nil {
			return abi.Signature{}, nil, err
		}
		if pointer != nil {
			pointers = append(pointers, FunctionPointer{Position: i - 1, Signature: *pointer})
		}
		if i == 0 {
			signature.Result = description.Type
			if complexDescription(description) {
				signature.ResultType = &description
			}
		} else {
			signature.Args = append(signature.Args, description.Type)
			arguments = append(arguments, description)
			complex = complex || complexDescription(description)
		}
	}
	if complex {
		signature.ArgTypes = arguments
	}
	if signature.Variadic {
		signature.FixedArgs = len(signature.Args)
	}
	return signature, pointers, signature.Validate()
}

func (r *typeResolver) boundaryType(node astNode, target Target, functionPointers bool) (abi.TypeDesc, *abi.Signature, error) {
	effective, err := r.effectiveType(node)
	if err != nil {
		return abi.TypeDesc{}, nil, err
	}
	if effective.Kind == "PointerType" && len(effective.Inner) == 1 {
		pointee, err := r.effectiveType(effective.Inner[0])
		if err != nil {
			return abi.TypeDesc{}, nil, err
		}
		if pointee.Kind == "FunctionProtoType" || pointee.Kind == "FunctionNoProtoType" {
			if !functionPointers {
				return abi.TypeDesc{}, nil, fmt.Errorf("nested function-pointer signatures require an adapter")
			}
			signature, _, err := r.prototype(pointee, target, false)
			return abi.TypeDesc{Type: abi.Pointer}, &signature, err
		}
	}
	description, err := r.describe(node.Type.QualType, true, 0)
	return description, nil, err
}

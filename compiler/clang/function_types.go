package clang

import (
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// FunctionPointer describes a native entry prototype. Position is a zero-based
// argument index or -1 for the result; on Record it is a zero-based field index.
// It does not own an address, callback lease or native registration.
type FunctionPointer struct {
	Position  int
	Signature abi.Signature
	// Indirection counts pointer layers to the entry. Zero means one for compatibility.
	Indirection int
	// FunctionPointers preserves prototypes within this entry's arguments/results.
	FunctionPointers []FunctionPointer
}

// LookupFunctionPointer returns an independent signature snapshot. Use a
// declaration returned by ForHost before creating callbacks or calling addresses.
func (d Declaration) LookupFunctionPointer(position int) (FunctionPointer, error) {
	if err := (&Header{Functions: []Declaration{d}}).validateDescriptions(); err != nil {
		return FunctionPointer{}, err
	}
	for _, pointer := range d.FunctionPointers {
		if pointer.Position == position {
			return pointer.clone(), nil
		}
	}
	return FunctionPointer{}, fmt.Errorf("clang: no function pointer at position %d", position)
}

// LookupFunctionPointer inspects a nested argument or result prototype.
func (p FunctionPointer) LookupFunctionPointer(position int) (FunctionPointer, error) {
	return (Declaration{Signature: p.Signature, FunctionPointers: p.FunctionPointers}).LookupFunctionPointer(position)
}

func (p FunctionPointer) pointerDepth() int {
	if p.Indirection == 0 {
		return 1
	}
	return p.Indirection
}

func (p FunctionPointer) clone() FunctionPointer {
	p.Signature = p.Signature.Clone()
	p.FunctionPointers = cloneFunctionPointers(p.FunctionPointers)
	return p
}

func cloneFunctionPointers(pointers []FunctionPointer) []FunctionPointer {
	if pointers == nil {
		return nil
	}
	result := make([]FunctionPointer, len(pointers))
	for i, pointer := range pointers {
		result[i] = pointer.clone()
	}
	return result
}

// WithTail describes a concrete call to a variadic native function pointer.
// Variadic Go callback entries remain unsupported by abi.NewCallback.
func (p FunctionPointer) WithTail(types ...abi.Type) (FunctionPointer, error) {
	d, err := (Declaration{Signature: p.Signature, FunctionPointers: p.FunctionPointers}).WithTail(types...)
	if err != nil {
		return FunctionPointer{}, err
	}
	p.Signature = d.Signature
	p.FunctionPointers = d.FunctionPointers
	return p, nil
}

func (d Declaration) clone() Declaration {
	d.Signature = d.Signature.Clone()
	d.FunctionPointers = cloneFunctionPointers(d.FunctionPointers)
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
		case "TypedefDecl", "TypedefType", "ParenType", "AttributedType", "MacroQualifiedType", "ElaboratedType", "TypeOfExprType", "QualType", "DecayedType", "AdjustedType":
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

func (r *typeResolver) prototype(node astNode, target Target, depth int) (abi.Signature, []FunctionPointer, error) {
	if depth > 8 {
		return abi.Signature{}, nil, fmt.Errorf("function-pointer nesting exceeds 8 levels")
	}
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
		description, pointer, err := r.boundaryType(child, target, depth)
		if err != nil {
			return abi.Signature{}, nil, err
		}
		if pointer != nil {
			pointer.Position = i - 1
			pointers = append(pointers, *pointer)
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

func (r *typeResolver) pointerPointee(node astNode) (astNode, int, error) {
	effective, err := r.effectiveType(node)
	if err != nil {
		return astNode{}, 0, err
	}
	pointee := effective
	layers := 0
	for pointee.Kind == "PointerType" && len(pointee.Inner) == 1 {
		layers++
		pointee, err = r.effectiveType(pointee.Inner[0])
		if err != nil {
			return astNode{}, 0, err
		}
	}
	return pointee, layers, nil
}

func (r *typeResolver) boundaryType(node astNode, target Target, depth int) (abi.TypeDesc, *FunctionPointer, error) {
	pointee, layers, err := r.pointerPointee(node)
	if err != nil {
		return abi.TypeDesc{}, nil, err
	}
	if layers != 0 && (pointee.Kind == "FunctionProtoType" || pointee.Kind == "FunctionNoProtoType") {
		if layers > 8 {
			return abi.TypeDesc{}, nil, fmt.Errorf("function-pointer indirection exceeds 8 levels")
		}
		signature, pointers, err := r.prototype(pointee, target, depth+1)
		pointer := &FunctionPointer{Signature: signature, FunctionPointers: pointers}
		if layers > 1 {
			pointer.Indirection = layers
		}
		return abi.TypeDesc{Type: abi.Pointer}, pointer, err
	}
	description, err := r.describe(node.Type.QualType, true, 0)
	return description, nil, err
}

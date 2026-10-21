package cxx

import (
	"fmt"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
)

// ObjectKind selects how a native class crosses a C++ call. The C facade always
// transports its address as an opaque pointer; Go never copies its object bytes.
type ObjectKind uint8

const (
	ObjectPointer ObjectKind = iota + 1
	ObjectReference
	ObjectValue
)

// ObjectType selects an exact named native type. ObjectPointer also accepts
// named scalar, void, enum, union, array and pointer pointees; no Go layout is
// required. Const qualifies the named pointee of a pointer/reference;
// top-level const values are unsupported. Value results are newly allocated and
// owned; pointer/reference results borrow the native API's lifetime. Const is a
// native signature contract, not a restriction enforced on Go address values.
type ObjectType struct {
	Name  string
	Kind  ObjectKind
	Const bool
}

// ObjectArg replaces a zero-based abi.Pointer argument with a named native type.
// References and values require non-null addresses. Value arguments copy the
// object in C++ and require a noexcept destructor; copy constructors may throw.
type ObjectArg struct {
	Index int
	Type  ObjectType
}

func (o ObjectType) native() (string, error) {
	if !qualifiedName(o.Name) || o.Kind < ObjectPointer || o.Kind > ObjectValue || o.Const && o.Kind == ObjectValue {
		return "", fmt.Errorf("invalid native object type %+v", o)
	}
	name := o.Name
	if o.Const {
		name = "const " + name
	}
	switch o.Kind {
	case ObjectPointer:
		name += " *"
	case ObjectReference:
		name += " &"
	}
	return name, nil
}

// Throwing temporary destructors could run after a new result is constructed but
// before its handle is published. Exclude that contract for by-value boundaries.
func valueDestructor(name, alias string) string {
	return fmt.Sprintf("using %s = %s;\nstatic_assert(noexcept(static_cast<%s *>(nullptr)->~%s()), \"class values require noexcept destruction\");\n", alias, name, alias, alias)
}

func nativeArguments(args []abi.Type, objects []ObjectArg, functions []FunctionArg) (parameters []string, types, values, checks string, err error) {
	parameters, _, _, err = arguments(args)
	if err != nil {
		return
	}
	kinds, names := make([]string, len(args)), make([]string, len(args))
	for i, arg := range args {
		kinds[i], _ = cType(arg)
		names[i] = fmt.Sprintf("p%d", i)
	}
	seen := make(map[int]bool)
	var validation strings.Builder
	for _, object := range objects {
		i, native := object.Index, object.Type
		if i < 0 || i >= len(args) || args[i] != abi.Pointer || seen[i] {
			err = fmt.Errorf("invalid or duplicate native object argument %d: expected a pointer slot", i)
			return
		}
		seen[i] = true
		kinds[i], err = native.native()
		if err != nil {
			return
		}
		pointee := native.Name
		if native.Const {
			pointee = "const " + pointee
		}
		names[i] = fmt.Sprintf("static_cast<%s *>(p%d)", pointee, i)
		if native.Kind != ObjectPointer {
			fmt.Fprintf(&validation, "if (!p%d) return DYLIB_CXX_INVALID_ARGUMENT;\n", i)
			names[i] = "*" + names[i]
		}
		if native.Kind == ObjectValue {
			validation.WriteString(valueDestructor(native.Name, fmt.Sprintf("dylib_value_%d", i)))
			names[i] = native.Name + "(" + names[i] + ")"
		}
	}
	for _, function := range functions {
		i := function.Index
		if i < 0 || i >= len(args) || args[i] != abi.Pointer || seen[i] {
			err = fmt.Errorf("invalid or duplicate native function argument %d: expected an exclusive pointer slot", i)
			return
		}
		seen[i] = true
		alias := fmt.Sprintf("dylib_function_%d", i)
		var declaration string
		declaration, err = functionAlias(function.Signature, alias)
		if err != nil {
			return
		}
		validation.WriteString(declaration)
		kinds[i] = alias
		names[i] = fmt.Sprintf("reinterpret_cast<%s>(p%d)", alias, i)
	}
	types, values, checks = strings.Join(kinds, ", "), strings.Join(names, ", "), validation.String()
	return
}

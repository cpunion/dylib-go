package clang

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
)

func (t *directTypes) addCallbacks(declarations []Declaration) error {
	t.callbackIndexes = make(map[string]int)
	var add func(FunctionPointer) error
	add = func(pointer FunctionPointer) error {
		s := pointer.Signature
		if s.Convention != abi.Default && s.Convention != abi.CDecl {
			return fmt.Errorf("clang: direct function pointers require a cdecl prototype")
		}
		if _, err := t.goType(s.ReturnType()); err != nil {
			return err
		}
		for i := range s.Args {
			if _, err := t.goType(s.ArgumentType(i)); err != nil {
				return err
			}
		}
		for _, child := range pointer.FunctionPointers {
			if err := add(child); err != nil {
				return err
			}
		}
		if t.callbackIndex(pointer) == -1 {
			t.callbackIndexes[callbackKey(pointer)] = len(t.callbacks)
			t.callbacks = append(t.callbacks, pointer)
		}
		return nil
	}
	for _, declaration := range declarations {
		for _, pointer := range declaration.FunctionPointers {
			if err := add(pointer); err != nil {
				return fmt.Errorf("clang: %s: %w", declaration.Name, err)
			}
		}
	}
	return nil
}

func (t *directTypes) callbackIndex(pointer FunctionPointer) int {
	if index, ok := t.callbackIndexes[callbackKey(pointer)]; ok {
		return index
	}
	return -1
}

// Reuse the deterministic declaration encoding instead of comparing every pair.
func callbackKey(pointer FunctionPointer) string {
	var key bytes.Buffer
	writeCallbackKey(&key, pointer)
	return key.String()
}

func writeCallbackKey(key *bytes.Buffer, pointer FunctionPointer) {
	signature := pointer.Signature
	// Concrete tails describe calls, not the native function-pointer prototype.
	if signature.Variadic {
		signature.Args = signature.Args[:signature.FixedArgs]
		if len(signature.ArgTypes) != 0 {
			signature.ArgTypes = signature.ArgTypes[:signature.FixedArgs]
		}
	}
	writeSignature(key, signature)
	children := append([]FunctionPointer(nil), pointer.FunctionPointers...)
	sort.Slice(children, func(i, j int) bool { return children[i].Position < children[j].Position })
	for _, child := range children {
		fmt.Fprintf(key, "[%d:%d:", child.Position, child.pointerDepth())
		writeCallbackKey(key, child)
		fmt.Fprint(key, "]")
	}
}

func (t *directTypes) callbackName(index int) string {
	return fmt.Sprintf("%sCallback%d", t.prefix, index)
}

func (t *directTypes) boundaryType(declaration Declaration, position int) (string, error) {
	for _, pointer := range declaration.FunctionPointers {
		if pointer.Position == position {
			index := t.callbackIndex(pointer)
			if index < 0 {
				return "", fmt.Errorf("clang: missing direct function-pointer prototype")
			}
			return strings.Repeat("*", pointer.pointerDepth()-1) + t.callbackName(index), nil
		}
	}
	if position == -1 {
		return t.goType(declaration.Signature.ReturnType())
	}
	return t.goType(declaration.Signature.ArgumentType(position))
}

func (t *directTypes) writeCallbacks(output *bytes.Buffer) {
	for i, pointer := range t.callbacks {
		signature := pointer.Signature
		declaration := Declaration{Signature: signature, FunctionPointers: pointer.FunctionPointers}
		var parameters []string
		count := len(signature.Args)
		if signature.Variadic {
			count = signature.FixedArgs
		}
		for j := 0; j < count; j++ {
			kind, _ := t.boundaryType(declaration, j)
			if signature.Variadic {
				kind = fmt.Sprintf("p%d %s", j, kind)
			}
			parameters = append(parameters, kind)
		}
		if signature.Variadic {
			parameters = append(parameters, "__llgo_va_list ...any")
		}
		result, _ := t.boundaryType(declaration, -1)
		name := t.callbackName(i)
		fmt.Fprintf(output, "\n// %s is a native C entry; retain its code and callback registration while invoking it.\n//llgo:type C\ntype %s func(%s) %s\n", name, name, strings.Join(parameters, ","), result)
	}
}

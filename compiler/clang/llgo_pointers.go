package clang

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

func (t *directTypes) addCallbacks(declarations []Declaration) error {
	t.callbackIndexes = make(map[string]int)
	for _, declaration := range declarations {
		for _, pointer := range declaration.FunctionPointers {
			s := pointer.Signature
			if s.Convention != abi.Default && s.Convention != abi.CDecl {
				return fmt.Errorf("clang: %s function pointer requires a cdecl prototype", declaration.Name)
			}
			if _, err := t.goType(s.ReturnType()); err != nil {
				return err
			}
			for i := range s.Args {
				if _, err := t.goType(s.ArgumentType(i)); err != nil {
					return err
				}
			}
			if t.callbackIndex(s) == -1 {
				t.callbackIndexes[callbackKey(s)] = len(t.callbacks)
				t.callbacks = append(t.callbacks, s)
			}
		}
	}
	return nil
}

func (t *directTypes) callbackIndex(signature abi.Signature) int {
	if index, ok := t.callbackIndexes[callbackKey(signature)]; ok {
		return index
	}
	return -1
}

// Reuse the deterministic declaration encoding instead of comparing every pair.
func callbackKey(signature abi.Signature) string {
	// Concrete tails describe calls, not the native function-pointer prototype.
	if signature.Variadic {
		signature.Args = signature.Args[:signature.FixedArgs]
		if len(signature.ArgTypes) != 0 {
			signature.ArgTypes = signature.ArgTypes[:signature.FixedArgs]
		}
	}
	var key bytes.Buffer
	writeSignature(&key, signature)
	return key.String()
}

func (t *directTypes) callbackName(index int) string {
	return fmt.Sprintf("%sCallback%d", t.prefix, index)
}

func (t *directTypes) boundaryType(declaration Declaration, position int) (string, error) {
	for _, pointer := range declaration.FunctionPointers {
		if pointer.Position == position {
			index := t.callbackIndex(pointer.Signature)
			if index < 0 {
				return "", fmt.Errorf("clang: missing direct function-pointer prototype")
			}
			return t.callbackName(index), nil
		}
	}
	if position == -1 {
		return t.goType(declaration.Signature.ReturnType())
	}
	return t.goType(declaration.Signature.ArgumentType(position))
}

func (t *directTypes) writeCallbacks(output *bytes.Buffer) {
	for i, signature := range t.callbacks {
		var parameters []string
		count := len(signature.Args)
		if signature.Variadic {
			count = signature.FixedArgs
		}
		for j := 0; j < count; j++ {
			kind, _ := t.goType(signature.ArgumentType(j))
			if signature.Variadic {
				kind = fmt.Sprintf("p%d %s", j, kind)
			}
			parameters = append(parameters, kind)
		}
		if signature.Variadic {
			parameters = append(parameters, "__llgo_va_list ...any")
		}
		result, _ := t.goType(signature.ReturnType())
		name := t.callbackName(i)
		fmt.Fprintf(output, "\n// %s is a native C entry; retain its code and callback registration while invoking it.\n//llgo:type C\ntype %s func(%s) %s\n", name, name, strings.Join(parameters, ","), result)
	}
}

//go:build !llgo && cgo && (linux || darwin || windows)

package recordcallbacks

import dylib "github.com/cpunion/dylib-go"

type Bindings = CBindings
type BindingsRecord0 = CBindingsRecord0
type BindingsRecord1 = CBindingsRecord1

var _BindingsDeclarations = _CBindingsDeclarations

func NewBindings(s *dylib.Session) (*Bindings, error) { return NewCBindings(s) }

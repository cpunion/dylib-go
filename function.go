package dylib

import "github.com/cpunion/llgo-dylib/abi"

// Function couples an explicit ABI signature with the lifetime of its code.
type Function struct {
	symbol    *Symbol
	signature abi.Signature
}

// Bind validates the signature and resolves the symbol once. No demangling or
// type guessing occurs; a header generator such as llcppg can supply metadata.
func (s *Session) Bind(name string, signature abi.Signature) (*Function, error) {
	if err := signature.Validate(); err != nil {
		return nil, err
	}
	if !abi.Available() {
		return nil, abi.ErrUnavailable
	}
	symbol, err := s.Resolve(name)
	if err != nil {
		return nil, err
	}
	signature.Args = append([]abi.Type(nil), signature.Args...)
	return &Function{symbol: symbol, signature: signature}, nil
}
func (f *Function) Call(args ...abi.Value) (abi.Value, error) {
	if f == nil {
		return abi.Value{}, ErrClosed
	}
	var result abi.Value
	err := f.symbol.WithAddress(func(address uintptr) error {
		var err error
		result, err = abi.Call(address, f.signature, args...)
		return err
	})
	return result, err
}

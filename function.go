package dylib

import "github.com/cpunion/dylib-go/abi"

// Function couples an explicit ABI signature with the lifetime of its code.
type Function struct {
	symbol *Symbol
	plan   *abi.CallPlan
}

// Bind validates the signature and resolves the symbol once. No demangling or
// type guessing occurs; a header generator such as llcppg can supply metadata.
func (s *Session) Bind(name string, signature abi.Signature) (*Function, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	plan, err := abi.Prepare(signature)
	if err != nil {
		return nil, err
	}
	address, err := s.lookup(name)
	if err != nil {
		plan.Close()
		return nil, err
	}
	s.plans = append(s.plans, plan)
	return &Function{symbol: &Symbol{owner: s, address: address}, plan: plan}, nil
}
func (f *Function) Call(args ...abi.Value) (abi.Value, error) {
	if f == nil {
		return abi.Value{}, ErrClosed
	}
	var result abi.Value
	err := f.symbol.WithAddress(func(address uintptr) error {
		var err error
		result, err = f.plan.Call(address, args...)
		return err
	})
	return result, err
}

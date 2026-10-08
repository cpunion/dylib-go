package dylib

import (
	"reflect"

	"github.com/cpunion/dylib-go/abi"
)

type preparedBinding struct {
	signature abi.Signature
	plan      *abi.CallPlan
}

// Function couples an explicit ABI signature with the lifetime of its code.
type Function struct {
	symbol *Symbol
	plan   *abi.CallPlan
}

// Bind validates the signature and resolves the symbol once. No demangling or
// type guessing occurs; a header generator such as llcppg can supply metadata.
// Exact signature snapshots share a session-owned plan across bindings. The
// symbol address is separate from the plan; no cache crosses session lifetimes.
func (s *Session) Bind(name string, signature abi.Signature) (*Function, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if len(signature.Args) == 0 {
		signature.Args = nil
	}
	if len(signature.ArgTypes) == 0 {
		signature.ArgTypes = nil
	}
	var plan *abi.CallPlan
	for _, cached := range s.plans {
		if reflect.DeepEqual(cached.signature, signature) {
			plan = cached.plan
			break
		}
	}
	fresh := plan == nil
	if fresh {
		var err error
		plan, err = abi.Prepare(signature)
		if err != nil {
			return nil, err
		}
	}
	address, err := s.lookup(name)
	if err != nil {
		if fresh {
			plan.Close()
		}
		return nil, err
	}
	if fresh {
		s.plans = append(s.plans, preparedBinding{signature.Clone(), plan})
	}
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

package dylib

import "github.com/cpunion/dylib-go/abi"

// Function couples an explicit ABI signature with the lifetime of its code.
type Function struct {
	symbol *Symbol
	plan   *abi.CallPlan
}

// Bind validates the signature and resolves the symbol once. No demangling or
// type guessing occurs; a header generator such as llcppg can supply metadata.
// Canonical logical signatures share a session-owned plan across bindings.
// Distinct logical signatures with compatible ABI shapes share native resources,
// preserving their validation and metadata. No cache crosses session lifetimes.
func (s *Session) Bind(name string, signature abi.Signature) (*Function, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if s.initErr != nil {
		return nil, s.initErr
	}
	key, err := bindingKey(signature)
	if err != nil {
		return nil, err
	}
	plan := s.plans[key]
	fresh := plan == nil
	var physicalKey string
	if fresh {
		shape, shapeErr := signature.ABIShape()
		if shapeErr != nil {
			return nil, shapeErr
		}
		physicalKey, err = bindingKey(shape)
		if err != nil {
			return nil, err
		}
		if owner := s.physicalPlans[physicalKey]; owner != nil {
			plan, err = owner.Share(signature)
		} else {
			plan, err = abi.Prepare(signature)
		}
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
		if s.plans == nil {
			s.plans = make(map[string]*abi.CallPlan)
		}
		s.plans[key] = plan
		if s.physicalPlans == nil {
			s.physicalPlans = make(map[string]*abi.CallPlan)
		}
		if s.physicalPlans[physicalKey] == nil {
			s.physicalPlans[physicalKey] = plan
		}
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

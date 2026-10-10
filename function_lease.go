package dylib

import "github.com/cpunion/dylib-go/abi"

// FunctionLease retains bound code and its session-owned ABI plan. It can call
// during session retirement, for example to unregister a retained callback.
// Close outside its own calls/callbacks, after joining native users. Do not copy it.
type FunctionLease struct {
	symbol *SymbolLease
	plan   *abi.CallPlan
}

func (f *Function) Acquire() (*FunctionLease, error) {
	if f == nil || f.plan == nil {
		return nil, ErrClosed
	}
	symbol, err := f.symbol.Acquire()
	if err != nil {
		return nil, err
	}
	return &FunctionLease{symbol: symbol, plan: f.plan}, nil
}

// Call invokes the retained bound signature. Calls may overlap or reenter; a
// closing lease rejects new calls and waits for existing calls before release.
func (l *FunctionLease) Call(args ...abi.Value) (abi.Value, error) {
	if l == nil || l.plan == nil {
		return abi.Value{}, ErrClosed
	}
	var result abi.Value
	err := l.symbol.WithAddress(func(address uintptr) error {
		var err error
		result, err = l.plan.Call(address, args...)
		return err
	})
	return result, err
}

// Close releases this lease without closing the function's session.
func (l *FunctionLease) Close() error {
	if l == nil {
		return nil
	}
	return l.symbol.Close()
}

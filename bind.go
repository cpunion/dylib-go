package dylib

import "github.com/cpunion/llgo-dylib/internal/native"

// Int32Func binds a known C ABI signature to an owning session. It avoids symbol
// lookup on each invocation and returns ErrClosed after its session is closed.
// It does not infer the signature from a symbol name or validate native code.
type Int32Func struct {
	owner   *Session
	address uintptr
}

func (s *Session) BindInt32(name string) (*Int32Func, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	return &Int32Func{owner: s, address: p}, nil
}

// Call invokes int32_t fn(int32_t,int32_t) while retaining the owner's memory.
// A native function must not re-enter this session's locking methods.
func (f *Int32Func) Call(a, b int32) (int32, error) {
	if f == nil || f.owner == nil {
		return 0, ErrClosed
	}
	f.owner.mu.Lock()
	defer f.owner.mu.Unlock()
	if f.owner.closed {
		return 0, ErrClosed
	}
	return native.CallInt32(f.address, a, b), nil
}

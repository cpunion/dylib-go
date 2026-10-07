package dylib

import "fmt"

// Symbol is a resolved native function or data symbol owned by a Session.
// It carries no type information or calling convention. Use WithAddress to
// access its address while preventing the session from closing.
type Symbol struct {
	owner   *Session
	address uintptr
}

// Resolve links on first use and resolves name once. The returned handle is
// valid until Close; neither the symbol name nor its address implies a type.
func (s *Session) Resolve(name string) (*Symbol, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	return &Symbol{owner: s, address: p}, nil
}

// WithAddress runs use while retaining the session's code and library handles.
// The callback must finish all address use before returning and must not
// re-enter this session's locking methods. Native calling conventions, types,
// pointer ownership and exception boundaries remain the adapter's responsibility.
// A nil or closed Symbol returns ErrClosed; a nil callback is an error.
func (s *Symbol) WithAddress(use func(uintptr) error) error {
	if s == nil || s.owner == nil {
		return ErrClosed
	}
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.owner.closed {
		return ErrClosed
	}
	if use == nil {
		return fmt.Errorf("dylib: nil symbol callback")
	}
	return use(s.address)
}

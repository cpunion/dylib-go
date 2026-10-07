package abi

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

var ErrCallbackClosed = errors.New("ABI callback is closed")

// CallbackFunc handles a native C call. Arguments are logical values; pointer
// bits denote borrowed native addresses, never temporary Go pointees. Captures
// must be safe for concurrent calls if native code invokes the entry concurrently.
// Return an error to supply a zero native result and record it in Callback.Err.
type CallbackFunc func(args []Value) (Value, error)

type callbackBackend interface {
	address() uintptr
	dispatch(*callbackState, unsafe.Pointer, unsafe.Pointer)
	close()
}

type callbackState struct {
	signature Signature
	handler   CallbackFunc
	backend   callbackBackend
	mu        sync.Mutex
	err       error
}

func (s *callbackState) record(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

// Callback owns a native C entry, its ABI layout, and its Go handler/captures.
// Acquire a lease before publishing its address. Unregister the entry and join
// native callers before releasing that lease. Close waits for leases before
// freeing code; it cannot revoke an address retained by uncooperative native code.
// No finalizer frees an entry at an arbitrary GC-selected time. Do not copy it.
type Callback struct {
	mu      sync.RWMutex
	state   *callbackState
	backend callbackBackend
}

// NewCallback snapshots one fixed C signature and builds a callable entry.
// Variadic callbacks are unsupported. Results cannot contain temporary pointees:
// use caller-owned native pointers, or return logical scalar/struct values.
func NewCallback(signature Signature, handler CallbackFunc) (*Callback, error) {
	if err := signature.Validate(); err != nil {
		return nil, err
	}
	if signature.Variadic {
		return nil, fmt.Errorf("variadic callbacks are unsupported")
	}
	if handler == nil {
		return nil, fmt.Errorf("nil callback handler")
	}
	state := &callbackState{signature: signature.Clone(), handler: handler}
	backend, err := prepareCallback(state)
	if err != nil {
		return nil, err
	}
	state.backend = backend
	return &Callback{state: state, backend: backend}, nil
}

// CallbackLease keeps an entry and its captures alive while native code may use
// its address. The address becomes invalid when the lease ends and the callback
// closes. Use must not race with lease release. Do not copy it.
type CallbackLease struct {
	mu      sync.Mutex
	owner   *Callback
	address uintptr
}

func (c *Callback) Acquire() (*CallbackLease, error) {
	if c == nil {
		return nil, ErrCallbackClosed
	}
	c.mu.RLock()
	if c.backend == nil {
		c.mu.RUnlock()
		return nil, ErrCallbackClosed
	}
	return &CallbackLease{owner: c, address: c.backend.address()}, nil
}

func (l *CallbackLease) Address() (uintptr, error) {
	if l == nil {
		return 0, ErrCallbackClosed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == nil {
		return 0, ErrCallbackClosed
	}
	return l.address, nil
}

// Close releases the lease after native code has stopped using the entry.
// It is idempotent; it does not close the callback itself.
func (l *CallbackLease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner != nil {
		l.owner.mu.RUnlock()
		l.owner, l.address = nil, 0
	}
	return nil
}

// WithAddress holds a lease for synchronous use. Native workers must finish and
// stored registrations must be removed before use returns. Do not acquire another
// lease or close this callback from its handler or use function.
func (c *Callback) WithAddress(use func(uintptr) error) error {
	if use == nil {
		return fmt.Errorf("nil callback address user")
	}
	lease, err := c.Acquire()
	if err != nil {
		return err
	}
	defer lease.Close()
	address, err := lease.Address()
	if err != nil {
		return err
	}
	return use(address)
}

// Err returns the first handler error, recovered panic, or invalid result. Native
// callers see a zero result on failure. Successful later calls do not clear this
// diagnostic; create a new callback for an independent error history.
func (c *Callback) Err() error {
	if c == nil || c.state == nil {
		return ErrCallbackClosed
	}
	state := c.state // State ownership remains stable after construction.
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.err
}

// Close waits for every lease, then releases the entry, types, and captures.
// Callers must unregister and join native callers before releasing their leases.
// Calling Close from a handler or from inside WithAddress would deadlock.
func (c *Callback) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backend != nil {
		c.backend.close()
		c.backend = nil
		c.state.backend, c.state.handler = nil, nil
		c.state.signature = Signature{}
	}
	return nil
}

func temporaryCallbackResult(v Value) bool {
	if v.Pointee != nil {
		return true
	}
	if v.Aggregate != nil {
		for _, field := range v.Aggregate.Fields {
			if temporaryCallbackResult(field) {
				return true
			}
		}
	}
	return false
}

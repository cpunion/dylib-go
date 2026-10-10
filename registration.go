package dylib

import (
	"errors"
	"fmt"
	"sync"

	"github.com/cpunion/dylib-go/abi"
)

// RegistrationResources lists every code, data and callback owner needed by a
// native registration and its stop operation. Owners remain independently owned;
// a Registration acquires its own leases, not ownership of the resources.
type RegistrationResources struct {
	Symbols   []*Symbol
	Functions []*Function
	Values    []*abi.NativeValue
	Callbacks []*abi.Callback
}

// RegistrationLeases contains borrowed leases in the same order as the resource
// lists. Do not close these leases yourself. Native addresses may be retained
// until the registration's stop operation completes successfully.
type RegistrationLeases struct {
	Symbols   []*SymbolLease
	Functions []*FunctionLease
	Values    []*abi.NativeValueLease
	Callbacks []*abi.CallbackLease
}

func (l RegistrationLeases) snapshot() RegistrationLeases {
	return RegistrationLeases{
		Symbols: append([]*SymbolLease(nil), l.Symbols...), Functions: append([]*FunctionLease(nil), l.Functions...),
		Values: append([]*abi.NativeValueLease(nil), l.Values...), Callbacks: append([]*abi.CallbackLease(nil), l.Callbacks...),
	}
}

func (l RegistrationLeases) release() {
	// Reverse acquisition order. Built-in lease releases are infallible.
	for i := len(l.Callbacks) - 1; i >= 0; i-- {
		l.Callbacks[i].Close()
	}
	for i := len(l.Values) - 1; i >= 0; i-- {
		l.Values[i].Close()
	}
	for i := len(l.Functions) - 1; i >= 0; i-- {
		l.Functions[i].Close()
	}
	for i := len(l.Symbols) - 1; i >= 0; i-- {
		l.Symbols[i].Close()
	}
}

type registrationClose struct {
	done chan struct{}
	err  error
}

// Registration owns the leases behind a retained native registration. Create it
// before publishing addresses, then register through WithLeases. Close retires
// Go use, runs the caller's unregister/join operation with every lease alive, and
// releases leases only after success. A failed stop retains resources for retry.
// No finalizer unregisters at an arbitrary time. Do not copy it.
type Registration struct {
	mu      sync.Mutex
	cond    *sync.Cond
	leases  RegistrationLeases
	stop    func(RegistrationLeases) error
	uses    int
	retired bool
	closed  bool
	attempt *registrationClose
}

// NewRegistration acquires all resources before native addresses are published.
// Acquisition failure rolls back earlier leases without calling stop. The stop
// function is mandatory: it must safely handle incomplete/absent registration,
// unregister native addresses and join their users, returning nil only when no
// native access remains. It runs even if WithLeases never published anything.
func NewRegistration(resources RegistrationResources, stop func(RegistrationLeases) error) (*Registration, error) {
	if stop == nil {
		return nil, fmt.Errorf("dylib: nil registration stop operation")
	}
	// Callback leases hold read locks. Reject duplicates before acquisition so a
	// pending callback close cannot block a second acquisition behind our first.
	seen := make(map[*abi.Callback]bool, len(resources.Callbacks))
	for i, callback := range resources.Callbacks {
		if seen[callback] {
			return nil, fmt.Errorf("dylib: duplicate registration callback at index %d", i)
		}
		seen[callback] = true
	}
	var leases RegistrationLeases
	keep := false
	defer func() {
		if !keep {
			leases.release()
		}
	}()
	for i, symbol := range resources.Symbols {
		lease, err := symbol.Acquire()
		if err != nil {
			return nil, fmt.Errorf("dylib: registration symbol %d: %w", i, err)
		}
		leases.Symbols = append(leases.Symbols, lease)
	}
	for i, function := range resources.Functions {
		lease, err := function.Acquire()
		if err != nil {
			return nil, fmt.Errorf("dylib: registration function %d: %w", i, err)
		}
		leases.Functions = append(leases.Functions, lease)
	}
	for i, value := range resources.Values {
		lease, err := value.Acquire()
		if err != nil {
			return nil, fmt.Errorf("dylib: registration value %d: %w", i, err)
		}
		leases.Values = append(leases.Values, lease)
	}
	for i, callback := range resources.Callbacks {
		lease, err := callback.Acquire()
		if err != nil {
			return nil, fmt.Errorf("dylib: registration callback %d: %w", i, err)
		}
		leases.Callbacks = append(leases.Callbacks, lease)
	}
	keep = true
	return &Registration{leases: leases, stop: stop}, nil
}

// WithLeases runs setup or a synchronous operation while preventing registration
// cleanup. It may overlap or reenter other operations; synchronize native state
// as required by the API. Slice snapshots may be modified, but the borrowed
// leases must not be closed. Close this registration outside its own operations,
// stop function and callbacks. A use error/panic does not unregister; call Close.
func (r *Registration) WithLeases(use func(RegistrationLeases) error) error {
	if r == nil {
		return ErrClosed
	}
	r.mu.Lock()
	if r.retired || r.stop == nil {
		r.mu.Unlock()
		return ErrClosed
	}
	if use == nil {
		r.mu.Unlock()
		return fmt.Errorf("dylib: nil registration lease user")
	}
	leases := r.leases.snapshot()
	r.uses++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.uses--
		if r.uses == 0 && r.cond != nil {
			r.cond.Broadcast()
		}
		r.mu.Unlock()
	}()
	return use(leases)
}

// Close rejects new uses, waits for existing operations, then unregisters and
// joins native users before releasing leases. Concurrent callers share one stop
// attempt and its result. On failure (or panic) resources remain retained and a
// later Close retries stop; WithLeases remains retired. A stop panic is preserved.
func (r *Registration) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.closed || r.stop == nil {
		r.mu.Unlock()
		return nil
	}
	if attempt := r.attempt; attempt != nil {
		r.mu.Unlock()
		<-attempt.done
		return attempt.err
	}
	attempt := &registrationClose{done: make(chan struct{})}
	r.attempt, r.retired = attempt, true
	for r.uses != 0 {
		if r.cond == nil {
			r.cond = sync.NewCond(&r.mu)
		}
		r.cond.Wait()
	}
	leases, stop := r.leases.snapshot(), r.stop
	r.mu.Unlock()
	finished := false
	defer func() {
		if !finished {
			// Preserve abnormal exit while waking other closers and retaining all
			// resources. The caller can repair its stop operation and retry.
			r.finishClose(attempt, errors.New("dylib: registration stop did not return normally"))
		}
	}()
	err := stop(leases)
	if err == nil {
		leases.release()
	}
	r.finishClose(attempt, err)
	finished = true
	return err
}

func (r *Registration) finishClose(attempt *registrationClose, err error) {
	r.mu.Lock()
	if err == nil {
		r.leases, r.stop, r.closed = RegistrationLeases{}, nil, true
	}
	attempt.err, r.attempt = err, nil
	close(attempt.done)
	r.mu.Unlock()
}

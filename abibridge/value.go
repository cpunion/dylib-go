package abibridge

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	dylib "github.com/ramblelabs/dylib-go"
)

// ErrValueUnsupported indicates a version 1 facade without value witnesses.
var ErrValueUnsupported = errors.New("abibridge: managed values require a version 2 native facade")

type valueEntries struct {
	prepare, releaseType, newValue, pointer, commit, release uintptr
}

// ValueLayout is a Swift value witness description, independent of C layouts.
// Empty Swift values can have Size zero and a positive Stride.
type ValueLayout struct{ Size, Stride, Alignment uint64 }

// ValueType retains compiler-provided complete metadata and its image through
// its Runtime. It only supports Copyable types. Do not copy it. Close the Runtime
// before closing the metadata provider's session.
type ValueType struct {
	owner    *Runtime
	provider *dylib.SymbolLease
	plan     uintptr
	layout   ValueLayout
}

// PrepareValueType calls a C-ABI metadata provider: const void *metadata(void).
// The caller must establish that it returns valid complete Swift metadata for a
// Copyable type. A null result is rejected; arbitrary or incomplete metadata is
// not safe to inspect. The provider's image and dependencies remain leased until
// Runtime.Close. Providers containing Swift types still need KeepLibraries.
func (r *Runtime) PrepareValueType(provider *dylib.Symbol) (*ValueType, error) {
	if err := r.begin(); err != nil {
		return nil, err
	}
	defer r.end()
	if r.valueOps.prepare == 0 {
		return nil, ErrValueUnsupported
	}
	lease, err := provider.Acquire()
	if err != nil {
		return nil, err
	}
	var plan uintptr
	var layout ValueLayout
	err = lease.WithAddress(func(address uintptr) error {
		plan, layout, err = nativeValueType(r.valueOps.prepare, address)
		return err
	})
	if err != nil {
		lease.Close()
		return nil, err
	}
	t := &ValueType{owner: r, provider: lease, plan: plan, layout: layout}
	r.mu.Lock()
	closed := r.closed
	if !closed {
		if r.valueTypes == nil {
			r.valueTypes = make(map[*ValueType]bool)
		}
		r.valueTypes[t] = true
	}
	r.mu.Unlock()
	if closed {
		nativeRelease(r.valueOps.releaseType, plan)
		lease.Close()
		return nil, ErrClosed
	}
	return t, nil
}

// Layout returns immutable metadata, also after retirement. It reads no native
// memory and does not keep the Runtime open.
func (t *ValueType) Layout() ValueLayout {
	if t == nil {
		return ValueLayout{}
	}
	return t.layout
}

// New allocates uninitialized native storage and lends its address to initialize.
// The adapter must initialize exactly one matching Swift value on success, and
// leave storage uninitialized on error. It must not panic after initialization
// or retain the borrowed address. Initialization runs without Go mutexes held.
// For a retained address use the resulting Value's Acquire method.
func (t *ValueType) New(initialize func(uintptr) error) (*Value, error) {
	if t == nil || t.owner == nil {
		return nil, ErrClosed
	}
	r := t.owner
	if err := r.begin(); err != nil {
		return nil, err
	}
	defer r.end()
	if initialize == nil {
		return nil, fmt.Errorf("abibridge: nil Swift value initializer")
	}
	handle, err := nativeValueNew(r.valueOps.newValue, t.plan, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if handle != 0 {
			nativeRelease(r.valueOps.release, handle)
		}
	}()
	if err := initialize(uintptr(nativeValuePointer(r.valueOps.pointer, handle))); err != nil {
		return nil, err
	}
	nativeRelease(r.valueOps.commit, handle)
	value, err := r.publishValue(t, handle)
	handle = 0 // publishValue either owns or releases the initialized value.
	return value, err
}

func (r *Runtime) publishValue(t *ValueType, handle uintptr) (*Value, error) {
	v := &Value{owner: r, valueType: t, handle: handle, done: make(chan struct{})}
	v.cond = sync.NewCond(&v.mu)
	r.mu.Lock()
	closed := r.closed
	if !closed {
		if r.values == nil {
			r.values = make(map[*Value]bool)
		}
		r.values[v] = true
	}
	r.mu.Unlock()
	if closed {
		nativeRelease(r.valueOps.release, handle)
		return nil, ErrClosed
	}
	return v, nil
}

// Value owns one initialized native Swift value. Clone uses the copy witness;
// Close uses the destroy witness exactly once. Representations are never exposed
// as Go byte slices. Do not copy a Value or close it from its own native adapter.
// Native access needs caller synchronization, including for non-Sendable types.
// Runtime.Close retires values automatically, after all operations/leases finish.
type Value struct {
	mu        sync.Mutex
	cond      *sync.Cond
	owner     *Runtime
	valueType *ValueType
	handle    uintptr
	uses      int
	closing   bool
	done      chan struct{}
}

func (v *Value) borrow() (unsafe.Pointer, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closing || v.handle == 0 {
		return nil, ErrClosed
	}
	v.uses++
	return nativeValuePointer(v.owner.valueOps.pointer, v.handle), nil
}
func (v *Value) releaseUse() {
	v.mu.Lock()
	v.uses--
	v.cond.Broadcast()
	v.mu.Unlock()
}

// Clone initializes independent native storage using this type's copy witness.
// The source remains initialized. A concurrent Close waits for the copy.
func (v *Value) Clone() (*Value, error) {
	if v == nil || v.owner == nil {
		return nil, ErrClosed
	}
	r := v.owner
	if err := r.begin(); err != nil {
		return nil, err
	}
	defer r.end()
	address, err := v.borrow()
	if err != nil {
		return nil, err
	}
	defer v.releaseUse()
	handle, err := nativeValueNew(r.valueOps.newValue, v.valueType.plan, uintptr(address))
	if err != nil {
		return nil, err
	}
	return r.publishValue(v.valueType, handle)
}

// WithAddress borrows initialized native storage until use returns. Native code
// must not destroy/consume the value, retain its storage address, or race other
// access. No Go mutex is held across use. Runtime/value Close waits for the
// adapter to finish.
func (v *Value) WithAddress(use func(uintptr) error) error {
	if v == nil || v.owner == nil {
		return ErrClosed
	}
	if err := v.owner.begin(); err != nil {
		return err
	}
	defer v.owner.end()
	if use == nil {
		return fmt.Errorf("abibridge: nil Swift value adapter")
	}
	address, err := v.borrow()
	if err != nil {
		return err
	}
	defer v.releaseUse()
	return use(uintptr(address))
}

// Acquire retains value storage, metadata and the facade until lease Close.
// Unregister/join native users before release. Existing leases remain usable
// while Value or Runtime retirement rejects new operations.
func (v *Value) Acquire() (*ValueLease, error) {
	if v == nil || v.owner == nil {
		return nil, ErrClosed
	}
	if err := v.owner.begin(); err != nil {
		return nil, err
	}
	address, err := v.borrow()
	if err != nil {
		v.owner.end()
		return nil, err
	}
	l := &ValueLease{owner: v, pointer: address, done: make(chan struct{})}
	l.cond = sync.NewCond(&l.mu)
	return l, nil
}

// Close retires this value, waits for native users and destroys it. It is
// idempotent. A caller holding a lease must release that lease before waiting
// for Close. It cannot be called from this value's own active adapter.
func (v *Value) Close() error {
	if v == nil || v.owner == nil {
		return nil
	}
	r := v.owner
	// Keep the value in the owner's map until destruction completes. Runtime
	// retirement either snapshots it and waits in close, or observes completed
	// removal. This also permits one destructor callback to close another value.
	v.close()
	r.mu.Lock()
	delete(r.values, v)
	r.mu.Unlock()
	return nil
}
func (v *Value) close() {
	v.mu.Lock()
	if v.closing {
		done := v.done
		v.mu.Unlock()
		<-done
		return
	}
	v.closing = true
	for v.uses != 0 {
		v.cond.Wait()
	}
	handle := v.handle
	v.handle = 0
	v.mu.Unlock()
	nativeRelease(v.owner.valueOps.release, handle)
	close(v.done)
}

// ValueLease retains borrowed storage and its native code. Do not copy it.
// Address use must stop before Close; WithAddress guards scoped native calls.
type ValueLease struct {
	mu      sync.Mutex
	cond    *sync.Cond
	owner   *Value
	pointer unsafe.Pointer
	uses    int
	closing bool
	done    chan struct{}
}

// Address returns leased native storage. Native users must stop before Close.
func (l *ValueLease) Address() (uintptr, error) {
	if l == nil {
		return 0, ErrClosed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == nil || l.closing {
		return 0, ErrClosed
	}
	return uintptr(l.pointer), nil
}

// Pointer returns the same leased native storage for typed cgo/llgo adapters.
// Its lifetime is identical to Address; remove native users before Close.
func (l *ValueLease) Pointer() (unsafe.Pointer, error) {
	if l == nil {
		return nil, ErrClosed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == nil || l.closing {
		return nil, ErrClosed
	}
	return l.pointer, nil
}

func (l *ValueLease) WithAddress(use func(uintptr) error) error {
	if l == nil {
		return ErrClosed
	}
	l.mu.Lock()
	if l.owner == nil || l.closing {
		l.mu.Unlock()
		return ErrClosed
	}
	if use == nil {
		l.mu.Unlock()
		return fmt.Errorf("abibridge: nil Swift value lease adapter")
	}
	l.uses++
	address := uintptr(l.pointer)
	l.mu.Unlock()
	defer func() { l.mu.Lock(); l.uses--; l.cond.Broadcast(); l.mu.Unlock() }()
	return use(address)
}

// Close retires the lease and waits for its scoped adapters before releasing
// storage and native-code retention. Do not call it from its own adapter.
func (l *ValueLease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if l.owner == nil || l.closing {
		done := l.done
		l.mu.Unlock()
		if done != nil {
			<-done
		}
		return nil
	}
	l.closing = true
	for l.uses != 0 {
		l.cond.Wait()
	}
	owner := l.owner
	l.owner = nil
	l.pointer = nil
	l.mu.Unlock()
	owner.releaseUse()
	owner.owner.end()
	close(l.done)
	return nil
}

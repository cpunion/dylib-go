package dylib

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
)

func TestRegistrationValidationAndAcquisitionRollback(t *testing.T) {
	if _, err := NewRegistration(RegistrationResources{}, nil); err == nil {
		t.Fatal("nil stop accepted")
	}
	stop := func(RegistrationLeases) error { t.Error("stop called during acquisition failure"); return nil }
	session := New(Options{})
	defer session.Close()
	symbol := &Symbol{owner: session, address: 1}
	for _, resources := range []RegistrationResources{
		{Symbols: []*Symbol{symbol, nil}},
		{Symbols: []*Symbol{symbol}, Functions: []*Function{nil}},
		{Symbols: []*Symbol{symbol}, Values: []*abi.NativeValue{nil}},
		{Symbols: []*Symbol{symbol}, Callbacks: []*abi.Callback{nil}},
	} {
		if r, err := NewRegistration(resources, stop); r != nil || err == nil {
			t.Fatal("invalid resource accepted:", r, err)
		}
		session.mu.Lock()
		retained := session.calls
		session.mu.Unlock()
		if retained != 0 {
			t.Fatal("acquisition leaked code lease:", retained)
		}
	}
	callback := &abi.Callback{}
	if _, err := NewRegistration(RegistrationResources{Callbacks: []*abi.Callback{callback, callback}}, stop); err == nil {
		t.Fatal("duplicate callback accepted")
	}
	for _, r := range []*Registration{nil, {}} {
		if err := r.WithLeases(nil); !errors.Is(err, ErrClosed) {
			t.Fatal("unbound registration:", err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []*Function{nil, {}} {
		if _, err := f.Acquire(); !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	}
	for _, l := range []*FunctionLease{nil, {}} {
		if _, err := l.Call(); !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistrationStopFailureRetainsResourcesForRetry(t *testing.T) {
	session := New(Options{})
	defer session.Close()
	symbol := &Symbol{owner: session, address: 42}
	want := errors.New("native users are still running")
	calls := 0
	resources := RegistrationResources{Symbols: []*Symbol{symbol}}
	r, err := NewRegistration(resources, func(leases RegistrationLeases) error {
		calls++
		if address, err := leases.Symbols[0].Address(); address != 42 || err != nil {
			t.Error("stop lost code:", address, err)
		}
		if calls == 1 {
			return want
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	resources.Symbols[0] = nil // Input and operation slices are independent.
	var borrowed *SymbolLease
	if err := r.WithLeases(func(leases RegistrationLeases) error {
		borrowed = leases.Symbols[0]
		leases.Symbols[0] = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.WithLeases(nil); err == nil {
		t.Fatal("nil use accepted")
	}
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	waitSessionRetired(t, session)
	if err := r.Close(); !errors.Is(err, want) {
		t.Fatal("stop error lost:", err)
	}
	if address, err := borrowed.Address(); address != 42 || err != nil {
		t.Fatal("failed stop released code:", address, err)
	}
	if err := r.WithLeases(func(RegistrationLeases) error { t.Error("retired registration ran use"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	select {
	case <-closed:
		t.Fatal("session released after failed unregister")
	default:
	}
	if err := r.Close(); err != nil || calls != 2 {
		t.Fatal("retry:", calls, err)
	}
	if _, err := borrowed.Address(); !errors.Is(err, ErrClosed) {
		t.Fatal("successful stop kept lease open:", err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	r.Close()
	if calls != 2 {
		t.Fatal("closed registration repeated stop")
	}
}

func waitRegistrationRetired(t *testing.T, r *Registration) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.mu.Lock()
		retired := r.retired
		r.mu.Unlock()
		if retired {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("registration did not retire")
		}
		runtime.Gosched()
	}
}

func TestRegistrationConcurrentOperationsCloseAndJoin(t *testing.T) {
	var stops atomic.Int32
	stopping, joined := make(chan struct{}), make(chan struct{})
	r, err := NewRegistration(RegistrationResources{}, func(RegistrationLeases) error {
		stops.Add(1)
		close(stopping)
		<-joined
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce, joinOnce sync.Once
	defer joinOnce.Do(func() { close(joined) })
	defer releaseOnce.Do(func() { close(release) })
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- r.WithLeases(func(RegistrationLeases) error {
				if err := r.WithLeases(func(RegistrationLeases) error { return nil }); err != nil {
					return err
				}
				entered <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("registration operations were serialized")
		}
	}
	closed := make(chan error, 2)
	go func() { closed <- r.Close() }()
	waitRegistrationRetired(t, r)
	go func() { closed <- r.Close() }()
	select {
	case <-stopping:
		t.Fatal("unregister ran during a setup/operation")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	<-stopping
	select {
	case <-closed:
		t.Fatal("close returned before native join")
	default:
	}
	joinOnce.Do(func() { close(joined) })
	for i := 0; i < 2; i++ {
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	}
	if stops.Load() != 1 {
		t.Fatal("concurrent close repeated stop:", stops.Load())
	}
}

func TestRegistrationPanicsPreservedAndCleanupRetried(t *testing.T) {
	session := New(Options{})
	defer session.Close()
	calls := 0
	r, err := NewRegistration(RegistrationResources{Symbols: []*Symbol{{owner: session, address: 42}}}, func(leases RegistrationLeases) error {
		if address, err := leases.Symbols[0].Address(); address != 42 || err != nil {
			t.Error("stop panic lost retention:", address, err)
		}
		calls++
		if calls == 1 {
			panic("stop panic")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, test := range []struct {
		want string
		run  func()
	}{
		{"use panic", func() { _ = r.WithLeases(func(RegistrationLeases) error { panic("use panic") }) }},
		{"stop panic", func() { _ = r.Close() }},
	} {
		func() {
			defer func() {
				if got := recover(); got != test.want {
					t.Error("panic:", got)
				}
			}()
			test.run()
		}()
	}
	session.mu.Lock()
	retained := session.calls
	session.mu.Unlock()
	if retained != 1 {
		t.Fatal("panic released code lease:", retained)
	}
	if err := r.Close(); err != nil || calls != 2 {
		t.Fatal("panic retry:", calls, err)
	}
}

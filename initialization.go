package dylib

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ramblelabs/dylib-go/internal/native"
)

// ErrInitializing rejects ordinary lookup, linking and mutation while native
// initialization is in progress. Retry after the initializing operation returns.
var ErrInitializing = errors.New("dylib: native initialization is in progress")

// ErrNotInitializing means a scoped initialization call has no active phase.
var ErrNotInitializing = errors.New("dylib: no native initialization is in progress")

// WithInitializingAddress calls use with a temporary native address while an
// initializer is running. Raw-image symbols are relocated and executable, but
// their native state may still be partially initialized. During OS loading,
// only already retained libraries, explicit definitions and process symbols
// are searched; the newly opening handle is not available yet.
//
// This does not publish a Symbol or lease. All native address use must finish
// before use returns, and use must not close this session or retain the address.
// Concurrent/nested uses retain code and libraries through return, including
// when Close begins or initialization fails. New uses after retirement fail.
// Calling conventions, initialized native state and data synchronization are
// the adapter's responsibility. No automatic linking occurs.
func (s *Session) WithInitializingAddress(name string, use func(uintptr) error) error {
	if s == nil {
		return ErrClosed
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.initErr != nil {
		s.mu.Unlock()
		return s.initErr
	}
	if !s.initializing {
		s.mu.Unlock()
		return ErrNotInitializing
	}
	if name == "" || strings.IndexByte(name, 0) >= 0 || use == nil {
		s.mu.Unlock()
		return fmt.Errorf("dylib: invalid initialization address use")
	}
	var address uintptr
	var err error
	if s.stagedImage != nil {
		address, err = s.stagedImage.lookup(name)
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	// OS symbol lookup can wait for the loader lock held by another thread's
	// constructor. Do not hold the session mutex while waiting for that lock.
	staged := s.stagedImage != nil
	defined, libraries, processSymbols := s.defined[name], s.libs, s.opts.ProcessSymbols
	s.calls++
	s.mu.Unlock()
	defer s.releaseCall()
	if !staged {
		address = lookupExternal(name, defined, libraries, processSymbols)
		if address == 0 {
			return fmt.Errorf("unresolved symbol %s", name)
		}
	}
	return use(address)
}

// These helpers enter and return with mu held. Native calls run without it;
// a lifetime guard makes concurrent Close wait even before image publication.
func (s *Session) duringInitialization(staged *image, run func() error) error {
	s.initializing, s.stagedImage = true, staged
	s.calls++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.initializing, s.stagedImage = false, nil
		s.calls--
		if s.calls == 0 && s.cond != nil {
			s.cond.Broadcast()
		}
	}()
	return run()
}

func (s *Session) openLibrary(path string) (handle uintptr, err error) {
	err = s.duringInitialization(nil, func() error {
		handle, err = native.Open(path)
		return err
	})
	return handle, err
}

func (s *Session) initializeImage(im *image) (err error) {
	returned := false
	defer func() {
		if !returned {
			s.failedImage = im
			s.initErr = fmt.Errorf("%w: initializer did not return", ErrInitialization)
		}
	}()
	err = s.duringInitialization(im, im.initialize)
	returned = true
	if err != nil {
		// Exit registrations and in-flight scoped uses can still need this code.
		s.failedImage, s.initErr = im, err
		return err
	}
	s.image = im
	if s.closed {
		return ErrClosed // Close waits for the guard and owns native cleanup.
	}
	return nil
}

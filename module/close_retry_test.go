package module

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"

	dylib "github.com/ramblelabs/dylib-go"
)

type closeFunc func() error

func (f closeFunc) Close() error { return f() }

func retryGraph(closers ...io.Closer) *Graph {
	return &Graph{sessions: map[string]*dylib.Session{"root": dylib.New(dylib.Options{})}, order: closers}
}

func TestGraphCloseRetryPreservesProviders(t *testing.T) {
	failure := errors.New("native cleanup unavailable")
	var events []string
	fail := true
	g := retryGraph(
		closeFunc(func() error { events = append(events, "provider"); return nil }),
		closeFunc(func() error {
			events = append(events, "consumer")
			if fail {
				return failure
			}
			return nil
		}),
		closeFunc(func() error { events = append(events, "leaf"); return nil }),
	)
	if err := g.Close(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"leaf", "consumer"}) {
		t.Fatal("closed a provider still retained by its consumer", events)
	}
	if _, err := g.Session("root"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	fail = false
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"leaf", "consumer", "consumer", "provider"}) {
		t.Fatal("retry repeated a completed session", events)
	}
}

func TestGraphConcurrentFailedCloseAndRetry(t *testing.T) {
	failure := errors.New("native cleanup unavailable")
	providers, consumers := 0, 0
	g := retryGraph(
		closeFunc(func() error { providers++; return nil }),
		closeFunc(func() error { consumers++; return failure }),
	)
	const count = 16
	closed := make(chan error, count)
	var start sync.WaitGroup
	start.Add(1)
	for i := 0; i < count; i++ {
		go func() { start.Wait(); closed <- g.Close() }()
	}
	start.Done()
	for i := 0; i < count; i++ {
		if err := <-closed; !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	if providers != 0 || consumers == 0 {
		t.Fatal("failed consumer released its provider", providers, consumers)
	}
	failure = nil
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if providers != 1 {
		t.Fatal("provider was not closed once", providers)
	}
}

func TestLoadFailureReturnsCleanupOwner(t *testing.T) {
	primary, cleanup := errors.New("load failed"), errors.New("native cleanup failed")
	providers := 0
	g := retryGraph(closeFunc(func() error { providers++; return nil }), closeFunc(func() error { return cleanup }))
	owner, err := g.loadFailure("consumer", primary)
	if owner != g || !errors.Is(err, primary) || !errors.Is(err, cleanup) {
		t.Fatal("lost failed rollback owner", owner, err)
	}
	if _, err := owner.Session("root"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	if providers != 0 {
		t.Fatal("rollback closed a retained provider")
	}
	cleanup = nil
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if providers != 1 {
		t.Fatal("rollback recovery omitted provider cleanup")
	}
}

func TestManagerCloseRetriesRetiredGraph(t *testing.T) {
	failure := errors.New("native cleanup unavailable")
	providers, consumers := 0, 0
	g := retryGraph(closeFunc(func() error { providers++; return nil }), closeFunc(func() error { consumers++; return failure }))
	m, err := NewManager(g)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if providers != 0 {
		t.Fatal("manager closed a retained provider")
	}
	if err := m.WithGraph(func(*Graph) error { t.Fatal("retired manager admitted a scope"); return nil }); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := m.Replace(func() (*Graph, error) { t.Fatal("retired manager admitted replacement"); return nil, nil }, nil); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(err)
	}
	failure = nil
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if providers != 1 || consumers != 2 || len(m.pending) != 0 {
		t.Fatal("manager lost or repeated cleanup", providers, consumers)
	}
}

func TestManagerRetainsFailedGenerationCleanup(t *testing.T) {
	for _, stage := range []string{"old", "candidate", "migration", "panic"} {
		t.Run(stage, func(t *testing.T) {
			cleanup, primary := errors.New("native cleanup unavailable"), errors.New("replacement failed")
			oldCalls, nextCalls := 0, 0
			old := retryGraph(closeFunc(func() error {
				oldCalls++
				if stage == "old" {
					return cleanup
				}
				return nil
			}))
			next := retryGraph(closeFunc(func() error {
				nextCalls++
				if stage != "old" {
					return cleanup
				}
				return nil
			}))
			m, err := NewManager(old)
			if err != nil {
				t.Fatal(err)
			}
			build := func() (*Graph, error) {
				if stage == "candidate" {
					return next, primary
				}
				return next, nil
			}
			migrate := func(*Graph, *Graph) error {
				if stage == "panic" {
					panic("migration failed")
				}
				if stage == "migration" {
					return primary
				}
				return nil
			}
			var published bool
			var caught any
			func() {
				defer func() { caught = recover() }()
				published, err = m.Replace(build, migrate)
			}()
			if stage == "panic" {
				if caught != "migration failed" {
					t.Fatal("migration panic was lost", caught)
				}
			} else if !errors.Is(err, cleanup) || (stage != "old" && !errors.Is(err, primary)) {
				t.Fatal("replacement lost cleanup error", err)
			}
			if published != (stage == "old") {
				t.Fatal("incorrect publication", published)
			}
			selected := old
			if published {
				selected = next
			}
			if err := m.WithGraph(func(g *Graph) error {
				if g != selected {
					t.Fatal("cleanup failure changed graph selection")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			cleanup = nil
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			wantOld, wantNext := 1, 2
			if stage == "old" {
				wantOld, wantNext = 2, 1
			}
			if oldCalls != wantOld || nextCalls != wantNext || len(m.pending) != 0 {
				t.Fatal("failed generation ownership lost", oldCalls, nextCalls)
			}
		})
	}
}

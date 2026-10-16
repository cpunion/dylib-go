//go:build cgo && (linux || darwin || windows)

package module_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dylib "github.com/cpunion/dylib-go"
	call "github.com/cpunion/dylib-go/examples/call"
	example "github.com/cpunion/dylib-go/examples/modules"
	"github.com/cpunion/dylib-go/module"
)

func graphCall(graph *module.Graph, name string, a, b int32) (value int32, err error) {
	session, err := graph.Session("app")
	if err != nil {
		return 0, err
	}
	symbol, err := session.Resolve(name)
	if err != nil {
		return 0, err
	}
	err = symbol.WithAddress(func(address uintptr) error { value, err = call.Int32(address, a, b); return err })
	return
}

func managerCall(manager *module.Manager) (value int32, err error) {
	err = manager.WithGraph(func(graph *module.Graph) error { value, err = graphCall(graph, "graph_call", 20, 20); return err })
	return
}

func replacementFixture(t *testing.T) (*module.Manager, *module.Graph, func() (*module.Graph, error)) {
	t.Helper()
	dir := t.TempDir()
	manifest := func(path, version string) module.Manifest {
		return module.Manifest{Format: module.FormatVersion, Roots: []string{"app"}, Modules: []module.Module{{
			Name: "app", Version: version, Files: []string{path}, Exports: []string{"graph_call", "graph_state", "graph_set"},
		}}}
	}
	source := func(initial, bias int) string {
		return fmt.Sprintf(`static int state=%d; API int graph_call(int a,int b){return state+a+b+%d;} API int graph_state(int a,int b){return state;} API int graph_set(int a,int b){state=a;return state;}`, initial, bias)
	}
	oldPath := produce(t, dir, "old", source(2, 0), "object", "-O2")
	nextPath := produce(t, dir, "next", source(0, 1000), "object", "-O2")
	old, err := module.Open(dir, manifest(oldPath, "1"), dylib.Options{})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := module.NewManager(old)
	if err != nil {
		old.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	return manager, old, func() (*module.Graph, error) { return module.Open(dir, manifest(nextPath, "2"), dylib.Options{}) }
}

func TestNativeModuleReplacementFailures(t *testing.T) {
	manager, old, build := replacementFixture(t)
	checkOld := func() {
		t.Helper()
		if got, err := managerCall(manager); err != nil || got != 42 {
			t.Fatal("old generation", got, err)
		}
	}
	checkOld()
	if err := manager.WithGraph(nil); err == nil {
		t.Fatal("nil user accepted")
	}
	if installed, err := manager.Replace(nil, nil); installed || err == nil {
		t.Fatal("nil builder accepted", installed, err)
	}
	if installed, err := manager.Replace(func() (*module.Graph, error) { return old, nil }, nil); installed || err == nil {
		t.Fatal("current graph returned as candidate", installed, err)
	}
	checkOld()
	failure := errors.New("replace failed")
	for _, stage := range []string{"builder", "migration", "nil", "closed", "panic"} {
		t.Run(stage, func(t *testing.T) {
			var candidate *module.Graph
			factory := func() (*module.Graph, error) {
				if stage == "nil" {
					return nil, nil
				}
				var err error
				candidate, err = build()
				if err != nil {
					return candidate, err
				}
				if stage == "builder" {
					return candidate, failure
				}
				if stage == "closed" {
					candidate.Close()
				}
				return candidate, nil
			}
			migrate := func(previous, next *module.Graph) error {
				// Reentry must not wait for an invocation/admission lock.
				if value, err := managerCall(manager); err != nil || value != 42 {
					return fmt.Errorf("migration reentry: %d, %v", value, err)
				}
				if err := example.MigrateState(previous, next); err != nil {
					return err
				}
				if stage == "panic" {
					panic(failure)
				}
				return failure
			}
			if stage == "panic" {
				func() {
					defer func() {
						if got := recover(); got != failure {
							t.Fatalf("migration panic: %v", got)
						}
					}()
					manager.Replace(factory, migrate)
				}()
			} else {
				installed, err := manager.Replace(factory, migrate)
				if installed || err == nil {
					t.Fatal("failed replacement published", installed, err)
				}
				if (stage == "builder" || stage == "migration") && !errors.Is(err, failure) {
					t.Fatal("original failure lost", err)
				}
			}
			if candidate != nil {
				if _, err := candidate.Session("app"); !errors.Is(err, dylib.ErrClosed) {
					t.Fatal("failed candidate not closed", err)
				}
			}
			checkOld()
		})
	}
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Fatalf("builder panic: %v", got)
			}
		}()
		manager.Replace(func() (*module.Graph, error) { panic(failure) }, nil)
	}()
	checkOld()
	// Panics and errors in ordinary scopes also release the generation count.
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Fatalf("scope panic: %v", got)
			}
		}()
		manager.WithGraph(func(*module.Graph) error { panic(failure) })
	}()
	if err := manager.WithGraph(func(*module.Graph) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if installed, err := example.ReplaceState(manager, build); !installed || err != nil {
		t.Fatal("successful replacement", installed, err)
	}
	if got, err := managerCall(manager); err != nil || got != 1042 {
		t.Fatal("migrated state", got, err)
	}
	if _, err := old.Session("app"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("old graph not closed", err)
	}
}

func TestNativeModuleReplacementRetirement(t *testing.T) {
	manager, old, build := replacementFixture(t)
	var lease *dylib.SymbolLease
	if err := manager.WithGraph(func(graph *module.Graph) error {
		session, err := graph.Session("app")
		if err != nil {
			return err
		}
		symbol, err := session.Resolve("graph_call")
		if err != nil {
			return err
		}
		lease, err = symbol.Acquire()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lease.Close() })
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseScope := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseScope)
	scoped := make(chan error, 1)
	go func() {
		scoped <- manager.WithGraph(func(graph *module.Graph) error {
			close(started)
			<-release
			value, err := graphCall(graph, "graph_call", 20, 20)
			if err == nil && value != 42 {
				return fmt.Errorf("old scope used replacement: %d", value)
			}
			return err
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("old scope did not start")
	}
	type result struct {
		installed bool
		err       error
	}
	replaced, candidates := make(chan result, 1), make(chan *module.Graph, 1)
	go func() {
		installed, err := manager.Replace(func() (*module.Graph, error) {
			graph, err := build()
			if err == nil {
				candidates <- graph
			}
			return graph, err
		}, example.MigrateState)
		replaced <- result{installed, err}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		value, err := managerCall(manager)
		if err != nil {
			t.Fatal(err)
		}
		if value == 1042 {
			break
		}
		if value != 42 || time.Now().After(deadline) {
			t.Fatal("replacement did not publish", value)
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case result := <-replaced:
		t.Fatal("replacement ignored active old scope", result)
	default:
	}
	// Close must retire admission even while Replace waits to drain old users.
	closed := make(chan error, 2)
	go func() { closed <- manager.Close() }()
	go func() { closed <- manager.Close() }()
	deadline = time.Now().Add(5 * time.Second)
	for {
		if _, err := managerCall(manager); errors.Is(err, dylib.ErrClosed) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("close did not retire admission during replacement")
		}
		time.Sleep(time.Millisecond)
	}
	releaseScope()
	select {
	case err := <-scoped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old scope did not finish")
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		if _, err := old.Session("app"); errors.Is(err, dylib.ErrClosed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old graph did not retire")
		}
		time.Sleep(time.Millisecond)
	}
	if err := lease.WithAddress(func(address uintptr) error {
		value, err := call.Int32(address, 20, 20)
		if err == nil && value != 42 {
			return fmt.Errorf("old leased code: %d", value)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-replaced:
		t.Fatal("replacement ignored external lease", result)
	default:
	}
	lease.Close()
	select {
	case result := <-replaced:
		if !result.installed || result.err != nil {
			t.Fatal("replacement", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not finish")
	}
	for range 2 {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("manager close did not finish")
		}
	}
	next := <-candidates
	if _, err := next.Session("app"); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("current graph not closed", err)
	}
	if installed, err := manager.Replace(func() (*module.Graph, error) { t.Fatal("closed manager built a graph"); return nil, nil }, nil); installed || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal(installed, err)
	}
}

func TestNativeModuleReplacementCloseBeforePublication(t *testing.T) {
	for _, stage := range []string{"builder", "migration"} {
		t.Run(stage, func(t *testing.T) {
			manager, old, build := replacementFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			candidates := make(chan *module.Graph, 1)
			type result struct {
				installed bool
				err       error
			}
			replaced := make(chan result, 1)
			go func() {
				installed, err := manager.Replace(func() (*module.Graph, error) {
					graph, err := build()
					if err == nil {
						candidates <- graph
						if stage == "builder" {
							close(entered)
							<-release
						}
					}
					return graph, err
				}, func(previous, next *module.Graph) error {
					close(entered)
					<-release
					return example.MigrateState(previous, next)
				})
				replaced <- result{installed, err}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("replacement did not enter", stage)
			}
			closed := make(chan error, 1)
			go func() { closed <- manager.Close() }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := managerCall(manager); errors.Is(err, dylib.ErrClosed) {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				if time.Now().After(deadline) {
					t.Fatal("close waited for operation before retiring admission")
				}
				time.Sleep(time.Millisecond)
			}
			unblock()
			select {
			case result := <-replaced:
				if result.installed || !errors.Is(result.err, dylib.ErrClosed) {
					t.Fatal("closed manager published replacement", result)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("replacement did not stop")
			}
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("close did not finish")
			}
			next := <-candidates
			for _, graph := range []*module.Graph{old, next} {
				if _, err := graph.Session("app"); !errors.Is(err, dylib.ErrClosed) {
					t.Fatal("close leaked a graph", err)
				}
			}
		})
	}
}

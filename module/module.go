package module

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	dylib "github.com/ramblelabs/dylib-go"
)

// Graph owns its sessions and retained provider dependencies. Session exposes
// borrowed sessions for normal Resolve/Bind/adapters; close the Graph, not its
// individual sessions. Native registrations/objects remain caller-owned and
// must be stopped/destroyed before Close. Do not copy a Graph.
type Graph struct {
	mu       sync.Mutex
	sessions map[string]*dylib.Session
	order    []io.Closer // Providers precede consumers; completed tails are removed.
	retired  bool
	attempt  *closeAttempt
}

type closeAttempt struct {
	done chan struct{}
	err  error
}

// Load reads a JSON manifest and resolves inputs relative to its directory.
// Options apply to each session. Native loading has the same initialization
// effects as Session.Load/Link; closing on failure cannot undo native effects.
// A non-nil graph returned with an error owns failed cleanup; retry its Close.
func Load(path string, options dylib.Options) (*Graph, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	manifest, err := Read(file)
	if err != nil {
		return nil, err
	}
	return Open(filepath.Dir(abs), manifest, options)
}

// Open validates all declared contracts before loading the roots and their
// transitive providers. Each selected module gets one session. Imports use
// DefineSymbol, retaining code/data until the consumer finishes native cleanup.
// A failure closes newly created sessions in reverse dependency order. If native
// cleanup fails too, the returned graph is non-nil and retired: retry its Close.
// Callers must keep the manifest and options stable for the duration of Open.
func Open(base string, manifest Manifest, options dylib.Options) (*Graph, error) {
	order, err := manifest.order()
	if err != nil {
		return nil, err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	graph := &Graph{sessions: make(map[string]*dylib.Session)}
	for _, index := range order {
		mod := manifest.Modules[index]
		session := dylib.New(options)
		graph.sessions[mod.Name] = session
		graph.order = append(graph.order, session)
		for _, imp := range mod.Imports {
			symbol, err := graph.sessions[imp.Provider].Resolve(imp.Symbol)
			if err != nil {
				return graph.loadFailure(mod.Name, err)
			}
			if err := session.DefineSymbol(imp.name(), symbol); err != nil {
				return graph.loadFailure(mod.Name, err)
			}
		}
		for _, path := range mod.Files {
			if !filepath.IsAbs(path) {
				path = filepath.Join(base, path)
			}
			if err := session.Load(path); err != nil {
				return graph.loadFailure(mod.Name, err)
			}
		}
		// Resolve declared exports even for leaf roots so a stale contract fails
		// during Open, without requiring a later consumer to discover it.
		if err := session.Link(mod.Exports...); err != nil {
			return graph.loadFailure(mod.Name, err)
		}
		for _, name := range mod.Exports {
			if _, err := session.Resolve(name); err != nil {
				return graph.loadFailure(mod.Name, err)
			}
		}
	}
	return graph, nil
}

func (g *Graph) loadFailure(name string, cause error) (*Graph, error) {
	err := fmt.Errorf("module: %s: %w", name, cause)
	if cleanup := g.Close(); cleanup != nil {
		return g, errors.Join(err, cleanup)
	}
	return nil, err
}

// Session borrows one loaded session. Unselected modules are unavailable.
// Close retires Graph lookup before retiring its sessions; ordinary session
// operations still use the core session's normal acquisition/retirement rules.
func (g *Graph) Session(name string) (*dylib.Session, error) {
	if g == nil {
		return nil, dylib.ErrClosed
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.retired {
		return nil, dylib.ErrClosed
	}
	session, ok := g.sessions[name]
	if !ok {
		return nil, fmt.Errorf("module: no loaded module %q", name)
	}
	return session, nil
}

// Close retires Graph lookup and closes consumers before their providers. It
// waits for external symbol/function leases too; release them after native users
// stop. Call outside this graph's calls, callbacks and finalizers. Concurrent
// callers share one cleanup attempt. On failure, providers remain
// alive and a later Close retries from the failed consumer without replaying
// completed sessions. Graph lookup remains retired.
func (g *Graph) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	if attempt := g.attempt; attempt != nil {
		g.mu.Unlock()
		<-attempt.done
		return attempt.err
	}
	if g.retired && len(g.order) == 0 {
		g.mu.Unlock()
		return nil
	}
	attempt := &closeAttempt{done: make(chan struct{})}
	g.attempt, g.retired = attempt, true
	order := g.order
	g.sessions, g.order = nil, nil
	g.mu.Unlock()
	var err error
	for index := len(order) - 1; index >= 0; index-- {
		if err = order[index].Close(); err != nil {
			break // This consumer still owns leases on its providers.
		}
		order[index] = nil
		order = order[:index]
	}
	g.mu.Lock()
	if len(order) != 0 {
		g.order = order
	}
	attempt.err, g.attempt = err, nil
	close(attempt.done)
	g.mu.Unlock()
	return err
}

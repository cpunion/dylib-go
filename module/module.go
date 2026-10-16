package module

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	dylib "github.com/cpunion/dylib-go"
)

// Graph owns its sessions and retained provider dependencies. Session exposes
// borrowed sessions for normal Resolve/Bind/adapters; close the Graph, not its
// individual sessions. Native registrations/objects remain caller-owned and
// must be stopped/destroyed before Close. Do not copy a Graph.
type Graph struct {
	mu       sync.Mutex
	sessions map[string]*dylib.Session
	order    []*dylib.Session // Providers precede consumers.
	done     chan struct{}
	err      error
}

// Load reads a JSON manifest and resolves inputs relative to its directory.
// Options apply to each session. Native loading has the same initialization
// effects as Session.Load/Link; closing on failure cannot undo native effects.
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
// A failure closes every newly created session in reverse dependency order.
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
	fail := func(mod string, err error) (*Graph, error) {
		return nil, errors.Join(fmt.Errorf("module: %s: %w", mod, err), graph.Close())
	}
	for _, index := range order {
		mod := manifest.Modules[index]
		session := dylib.New(options)
		graph.sessions[mod.Name] = session
		graph.order = append(graph.order, session)
		for _, imp := range mod.Imports {
			symbol, err := graph.sessions[imp.Provider].Resolve(imp.Symbol)
			if err != nil {
				return fail(mod.Name, err)
			}
			if err := session.DefineSymbol(imp.name(), symbol); err != nil {
				return fail(mod.Name, err)
			}
		}
		for _, path := range mod.Files {
			if !filepath.IsAbs(path) {
				path = filepath.Join(base, path)
			}
			if err := session.Load(path); err != nil {
				return fail(mod.Name, err)
			}
		}
		// Resolve declared exports even for leaf roots so a stale contract fails
		// during Open, without requiring a later consumer to discover it.
		if err := session.Link(mod.Exports...); err != nil {
			return fail(mod.Name, err)
		}
		for _, name := range mod.Exports {
			if _, err := session.Resolve(name); err != nil {
				return fail(mod.Name, err)
			}
		}
	}
	return graph, nil
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
	if g.done != nil {
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
// stop. Call outside this graph's calls, callbacks and finalizers. Concurrent or
// repeated closes wait for the same cleanup and return its result.
func (g *Graph) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	if g.done != nil {
		done := g.done
		g.mu.Unlock()
		<-done
		return g.err
	}
	g.done = make(chan struct{})
	g.mu.Unlock()
	var err error
	for index := len(g.order) - 1; index >= 0; index-- {
		err = errors.Join(err, g.order[index].Close())
	}
	g.mu.Lock()
	g.err = err
	g.sessions, g.order = nil, nil
	close(g.done)
	g.mu.Unlock()
	return err
}

package module

import (
	"errors"
	"fmt"
	"sync"

	dylib "github.com/ramblelabs/dylib-go"
)

type generation struct {
	graph *Graph
	uses  int
}

// Manager owns a replaceable graph. WithGraph scopes retain one generation;
// replacement publishes a new graph before waiting for old scopes and external
// code leases to finish. It does not rebind native pointers or language objects.
// Do not close its graphs independently or copy a Manager.
type Manager struct {
	mu        sync.Mutex
	changed   *sync.Cond
	operation sync.Mutex // Serialize replacement and cleanup, not native calls.
	current   *generation
	retired   bool
	attempt   *closeAttempt
	pending   []*Graph // Owned generations/candidates whose native cleanup failed.
}

func (g *Graph) isOpen() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.retired && len(g.sessions) != 0
}

// NewManager takes ownership of an open graph on success. On error, ownership
// remains with the caller. The graph must not already belong to another owner.
func NewManager(graph *Graph) (*Manager, error) {
	if !graph.isOpen() {
		return nil, fmt.Errorf("module: manager requires an open graph")
	}
	m := &Manager{current: &generation{graph: graph}}
	m.changed = sync.NewCond(&m.mu)
	return m, nil
}

// WithGraph borrows a generation until use returns or panics. Native calls and
// nested uses run without the admission mutex. Borrowed graphs/sessions must not
// escape the scope; explicitly acquired symbol/function leases may outlive it.
// Close and Replace must run outside this manager's scopes and native callbacks.
func (m *Manager) WithGraph(use func(*Graph) error) error {
	if m == nil {
		return dylib.ErrClosed
	}
	if use == nil {
		return fmt.Errorf("module: nil graph user")
	}
	m.mu.Lock()
	if m.retired || m.current == nil {
		m.mu.Unlock()
		return dylib.ErrClosed
	}
	current := m.current
	current.uses++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		current.uses--
		m.changed.Broadcast()
		m.mu.Unlock()
	}()
	return use(current.graph)
}

// Replace builds and owns a fresh graph, optionally migrates state, then
// publishes it. Old scopes continue using their original graph; new scopes use
// the replacement. The method waits for old scopes and graph cleanup afterward,
// including external symbol/function leases. Release such leases before this
// call, or concurrently after their native users finish.
//
// The builder transfers ownership of every non-nil returned graph, including on
// error. It must not return an already owned graph. Migration borrows both graphs;
// ordinary old calls may still run, so the caller synchronizes mutable state and
// native registrations. Migration effects are not rolled back automatically.
//
// After build returns, pre-publication failure/panic closes the candidate and
// preserves the current graph selection. published is true once installed, including
// when old cleanup returns an error. Close during build/migration rejects
// publication. Neither build nor migrate may call this manager's Replace/Close;
// WithGraph reentry is allowed until Close retires admission.
func (m *Manager) Replace(build func() (*Graph, error), migrate func(old, next *Graph) error) (published bool, err error) {
	if m == nil {
		return false, dylib.ErrClosed
	}
	if build == nil {
		return false, fmt.Errorf("module: nil graph builder")
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	if m.retired || m.current == nil {
		m.mu.Unlock()
		return false, dylib.ErrClosed
	}
	old := m.current
	m.mu.Unlock()
	next, err := build()
	if next == old.graph {
		return false, errors.Join(err, fmt.Errorf("module: builder returned the current graph"))
	}
	defer func() {
		if !published && next != nil {
			m.pending = append(m.pending, next)
			err = errors.Join(err, m.cleanupPending())
		}
	}()
	if err != nil {
		return false, err
	}
	if !next.isOpen() {
		return false, fmt.Errorf("module: builder must return a fresh open graph")
	}
	m.mu.Lock()
	closing := m.retired
	m.mu.Unlock()
	if closing {
		return false, dylib.ErrClosed
	}
	if migrate != nil {
		if err := migrate(old.graph, next); err != nil {
			return false, err
		}
	}
	m.mu.Lock()
	if m.retired {
		m.mu.Unlock()
		return false, dylib.ErrClosed
	}
	m.current = &generation{graph: next}
	published = true
	for old.uses != 0 {
		m.changed.Wait()
	}
	m.mu.Unlock()
	m.pending = append(m.pending, old.graph)
	return true, m.cleanupPending()
}

// operation guards the queue. Graphs are independent owners; a failed graph
// retains its internal dependency order while other generations can complete.
func (m *Manager) cleanupPending() error {
	pending := m.pending[:0]
	var err error
	for _, graph := range m.pending {
		if e := graph.Close(); e != nil {
			err = errors.Join(err, e)
			pending = append(pending, graph)
		}
	}
	clear(m.pending[len(pending):])
	m.pending = pending
	return err
}

// Close immediately rejects new scopes and publications, then waits for any
// replacement, current scopes and graph leases before cleanup. It holds no
// admission mutex across native cleanup. Concurrent callers share one attempt.
// Failed current/retired/candidate cleanup stays owned for a later Close retry;
// admission remains retired. Call outside scopes, builders, migration and callbacks.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if attempt := m.attempt; attempt != nil {
		m.mu.Unlock()
		<-attempt.done
		return attempt.err
	}
	attempt := &closeAttempt{done: make(chan struct{})}
	m.attempt, m.retired = attempt, true
	m.mu.Unlock()
	m.operation.Lock()
	m.mu.Lock()
	current := m.current
	if current != nil {
		for current.uses != 0 {
			m.changed.Wait()
		}
	}
	m.current = nil
	m.mu.Unlock()
	if current != nil {
		m.pending = append(m.pending, current.graph)
	}
	err := m.cleanupPending()
	m.operation.Unlock()
	m.mu.Lock()
	attempt.err, m.attempt = err, nil
	close(attempt.done)
	m.mu.Unlock()
	return err
}

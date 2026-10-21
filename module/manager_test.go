package module_test

import (
	"errors"
	"testing"

	dylib "github.com/ramblelabs/dylib-go"
	"github.com/ramblelabs/dylib-go/module"
)

func TestManagerRejectsInvalidOwners(t *testing.T) {
	for _, graph := range []*module.Graph{nil, {}} {
		if owner, err := module.NewManager(graph); owner != nil || err == nil {
			t.Fatal("manager accepted an absent graph", owner, err)
		}
	}
	closed := new(module.Graph)
	closed.Close()
	if owner, err := module.NewManager(closed); owner != nil || err == nil {
		t.Fatal("manager accepted a closed graph", owner, err)
	}
	for _, manager := range []*module.Manager{nil, {}} {
		if err := manager.WithGraph(func(*module.Graph) error { t.Fatal("invalid scope executed"); return nil }); !errors.Is(err, dylib.ErrClosed) {
			t.Fatal("uninitialized manager admitted a scope", err)
		}
		if installed, err := manager.Replace(func() (*module.Graph, error) { t.Fatal("invalid builder executed"); return nil, nil }, nil); installed || !errors.Is(err, dylib.ErrClosed) {
			t.Fatal("uninitialized manager started replacement", installed, err)
		}
		if err := manager.Close(); err != nil {
			t.Fatal(err)
		}
		if err := manager.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

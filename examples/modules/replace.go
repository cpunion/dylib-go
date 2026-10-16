//go:build cgo && (linux || darwin || windows)

package moduleexample

import (
	"fmt"

	call "github.com/cpunion/dylib-go/examples/call"
	"github.com/cpunion/dylib-go/module"
)

// ReplaceState demonstrates explicit state migration for the native test API.
// Its graph_state/graph_set functions use known int32(int32,int32) signatures;
// applications select their own native state and adapters. No libffi is needed.
func ReplaceState(manager *module.Manager, build func() (*module.Graph, error)) (bool, error) {
	return manager.Replace(build, MigrateState)
}

// MigrateState copies a scalar through native accessors while both graphs live.
// This fixture's old state is immutable during copying. Mutable applications
// must provide their own synchronization and native registration lifecycle.
func MigrateState(old, next *module.Graph) error {
	value, err := stateCall(old, "graph_state", 0)
	if err != nil {
		return err
	}
	got, err := stateCall(next, "graph_set", value)
	if err == nil && got != value {
		return fmt.Errorf("migration got %d; want %d", got, value)
	}
	return err
}

func stateCall(graph *module.Graph, name string, argument int32) (value int32, err error) {
	session, err := graph.Session("app")
	if err != nil {
		return 0, err
	}
	symbol, err := session.Resolve(name)
	if err != nil {
		return 0, err
	}
	err = symbol.WithAddress(func(address uintptr) error { value, err = call.Int32(address, argument, 0); return err })
	return
}

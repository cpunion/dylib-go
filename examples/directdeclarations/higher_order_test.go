//go:build llgo && cgo && (linux || darwin || windows)

package main

import (
	"testing"

	dylib "github.com/ramblelabs/dylib-go"
)

func checkHigherOrderCalls(t *testing.T, session *dylib.Session, b *Bindings) {
	t.Helper()
	lease, err := session.Resolve("maker_factory")
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got int32, err error, want int32) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("%s: got %d, %v; want %d", name, got, err, want)
		}
	}
	err = lease.WithAddress(func(uintptr) error {
		maker, err := b.Maker_factory()
		if err != nil || maker == nil {
			t.Fatal("maker", err)
		}
		check("returned maker", maker()(20, 22), nil, 42)
		got, err := b.Apply_maker(maker, 20, 22)
		check("forwarded maker", got, err, 42)
		meta, err := b.Meta_factory()
		if err != nil || meta == nil {
			t.Fatal("meta maker", err)
		}
		check("returned meta maker", meta()()(20, 22), nil, 42)
		got, err = b.Apply_meta(meta, 20, 22)
		check("forwarded meta maker", got, err, 42)
		entry := maker()
		consumer, err := b.Consumer_factory()
		if err != nil || consumer == nil {
			t.Fatal("consumer", err)
		}
		check("returned consumer", consumer(entry, 20, 22), nil, 42)
		got, err = b.Apply_consumer(consumer, entry, 20, 22)
		check("forwarded consumer", got, err, 42)
		goConsumer := BindingsCallback9(func(fn BindingsCallback0, a, b int32) int32 { return fn(a, b) + 1 })
		got, err = b.Apply_consumer(goConsumer, entry, 20, 21)
		check("Go consumer calling C entry", got, err, 42)
		slot, err := b.Adder_slot()
		if err != nil || slot == nil {
			t.Fatal("native pointer slot", err)
		}
		check("native slot entry", (*slot)(20, 22), nil, 42)
		subtract, err := b.Subtract_factory()
		if err != nil || subtract == nil {
			t.Fatal("subtract", err)
		}
		if err := b.Replace_adder(slot, subtract); err != nil {
			t.Fatal(err)
		}
		check("replaced native slot", (*slot)(64, 22), nil, 42)
		if err := b.Replace_adder(slot, entry); err != nil {
			t.Fatal(err)
		}
		goSlot := entry
		if err := b.Replace_adder(&goSlot, subtract); err != nil {
			t.Fatal(err)
		}
		check("replaced Go slot", goSlot(64, 22), nil, 42)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

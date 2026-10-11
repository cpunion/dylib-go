//go:build cgo && (linux || darwin || windows)

package main

import (
	"testing"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
)

func checkHigherOrderCalls(t *testing.T, session *dylib.Session, b *Bindings) {
	t.Helper()
	lease, err := session.Resolve("maker_factory")
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got int32, err error) {
		t.Helper()
		if err != nil || got != 42 {
			t.Fatalf("%s: got %d, %v; want 42", name, got, err)
		}
	}
	err = lease.WithAddress(func(uintptr) error {
		maker, err := b.Maker_factory()
		if err != nil || maker == nil {
			t.Fatal("maker", err)
		}
		got, err := b.Apply_maker(maker, 20, 22)
		check("forwarded maker", got, err)
		meta, err := b.Meta_factory()
		if err != nil || meta == nil {
			t.Fatal("meta maker", err)
		}
		got, err = b.Apply_meta(meta, 20, 22)
		check("forwarded meta maker", got, err)
		entry, err := b.Adder_factory()
		if err != nil || entry == nil {
			t.Fatal("adder", err)
		}
		consumer, err := b.Consumer_factory()
		if err != nil || consumer == nil {
			t.Fatal("consumer", err)
		}
		got, err = b.Apply_consumer(consumer, entry, 20, 22)
		check("forwarded consumer", got, err)
		slot, err := b.Adder_slot()
		if err != nil || slot == nil {
			t.Fatal("native pointer slot", err)
		}
		got, err = b.Apply_adder(*(*unsafe.Pointer)(slot), 20, 22)
		check("native slot entry", got, err)
		subtract, err := b.Subtract_factory()
		if err != nil || subtract == nil {
			t.Fatal("subtract", err)
		}
		if err := b.Replace_adder(slot, subtract); err != nil {
			t.Fatal(err)
		}
		got, err = b.Apply_adder(*(*unsafe.Pointer)(slot), 64, 22)
		check("replaced native slot", got, err)
		if err := b.Replace_adder(slot, entry); err != nil {
			t.Fatal(err)
		}
		goSlot := entry
		if err := b.Replace_adder(unsafe.Pointer(&goSlot), subtract); err != nil {
			t.Fatal(err)
		}
		got, err = b.Apply_adder(goSlot, 64, 22)
		check("replaced Go slot", got, err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

package ar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func thinFixture(names ...string) []byte {
	var table, headers []byte
	for _, name := range names {
		headers = append(headers, []byte(fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", fmt.Sprintf("/%d", len(table)), "0", "0", "0", "644", 1))...)
		table = append(table, []byte(name+"/\n")...)
	}
	header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", "//", "0", "0", "0", "644", len(table))
	data := append([]byte("!<thin>\n"+header), table...)
	if len(table)&1 != 0 {
		data = append(data, '\n')
	}
	return append(data, headers...)
}

func TestReaderBudgetAndSnapshotReuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	data := []byte("snapshot")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reads := 0
	read := func(path string, limit int64) ([]byte, error) {
		reads++
		if int64(len(data)) > limit {
			return nil, fmt.Errorf("expected file of at most %d bytes", limit)
		}
		return os.ReadFile(path)
	}
	r := thinReader{remaining: 4, sources: make(map[string]*thinSource), readFile: read, maxSize: 1024}
	if _, err := r.read(path); err == nil || !strings.Contains(err.Error(), "at most 4 bytes") || len(r.sources) != 0 || r.remaining != 4 {
		t.Fatalf("failed read changed budget/cache: %v", err)
	}
	r.remaining = 1024
	first, err := r.read(path)
	if err != nil || !bytes.Equal(first.data, data) || r.remaining != 1024-int64(len(data)) {
		t.Fatalf("first snapshot: %v", err)
	}
	remaining := r.remaining
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if again, err := r.read(path); err != nil || again != first || r.remaining != remaining || reads != 2 {
		t.Fatalf("external snapshot charged/read more than once: %v", err)
	}
}

func TestSnapshotValidationCancellationAndBudgets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thin.a")
	payload := bytes.Repeat([]byte("x"), 100)
	reads := 0
	read := func(_ string, limit int64) ([]byte, error) {
		reads++
		if int64(len(payload)) > limit {
			return nil, fmt.Errorf("expected file of at most %d bytes", limit)
		}
		return payload, nil
	}
	bad := append(thinFixture("missing"), 'x')
	if _, err := Snapshot(context.Background(), path, bad, 1024, 0, read); err == nil || !strings.Contains(err.Error(), "bad archive header") || reads != 0 {
		t.Fatalf("container was not validated before I/O: %v", err)
	}
	data := thinFixture("same", "same", "same", "same", "same")
	if _, err := Snapshot(context.Background(), path, data, len(data)+len(payload), 0, read); err == nil || !strings.Contains(err.Error(), "decoded object bytes") || reads != 1 {
		t.Fatalf("decoded budget/cache: %v, %d reads", err, reads)
	}
	reads = 0
	data = thinFixture("one", "two")
	if _, err := Snapshot(context.Background(), path, data, len(data)+len(payload), 0, read); err == nil || !strings.Contains(err.Error(), "at most 0 bytes") || reads != 2 {
		t.Fatalf("unique read budget: %v, %d reads", err, reads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reads = 0
	cancelRead := func(path string, limit int64) ([]byte, error) {
		cancel()
		return read(path, limit)
	}
	if _, err := Snapshot(ctx, path, data, 1024, 0, cancelRead); !errors.Is(err, context.Canceled) || reads != 1 {
		t.Fatalf("cancellation did not stop external reads: %v", err)
	}
	if _, err := Snapshot(nil, path, data, 1024, 0, read); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := Snapshot(context.Background(), path, data, 1024, 0, nil); err == nil {
		t.Fatal("nil file reader accepted")
	}
}

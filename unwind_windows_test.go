package dylib

import (
	"errors"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func TestNativeWindowsRuntimeFunctionTables(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("Windows runtime function tables require amd64/arm64")
	}
	needNative(t)
	dir := t.TempDir()
	host := filepath.Join(dir, "unwind_host.dll")
	command(t, compiler(), "-shared", "-O0", "-funwind-tables", "testdata/unwind_host_windows.c", "-o", host)
	obj := compile(t, "testdata/unwind_frames.c", filepath.Join(dir, "frames.o"), "-funwind-tables")
	lookup := syscall.NewLazyDLL("kernel32.dll").NewProc("RtlLookupFunctionEntry")
	defaultSession := New(Options{})
	defer defaultSession.Close()
	load(t, defaultSession, host, obj)
	call(t, defaultSession, "unwind_root", 20, 22, -1) // No raw table without the option.
	if err := defaultSession.Close(); err != nil {
		t.Fatal(err)
	}
	for _, archive := range []bool{false, true} {
		input := obj
		if archive {
			input = filepath.Join(dir, "frames.a")
			command(t, "ar", "rcs", input, obj)
		}
		s := New(Options{RegisterUnwind: true})
		defer s.Close()
		load(t, s, host, input)
		call(t, s, "unwind_root", 20, 22, 42) // OS unwinds through both raw C frames.
		address, err := s.Lookup("unwind_root")
		if err != nil {
			t.Fatal(err)
		}
		var base uint64
		entry, _, _ := lookup.Call(address, uintptr(unsafe.Pointer(&base)), 0)
		if entry == 0 || base != uint64(s.image.base) || s.image.unwind == nil || !s.image.unwind.registered {
			t.Fatalf("OS table lookup: entry=%#x base=%#x", entry, base)
		}
		owned := s.image.unwind
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		entry, _, _ = lookup.Call(address, uintptr(unsafe.Pointer(&base)), 0)
		if entry != 0 || owned.registered || owned.mem != nil {
			t.Fatalf("OS table retained after Close: entry=%#x", entry)
		}
	}
	// Unsupported metadata rejects the unpublished image, and validation
	// remains retryable after correcting the staged table.
	validation := New(Options{RegisterUnwind: true})
	defer validation.Close()
	load(t, validation, host, obj)
	var metadata *section
	for _, s := range validation.files[0].obj.sections {
		if s != nil && s.name == ".xdata" {
			metadata = s
			break
		}
	}
	if metadata == nil {
		t.Fatal("producer .xdata missing")
	}
	byteIndex, flag := 0, byte(8)
	if runtime.GOARCH == "arm64" {
		byteIndex, flag = 2, 16
	}
	original := metadata.data[byteIndex]
	metadata.data[byteIndex] |= flag
	if err := validation.Link("unwind_root"); err == nil || validation.image != nil || validation.initErr != nil {
		t.Fatalf("unsupported metadata published or became permanent: %v", err)
	}
	metadata.data[byteIndex] = original
	call(t, validation, "unwind_root", 20, 22, 42)
	if err := validation.Close(); err != nil {
		t.Fatal(err)
	}
	// Registration remains alive through failed integer initialization cleanup.
	observer, _, read := lifecycleObserver(t, dir)
	failed := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(dir, "failed.o"), "-DFAIL_CODE=7", "-funwind-tables")
	s := New(Options{RegisterUnwind: true})
	defer s.Close()
	load(t, s, observer, failed)
	if err := s.Link("cinit_root"); !errors.Is(err, ErrInitialization) || s.failedImage == nil || s.failedImage.unwind == nil || !s.failedImage.unwind.registered {
		t.Fatalf("failed initializer unwind ownership: %v", err)
	}
	owned := s.failedImage.unwind
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if owned.mem != nil || owned.registered {
		t.Fatal("failed image table retained after cleanup")
	}
	expectEvents(t, read, 1, 2, 7, 8)
}

func TestNativeWindowsChainedRuntimeFunctionTables(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("x64 chained unwind requires Windows AMD64")
	}
	needNative(t)
	dir := t.TempDir()
	host := filepath.Join(dir, "chain_host.dll")
	command(t, compiler(), "-shared", "-O0", "-funwind-tables", "testdata/unwind_host_windows.c", "-o", host)
	obj := compile(t, "testdata/unwind_chained_amd64.S", filepath.Join(dir, "chain.o"))
	lookup := syscall.NewLazyDLL("kernel32.dll").NewProc("RtlLookupFunctionEntry")
	for _, mode := range []string{"default", "object", "archive"} {
		t.Run(mode, func(t *testing.T) {
			input := obj
			if mode == "archive" {
				input = filepath.Join(dir, "chain.a")
				command(t, "ar", "rcs", input, obj)
			}
			s := New(Options{RegisterUnwind: mode != "default"})
			defer s.Close()
			load(t, s, host, input)
			if mode == "default" {
				call(t, s, "unwind_chain_root", 20, 22, -1)
				return
			}
			// The observer must unwind both secondary records and the primary
			// allocation to reach the second raw frame and return 42.
			call(t, s, "unwind_chain_root", 20, 22, 42)
			owned := s.image.unwind
			if owned == nil || !owned.registered || len(owned.mem) != 48 {
				t.Fatal("four native runtime function ranges were not registered")
			}
			var addresses []uintptr
			for p := 0; p < len(owned.mem); p += 12 {
				begin, end := le.Uint32(owned.mem[p:]), le.Uint32(owned.mem[p+4:])
				address := s.image.base + uintptr(begin+(end-begin)/2)
				var base uint64
				entry, _, _ := lookup.Call(address, uintptr(unsafe.Pointer(&base)), 0)
				if entry == 0 || base != uint64(s.image.base) {
					t.Fatalf("OS lookup for range %d: entry=%#x base=%#x", p/12, entry, base)
				}
				addresses = append(addresses, address)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			for _, address := range addresses {
				var base uint64
				if entry, _, _ := lookup.Call(address, uintptr(unsafe.Pointer(&base)), 0); entry != 0 {
					t.Fatalf("chained function table retained after close: %#x", entry)
				}
			}
			if owned.mem != nil || owned.registered {
				t.Fatal("native table storage retained")
			}
		})
	}
	t.Run("validation retry", func(t *testing.T) {
		s := New(Options{RegisterUnwind: true})
		defer s.Close()
		load(t, s, host, obj)
		var metadata *section
		index := -1
		for _, section := range s.files[0].obj.sections {
			if section == nil || section.name != ".xdata" {
				continue
			}
			for p := 0; p+8 <= len(section.data); p += 4 {
				if section.data[p] == 0x21 && section.data[p+2] == 2 {
					metadata, index = section, p
					break
				}
			}
		}
		if index == -1 {
			t.Fatal("producer chained metadata missing")
		}
		original := metadata.data[index]
		metadata.data[index] |= 8 // A chain cannot also declare a handler.
		if err := s.Link("unwind_chain_root"); err == nil || s.image != nil || s.initErr != nil {
			t.Fatalf("invalid chain published or became permanent: %v", err)
		}
		metadata.data[index] = original
		call(t, s, "unwind_chain_root", 20, 22, 42)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

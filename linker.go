package dylib

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/internal/native"
)

var ErrClosed = errors.New("dylib: session is closed")
var ErrLinked = errors.New("dylib: session is already linked; create a new session to load more code")

// ErrInitialization marks a nonzero native C initializer result. Close the
// failed session and use a new one; native side effects cannot be retried safely.
var ErrInitialization = errors.New("dylib: C initialization failed; close the session and create a new one")

// Options controls resolution. ProcessSymbols exposes exported host symbols
// (for example libc on POSIX). Explicitly loaded libraries are always searched.
type Options struct {
	ProcessSymbols bool
	// RegisterUnwind registers supported raw-image runtime function tables.
	// Supports Windows/macOS amd64/arm64 and Linux amd64/arm64/386 C frames.
	// Other targets and language handlers are rejected. Exceptions must remain
	// inside native code.
	RegisterUnwind bool
	// LibraryPaths searches these directories before the importing file's
	// directory and OS loader paths. Explicitly loaded matching DLLs win.
	LibraryPaths []string
	// KeepLibraries leaves OS library references alive after Close. Use this
	// for runtimes with background threads or registrations (e.g. Go c-shared).
	KeepLibraries bool
}

// Session owns one linked image and its shared-library handles. Do not copy it.
// A session is built in two phases: Load/Define, then Link/Lookup/Resolve/Bind.
// Close invalidates every returned address. Callers using raw addresses must
// ensure no native threads or callbacks still use them before calling Close.
type Session struct {
	mu          sync.Mutex
	cond        *sync.Cond
	calls       int
	closing     bool
	opts        Options
	files       []*file
	libs        []uintptr
	dllHandles  map[string]uintptr
	paths       map[string]bool
	defined     map[string]uintptr
	image       *image
	failedImage *image // Retain code for exit registrations until Close after init failure.
	initErr     error
	closed      bool
	plans       map[string]*abi.CallPlan
}

func New(opts Options) *Session {
	opts.LibraryPaths = append([]string(nil), opts.LibraryPaths...)
	return &Session{opts: opts, paths: map[string]bool{}, defined: map[string]uintptr{}}
}

func (s *Session) mutable() error {
	if s.closed {
		return ErrClosed
	}
	if s.initErr != nil {
		return s.initErr
	}
	if s.image != nil {
		return ErrLinked
	}
	if s.paths == nil {
		s.paths = make(map[string]bool)
	}
	if s.defined == nil {
		s.defined = make(map[string]uintptr)
	}
	return nil
}

// Load identifies files by content. Objects are staged, archives are extracted
// by demand at Link, and shared libraries are opened immediately by the OS.
func (s *Session) Load(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mutable(); err != nil {
		return err
	}
	if strings.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("NUL in path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if s.paths[abs] {
		return nil
	}
	b, err := readFile(abs)
	if err != nil {
		return err
	}
	f, err := parse(abs, b)
	if err != nil {
		return err
	}
	if f.info.Kind == "universal" {
		f, err = f.hostSlice(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}
	}
	if !s.opts.RegisterUnwind {
		discardUnwind(f.obj)
		for _, member := range f.members {
			discardUnwind(member.obj)
		}
	}
	if f.obj != nil {
		f.obj.directory = filepath.Dir(abs)
	}
	for _, member := range f.members {
		if member.obj.directory == "" {
			member.obj.directory = filepath.Dir(abs)
		}
	}
	switch f.info.Kind {
	case "archive":
		s.files = append(s.files, f)
	case "object":
		if err = checkTarget(f.obj); err != nil {
			return err
		}
		s.files = append(s.files, f)
	case "shared":
		if err = checkHost(f.info); err != nil {
			return err
		}
		h, e := native.Open(abs)
		if e != nil {
			return e
		}
		s.libs = append(s.libs, h)
		if runtime.GOOS == "windows" {
			if s.dllHandles == nil {
				s.dllHandles = make(map[string]uintptr)
			}
			s.dllHandles[strings.ToLower(filepath.Base(abs))] = h
		}
	default:
		return fmt.Errorf("%s: executables cannot be loaded as libraries", path)
	}
	s.paths[abs] = true
	return nil
}

// Define registers an unmangled C name (or exact C++ mangled name). address
// must be a native C-ABI function or native data, not a Go function value.
func (s *Session) Define(name string, address uintptr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.mutable(); e != nil {
		return e
	}
	if name == "" || strings.IndexByte(name, 0) >= 0 || address == 0 {
		return fmt.Errorf("invalid native symbol")
	}
	if runtimeHelper(strings.TrimPrefix(name, "__imp_")) {
		return fmt.Errorf("%s is reserved for session-owned object lifecycle", name)
	}
	if hostELFOwnedGOT(name) {
		return fmt.Errorf("%s is reserved for the image-owned GOT", name)
	}
	if _, ok := s.defined[name]; ok {
		return fmt.Errorf("duplicate symbol %s", name)
	}
	s.defined[name] = address
	return nil
}

func checkHost(i Info) error {
	if i.Format == "Mach-O" && i.Arch == "arm64" && i.CPUSubtype&0xffffff > 1 {
		return fmt.Errorf("%s: unsupported ARM64 CPU subtype %#x", i.Name, i.CPUSubtype)
	}
	if i.OS != "" && i.OS != runtime.GOOS {
		return fmt.Errorf("%s: OS %s is incompatible with host %s", i.Name, i.OS, runtime.GOOS)
	}
	format := map[string]string{"darwin": "Mach-O", "linux": "ELF", "windows": "COFF"}[runtime.GOOS]
	if i.Format == "PE" && runtime.GOOS == "windows" {
		format = "PE"
	}
	bits := 64
	if runtime.GOARCH == "386" {
		bits = 32
	}
	if i.Bits != bits || i.Arch != runtime.GOARCH || i.Format != format {
		return fmt.Errorf("%s: target %s/%s/%d is incompatible with host %s/%s", i.Name, i.Format, i.Arch, i.Bits, runtime.GOOS, runtime.GOARCH)
	}
	if i.Arch != "amd64" && i.Arch != "arm64" && i.Arch != "386" {
		return fmt.Errorf("unsupported execution architecture %s", i.Arch)
	}
	return nil
}
func checkTarget(o *object) error {
	if e := checkHost(o.info); e != nil {
		return e
	}
	if len(o.info.Unsupported) > 0 {
		return fmt.Errorf("%s: unsupported: %s", o.info.Name, strings.Join(o.info.Unsupported, ", "))
	}
	return checkELFGOTDefinitions(o)
}

func (s *Session) external(name string) uintptr {
	if p := s.defined[name]; p != 0 {
		return p
	}
	for _, h := range s.libs {
		if p := native.Lookup(h, name); p != 0 {
			return p
		}
	}
	if s.opts.ProcessSymbols {
		return native.Lookup(0, name)
	}
	return 0
}

type definition struct {
	o     *object
	index int
}

func (d definition) sym() symbol { return d.o.symbols[d.index] }

func definitions(objs []*object) (map[string]definition, error) {
	m := map[string]definition{}
	for _, o := range objs {
		for i, v := range o.symbols {
			if !v.global || v.name == "" || v.section == 0 || v.section == -3 {
				continue
			}
			d, ok := m[v.name]
			if !ok {
				m[v.name] = definition{o, i}
				continue
			}
			old := d.sym()
			if old.imported != nil || v.imported != nil {
				if old.imported != nil && v.imported == nil {
					m[v.name] = definition{o, i}
					continue
				}
				if old.imported == nil && v.imported != nil {
					continue
				}
				a, b := old.imported, v.imported
				if strings.EqualFold(a.entry.dll, b.entry.dll) && a.entry.name == b.entry.name && a.entry.ordinal == b.entry.ordinal && a.indirect == b.indirect {
					continue
				}
				return nil, fmt.Errorf("conflicting DLL imports for %s", v.name)
			}
			if old.section == -2 && v.section == -2 {
				continue
			} // Merged by layout.
			if !old.weak && old.section != -2 && !v.weak && v.section != -2 {
				return nil, fmt.Errorf("duplicate strong symbol %s (%s, %s)", v.name, d.o.info.Name, o.info.Name)
			}
			if old.weak && !v.weak || old.section == -2 && v.section != -2 && !v.weak {
				m[v.name] = definition{o, i}
			}
		}
	}
	return m, nil
}

func references(o *object) ([]symbol, error) {
	var out []symbol
	for _, v := range o.symbols {
		if v.forward != nil {
			index := v.forward.target
			if index < 0 || index >= len(o.symbols) {
				return nil, fmt.Errorf("%s: invalid forwarding target %d", o.info.Name, index)
			}
			out = append(out, o.symbols[index])
		}
	}
	for _, r := range o.relocs {
		if elfNoRelocation(o, r.typ) || elfGOTBaseRelocation(o, r.typ) {
			continue
		}
		if !r.local {
			if r.symbol < 0 || r.symbol >= len(o.symbols) {
				return nil, fmt.Errorf("%s: invalid relocation symbol %d", o.info.Name, r.symbol)
			}
			v := o.symbols[r.symbol]
			if v.global && v.section == 0 && elfOwnedGOT(o.info, v.name) {
				continue // The image owns this base; no archive provider is needed.
			}
			out = append(out, v)
		}
		if r.pair >= 0 {
			if r.pair >= len(o.symbols) {
				return nil, fmt.Errorf("invalid subtractor symbol")
			}
			out = append(out, o.symbols[r.pair])
		}
	}
	return out, nil
}

func (s *Session) selectObjects(roots []string) ([]*object, error) {
	var objs, archives []*object
	for _, f := range s.files {
		if f.obj != nil {
			objs = append(objs, f.obj)
		}
		for _, m := range f.members {
			archives = append(archives, m.obj)
		}
	}
	used := map[*object]bool{}
	for {
		selected, err := coalesceObjects(objs)
		if err != nil {
			return nil, err
		}
		defs, err := definitions(selected)
		if err != nil {
			return nil, err
		}
		aliases, err := weakAliases(selected)
		if err != nil {
			return nil, err
		}
		probe := &image{defs: defs, aliases: aliases, external: s.external}
		var wanted []string
		for _, name := range roots {
			if hostELFOwnedGOT(name) {
				continue
			}
			if _, weak := aliases[name]; !weak {
				wanted = append(wanted, name)
			}
		}
		referenced := append([]string{}, roots...)
		for _, o := range selected {
			refs, e := references(o)
			if e != nil {
				return nil, e
			}
			for _, r := range refs {
				if r.section == 0 && r.name != "" {
					referenced = append(referenced, r.name)
				}
				if r.section == 0 && !r.weak && r.name != "" {
					if _, weak := aliases[r.name]; !weak {
						wanted = append(wanted, r.name)
					}
				}
			}
		}
		wanted = append(wanted, probe.weakLibraryNames(referenced)...)
		changed := false
		// First exhaust ordinary dependencies and SEARCH_LIBRARY names. Only
		// then consider fallback dependencies, preserving COFF search policy.
		for phase := 0; phase < 2 && !changed; phase++ {
			if phase == 1 {
				wanted = nil
				for _, name := range referenced {
					alias, ok := aliases[name]
					if !ok {
						continue
					}
					d, err := probe.resolveDefinition(alias.o, alias.index)
					if err != nil {
						return nil, err
					}
					if v := d.sym(); v.section == 0 && !v.weak && probe.externalSymbol(d.o, v.name) == 0 {
						wanted = append(wanted, v.name)
					}
				}
			}
			for _, n := range wanted {
				if _, ok := defs[n]; ok {
					continue
				}
				if s.defined[n] != 0 {
					continue
				}
				for _, o := range archives {
					if used[o] {
						continue
					}
					found := false
					for _, v := range o.symbols {
						if v.global && v.name == n && (v.section != 0 && v.section != -3 || v.alias != nil) {
							found = true
							break
						}
					}
					if !found {
						continue
					}
					if e := checkTarget(o); e != nil {
						return nil, e
					}
					used[o] = true
					objs = append(objs, o)
					changed = true
					break
				}
				if changed {
					break // Rebuild the symbol index before selecting another member.
				}
			}
		}
		if !changed {
			for _, o := range selected {
				for i, v := range o.symbols {
					if v.forward != nil {
						if _, err := probe.resolveDefinition(o, i); err != nil {
							return nil, err
						}
					}
				}
			}
			return selected, nil
		}
	}
}

// Link extracts archive members needed by roots and object references, then
// resolves all relocations before granting execute permission. Before native
// initialization, failure permits adding dependencies and retrying. A nonzero
// C initializer result permanently fails the session; call Close to clean up.
// No executable image is published on failure.
// Supported object initializers run after every link validation succeeds.
// Native initialization side effects cannot be rolled back.
func (s *Session) Link(roots ...string) error { s.mu.Lock(); defer s.mu.Unlock(); return s.link(roots) }
func (s *Session) link(roots []string) error {
	if s.closed {
		return ErrClosed
	}
	if s.initErr != nil {
		return s.initErr
	}
	for _, name := range roots {
		if name == "" || strings.IndexByte(name, 0) >= 0 {
			return fmt.Errorf("invalid root symbol name")
		}
	}
	if s.image != nil {
		for _, name := range roots {
			if _, err := s.image.lookup(name); err != nil {
				return err
			}
		}
		return nil
	}
	if !native.Available() {
		return fmt.Errorf("native execution requires cgo or llgo")
	}
	objs, err := s.selectObjects(roots)
	if err != nil {
		return err
	}
	// Selection returns private snapshots, including COMDAT discard decisions.
	defs, err := definitions(objs)
	if err != nil {
		return err
	}
	for n := range s.defined {
		if d, ok := defs[n]; ok && d.sym().imported == nil {
			return fmt.Errorf("symbol %s is defined by both host and object", n)
		}
	}
	imports, err := s.prepareImports(objs)
	if err != nil {
		return err
	}
	im, err := newImage(objs, defs, s.external, imports)
	if err != nil {
		return err
	}
	for _, n := range roots {
		if _, err = im.lookup(n); err != nil {
			im.close()
			return err
		}
	}
	if s.opts.RegisterUnwind && len(objs) != 0 {
		if err := im.registerUnwind(); err != nil {
			im.close()
			return err
		}
	}
	if err := im.initialize(); err != nil {
		// Do not run cleanup under mu: exit callbacks may reenter Go. Keep
		// their code alive until Close retires the failed session and drains
		// registrations outside the lock. No symbol is published meanwhile.
		s.failedImage, s.initErr = im, err
		return err
	}
	s.image = im
	return nil
}

// Lookup links on first use. Names omit Mach-O's leading linker underscore;
// C++ mangled names are passed unchanged. No type signature is inferred.
func (s *Session) Lookup(name string) (uintptr, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookup(name)
}
func (s *Session) lookup(name string) (uintptr, error) {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return 0, fmt.Errorf("invalid symbol name")
	}
	if err := s.link([]string{name}); err != nil {
		return 0, err
	}
	return s.image.lookup(name)
}

// Close retires the session, rejects new calls, and waits for active address
// users before releasing resources. Call it outside this session's adapters,
// callbacks, and finalizers: waiting for one's own call would deadlock.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		for s.closing {
			s.waitCalls()
		}
		s.mu.Unlock()
		return nil
	}
	s.closed, s.closing = true, true
	for s.calls != 0 {
		s.waitCalls()
	}
	plans, image, libs := s.plans, s.image, s.libs
	if image == nil {
		image = s.failedImage
	}
	s.plans, s.image, s.failedImage, s.libs, s.dllHandles = nil, nil, nil, nil, nil
	s.mu.Unlock()
	// Native finalizers may call back into Go. The session is already retired,
	// so reentrant lookups return ErrClosed without blocking on cleanup.
	var err error
	for _, plan := range plans {
		plan.Close()
	}
	if image != nil {
		err = image.close()
	}
	if !s.opts.KeepLibraries {
		for i := len(libs) - 1; i >= 0; i-- {
			err = errors.Join(err, native.Close(libs[i]))
		}
	}
	s.mu.Lock()
	s.closing = false
	if s.cond != nil {
		s.cond.Broadcast()
	}
	s.mu.Unlock()
	return err
}

// waitCalls requires mu. Lazy creation preserves the usable zero value.
func (s *Session) waitCalls() {
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
	s.cond.Wait()
}

func (s *Session) releaseCall() {
	s.mu.Lock()
	s.calls--
	if s.calls == 0 && s.cond != nil {
		s.cond.Broadcast()
	}
	s.mu.Unlock()
}

type image struct {
	mem                                    []byte
	base                                   uintptr
	objects                                []*object
	defs                                   map[string]definition
	aliases                                map[string]definition
	imports                                map[*coffImport]uintptr
	common                                 map[string]commonAllocation
	external                               func(string) uintptr
	resolved                               map[string]uintptr
	got, stubs                             map[uintptr]uintptr
	gotStart, stubStart, gotNext, stubNext uint64
	page                                   uint64
	pointerSize                            uint64
	lifecycle                              *native.Lifecycle
	hooks                                  map[string]uintptr
	initializers, finalizers               []uintptr
	cInitializers                          []cInitializer
	unwind                                 *runtimeFunctions
	initialized                            bool
}

type commonAllocation struct{ offset, size uint64 }

func alignUp(n, a uint64) uint64 {
	if a < 1 {
		return n
	}
	return (n + a - 1) &^ (a - 1)
}
func newImage(objs []*object, defs map[string]definition, external func(string) uintptr, imports map[*coffImport]uintptr) (*image, error) {
	im := &image{objects: objs, defs: defs, external: external, imports: imports, common: map[string]commonAllocation{}, resolved: map[string]uintptr{}, got: map[uintptr]uintptr{}, stubs: map[uintptr]uintptr{}, page: uint64(os.Getpagesize())}
	var err error
	im.aliases, err = weakAliases(objs)
	if err != nil {
		return nil, err
	}
	im.pointerSize = uint64(unsafe.Sizeof(uintptr(0)))
	if len(objs) != 0 && (objs[0].info.Bits == 32 || objs[0].info.Bits == 64) {
		im.pointerSize = uint64(objs[0].info.Bits / 8)
	}
	var size, count uint64
	for _, o := range objs {
		count += uint64(len(o.relocs))
		for _, v := range o.symbols {
			if v.imported != nil && v.imported.indirect {
				count++
			}
		}
		for _, s := range o.sections {
			if s == nil {
				continue
			}
			if s.align > im.page {
				return nil, fmt.Errorf("%s: alignment exceeds host page size", s.name)
			}
			s.offset = size
			size += alignUp(s.size+s.tail, im.page)
			if size > maxImage {
				return nil, fmt.Errorf("image exceeds 64 MiB")
			}
		}
	}
	// Merge tentative/common definitions by maximum size and alignment.
	type commonSize struct{ size, align uint64 }
	commons := map[string]commonSize{}
	for _, o := range objs {
		for _, s := range o.symbols {
			if s.section != -2 {
				continue
			}
			d, ok := defs[s.name]
			if !ok || d.sym().section != -2 {
				continue
			}
			v := commons[s.name]
			if s.size > v.size {
				v.size = s.size
			}
			if s.align > v.align {
				v.align = s.align
			}
			commons[s.name] = v
		}
	}
	names := make([]string, 0, len(commons))
	for n := range commons {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v := commons[n]
		if v.align > im.page || v.align != 0 && v.align&(v.align-1) != 0 || v.size > maxImage {
			return nil, fmt.Errorf("invalid common symbol %s", n)
		}
		size = alignUp(size, v.align)
		im.common[n] = commonAllocation{offset: size, size: v.size}
		size += v.size
		if size > maxImage {
			return nil, fmt.Errorf("common data exceeds image limit")
		}
	}
	size = alignUp(size, im.page)
	im.gotStart = size
	im.gotNext = size
	size += alignUp((count+1)*im.pointerSize, im.page)
	im.stubStart = size
	im.stubNext = size
	size += alignUp((count+1)*16, im.page)
	// Three context-bound exit helpers need at most 64 bytes each.
	// This also leaves capacity for branch stubs from every relocation.
	size += im.page
	if size > maxImage {
		return nil, fmt.Errorf("image exceeds 64 MiB")
	}
	b, e := native.Alloc(int(size))
	if e != nil {
		return nil, e
	}
	im.mem = b
	im.base = uintptr(unsafe.Pointer(&b[0]))
	ok := false
	defer func() {
		if !ok {
			im.close()
		}
	}()
	if e := im.prepareRuntimeHelpers(); e != nil {
		return nil, e
	}
	// Populate every selected IAT slot and validate forwarding targets before
	// publication, including definitions with no object relocations.
	for _, o := range objs {
		for i, v := range o.symbols {
			if v.imported != nil || v.forward != nil {
				if _, err := im.symbol(o, i, false); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, o := range objs {
		for _, s := range o.sections {
			if s != nil {
				copy(b[s.offset:s.offset+s.size], s.data)
			}
		}
	}
	for _, o := range objs {
		for _, r := range o.relocs {
			if e := im.relocate(o, r); e != nil {
				return nil, fmt.Errorf("%s: relocation %d at section %d+%#x: %w", o.info.Name, r.typ, r.section, r.offset, e)
			}
		}
	}
	if e := im.rebaseMachODWARF(); e != nil {
		return nil, e
	}
	if e := im.prepareLifecycle(); e != nil {
		return nil, e
	}
	native.ClearCache(b)
	// Each section owns whole pages; never grant WRITE and EXEC together.
	for _, o := range objs {
		for _, s := range o.sections {
			if s == nil || s.size+s.tail == 0 {
				continue
			}
			if e := native.Protect(b[s.offset:s.offset+alignUp(s.size+s.tail, im.page)], s.exec, s.write); e != nil {
				return nil, e
			}
		}
	}
	if e := native.Protect(b[im.gotStart:im.stubStart], false, false); e != nil {
		return nil, e
	}
	if e := native.Protect(b[im.stubStart:], true, false); e != nil {
		return nil, e
	}
	ok = true
	return im, nil
}
func (im *image) close() error {
	if im.mem == nil {
		return nil
	}
	// Registrations from a failed C initializer still own pointers into the
	// image. Drain them before unmapping, without running static termination
	// arrays for constructors that never completed.
	im.lifecycle.Finalize()
	if im.initialized {
		im.initialized = false
		for _, address := range im.finalizers {
			native.CallVoid(address)
		}
		// A finalizer may itself register an exit function.
		im.lifecycle.Finalize()
	}
	if err := im.unwind.close(); err != nil {
		// Never free code/table storage while the OS may still reference it.
		return err
	}
	im.lifecycle.Close()
	e := native.Free(im.mem)
	im.mem = nil
	im.initializers, im.finalizers = nil, nil
	im.cInitializers = nil
	return e
}
func (im *image) lookup(n string) (uintptr, error) {
	if len(im.objects) != 0 && elfOwnedGOT(im.objects[0].info, n) {
		return im.gotBase(), nil
	}
	if d, ok := im.defs[n]; ok {
		return im.symbol(d.o, d.index, false)
	}
	if p := im.hooks[n]; p != 0 {
		return p, nil
	}
	if p := im.external(n); p != 0 {
		return p, nil
	}
	if d, ok := im.aliases[n]; ok {
		return im.symbol(d.o, d.index, false)
	}
	return 0, fmt.Errorf("unresolved symbol %s", n)
}
func (im *image) symbol(o *object, index int, local bool) (uintptr, error) {
	if local {
		if index <= 0 || index >= len(o.sections) || o.sections[index] == nil {
			return 0, fmt.Errorf("invalid local section %d", index)
		}
		return im.base + uintptr(o.sections[index].offset), nil
	}
	if index < 0 || index >= len(o.symbols) {
		return 0, fmt.Errorf("invalid symbol index %d", index)
	}
	d, err := im.resolveDefinition(o, index)
	if err != nil {
		return 0, err
	}
	o, s := d.o, d.sym()
	switch s.section {
	case -4:
		if s.imported == nil {
			return 0, fmt.Errorf("missing COFF import metadata")
		}
		entry := s.imported.entry
		p := im.hooks[entry.public]
		if p == 0 {
			p = im.imports[entry]
		}
		if p == 0 && runtimeHelper(entry.public) {
			if d, ok := im.defs[entry.public]; ok && d.sym().imported == nil {
				var err error
				p, err = im.symbol(d.o, d.index, false)
				if err != nil {
					return 0, err
				}
			}
		}
		if p == 0 {
			return 0, fmt.Errorf("unresolved DLL import %s", entry.public)
		}
		if s.imported.indirect {
			return im.gotSlot(p)
		}
		return p, nil
	case 0:
		if s.global && elfOwnedGOT(o.info, s.name) {
			return im.gotBase(), nil
		}
		if p, ok := im.resolved[s.name]; ok {
			return p, nil
		}
		n := s.name
		indirect := o.info.Format == "COFF" && strings.HasPrefix(n, "__imp_")
		if indirect {
			n = strings.TrimPrefix(n, "__imp_")
		}
		p := im.hooks[n]
		if p == 0 {
			p = im.external(n)
		}
		if p == 0 && !s.weak {
			return 0, fmt.Errorf("unresolved symbol %s", s.name)
		}
		found := p != 0
		if indirect {
			var e error
			p, e = im.gotSlot(p)
			if e != nil {
				return 0, e
			}
		}
		// A missing weak target cannot satisfy a later strong reference. In
		// COFF, even a nonzero import slot can hold that missing target zero.
		if found {
			im.resolved[s.name] = p
		}
		return p, nil
	case -1:
		return uintptr(s.value), nil
	case -2:
		allocation, ok := im.common[s.name]
		if !ok {
			return 0, fmt.Errorf("unallocated common %s", s.name)
		}
		return im.base + uintptr(allocation.offset), nil
	default:
		if s.section < 0 || s.section >= len(o.sections) || o.sections[s.section] == nil {
			return 0, fmt.Errorf("symbol %s refers to an unsupported section", s.name)
		}
		sec := o.sections[s.section]
		if s.value > sec.size {
			return 0, fmt.Errorf("symbol %s outside section", s.name)
		}
		return im.base + uintptr(sec.offset+s.value), nil
	}
}
func (im *image) gotSlot(target uintptr) (uintptr, error) {
	if p, ok := im.got[target]; ok {
		return p, nil
	}
	if im.gotNext+im.pointerSize > im.stubStart {
		return 0, fmt.Errorf("GOT capacity exceeded")
	}
	off := im.gotNext
	if im.pointerSize == 4 {
		if uint64(target) > 0xffffffff {
			return 0, fmt.Errorf("32-bit GOT address overflow")
		}
		binary.LittleEndian.PutUint32(im.mem[off:], uint32(target))
	} else {
		binary.LittleEndian.PutUint64(im.mem[off:], uint64(target))
	}
	im.gotNext += im.pointerSize
	p := im.base + uintptr(off)
	im.got[target] = p
	return p, nil
}
func (im *image) stub(target uintptr, arch string) (uintptr, error) {
	if p, ok := im.stubs[target]; ok {
		return p, nil
	}
	if im.stubNext+16 > uint64(len(im.mem)) {
		return 0, fmt.Errorf("stub capacity exceeded")
	}
	off := im.stubNext
	im.stubNext += 16
	b := im.mem[off : off+16]
	switch arch {
	case "arm64":
		binary.LittleEndian.PutUint32(b, 0x58000050)
		binary.LittleEndian.PutUint32(b[4:], 0xd61f0200)
		binary.LittleEndian.PutUint64(b[8:], uint64(target)) // ldr x16, +8; br x16
	case "amd64":
		copy(b, []byte{0xff, 0x25, 0, 0, 0, 0})
		binary.LittleEndian.PutUint64(b[6:], uint64(target)) // jmp [rip]
	default:
		return 0, fmt.Errorf("no branch stub for %s", arch)
	}
	p := im.base + uintptr(off)
	im.stubs[target] = p
	return p, nil
}

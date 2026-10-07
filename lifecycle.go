package dylib

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/internal/native"
)

type lifecycleKind uint8

const (
	lifecyclePreinit lifecycleKind = iota + 1
	lifecycleInit
	lifecycleFini
)

func configureLifecycle(s *section, bits int) error {
	width := uint64(bits / 8)
	if width != 4 && width != 8 || s.size%width != 0 || s.exec {
		return fmt.Errorf("%s: invalid lifecycle pointer array", s.name)
	}
	s.priority = 65535
	for _, prefix := range []string{".preinit_array.", ".init_array.", ".fini_array."} {
		if strings.HasPrefix(s.name, prefix) {
			value, err := strconv.ParseUint(strings.TrimPrefix(s.name, prefix), 10, 16)
			if err != nil {
				return fmt.Errorf("%s: invalid initialization priority", s.name)
			}
			s.priority = uint32(value)
		}
	}
	return nil
}

// Object dependency order is derived from retained relocations. Strongly
// connected components retain input order; providers precede their consumers.
// Numeric ELF priorities and COFF subsection names take precedence over this
// order, just as explicit init priorities override translation-unit order.
func lifecycleObjectOrder(objects []*object, defs map[string]definition) (map[*object]int, error) {
	indices := make(map[*object]int, len(objects))
	for i, o := range objects {
		indices[o] = i
	}
	deps := make([][]int, len(objects))
	for i, o := range objects {
		refs, err := references(o)
		if err != nil {
			return nil, err
		}
		seen := make(map[int]bool)
		for _, ref := range refs {
			if ref.global {
				if d, ok := defs[ref.name]; ok {
					j := indices[d.o]
					if j != i && !seen[j] {
						seen[j] = true
						deps[i] = append(deps[i], j)
					}
				}
			}
		}
		sort.Ints(deps[i])
	}
	var next int
	number, low := make([]int, len(objects)), make([]int, len(objects))
	active := make([]bool, len(objects))
	var stack []int
	var components [][]int
	var visit func(int)
	visit = func(i int) {
		next++
		number[i], low[i], active[i] = next, next, true
		stack = append(stack, i)
		for _, j := range deps[i] {
			if number[j] == 0 {
				visit(j)
				if low[j] < low[i] {
					low[i] = low[j]
				}
			} else if active[j] && number[j] < low[i] {
				low[i] = number[j]
			}
		}
		if low[i] == number[i] {
			var component []int
			for {
				j := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				active[j] = false
				component = append(component, j)
				if j == i {
					break
				}
			}
			sort.Ints(component)
			components = append(components, component)
		}
	}
	for i := range objects {
		if number[i] == 0 {
			visit(i)
		}
	}
	// Tarjan emits provider components first for consumer -> provider edges.
	order := make(map[*object]int, len(objects))
	for _, component := range components {
		for _, i := range component {
			order[objects[i]] = len(order)
		}
	}
	return order, nil
}

// Read and validate every relocated entry before any native initializer runs.
// Keep a private list: user code can later mutate writable array storage, but
// cannot replace the finalizers that were validated for this linked image.
func (im *image) prepareLifecycle() error {
	type array struct {
		object  *object
		section *section
	}
	order, err := lifecycleObjectOrder(im.objects, im.defs)
	if err != nil {
		return err
	}
	var arrays []array
	for _, o := range im.objects {
		for _, s := range o.sections {
			if s != nil && s.lifecycle != 0 {
				arrays = append(arrays, array{o, s})
			}
		}
	}
	sort.SliceStable(arrays, func(i, j int) bool {
		a, b := arrays[i], arrays[j]
		if a.section.lifecycle != b.section.lifecycle {
			return a.section.lifecycle < b.section.lifecycle
		}
		if a.section.priority != b.section.priority {
			return a.section.priority < b.section.priority
		}
		if a.object.info.Format == "COFF" && a.section.name != b.section.name {
			return a.section.name < b.section.name
		}
		return order[a.object] < order[b.object]
	})
	var fini []uintptr
	coff := false
	for _, a := range arrays {
		s := a.section
		if err := configureLifecycle(s, int(im.pointerSize*8)); err != nil {
			return err
		}
		coff = a.object.info.Format == "COFF"
		for pos := uint64(0); pos < s.size; pos += im.pointerSize {
			data := im.mem[s.offset+pos:]
			var address uint64
			if im.pointerSize == 4 {
				address = uint64(binary.LittleEndian.Uint32(data[:4]))
			} else {
				address = binary.LittleEndian.Uint64(data[:8])
			}
			if address == 0 { // COFF boundary sentinels and empty entries.
				continue
			}
			if a.object.info.Arch == "arm64" && address%4 != 0 || !im.executableAddress(uintptr(address)) {
				return fmt.Errorf("%s: %s+%#x: initializer/finalizer is outside executable image sections", a.object.info.Name, s.name, pos)
			}
			if s.lifecycle == lifecycleFini {
				fini = append(fini, uintptr(address))
			} else {
				im.initializers = append(im.initializers, uintptr(address))
			}
		}
	}
	if !coff { // ELF fini arrays and Mach-O term functions run backward.
		for i, j := 0, len(fini)-1; i < j; i, j = i+1, j-1 {
			fini[i], fini[j] = fini[j], fini[i]
		}
	}
	im.finalizers = fini
	return nil
}

func (im *image) executableAddress(address uintptr) bool {
	for _, o := range im.objects {
		for _, s := range o.sections {
			if s != nil && s.exec {
				start := im.base + uintptr(s.offset)
				if address >= start && address-start < uintptr(s.size) {
					return true
				}
			}
		}
	}
	return false
}

func (im *image) initialize() {
	for _, address := range im.initializers {
		native.CallVoid(address)
	}
	im.initialized = true
}

func runtimeHelper(name string) bool {
	switch name {
	case "atexit", "__cxa_atexit", "__cxa_finalize", "__dso_handle":
		return true
	}
	return false
}

// These private C-ABI tail thunks bind the native registration helper to one
// image. No Go pointers or thread-local ambient session state cross the ABI.
func lifecycleThunk(arch, os string, context, target uintptr, args int) ([]byte, error) {
	var code []byte
	switch arch {
	case "amd64":
		reg := byte(0xbe) // rsi: second SysV argument.
		prefix := byte(0x48)
		if args == 3 {
			reg = 0xb9 // rcx: fourth SysV argument.
		}
		if os == "windows" {
			reg = 0xba // rdx: second Windows argument.
			if args == 3 {
				prefix, reg = 0x49, 0xb9 // r9: fourth Windows argument.
			}
		}
		code = append(code, prefix, reg)
		code = binary.LittleEndian.AppendUint64(code, uint64(context))
		code = append(code, 0x48, 0xb8)
		code = binary.LittleEndian.AppendUint64(code, uint64(target))
		code = append(code, 0xff, 0xe0) // jmp rax; original return address/stack.
	case "arm64":
		reg := uint32(1)
		if args == 3 {
			reg = 3
		}
		code = binary.LittleEndian.AppendUint32(code, 0x58000080|reg) // ldr xN, +16
		code = binary.LittleEndian.AppendUint32(code, 0x580000b0)     // ldr x16, +20
		code = binary.LittleEndian.AppendUint32(code, 0xd61f0200)     // br x16
		code = binary.LittleEndian.AppendUint32(code, 0xd503201f)     // nop / alignment.
		code = binary.LittleEndian.AppendUint64(code, uint64(context))
		code = binary.LittleEndian.AppendUint64(code, uint64(target))
	case "386":
		if uint64(context) > 0xffffffff || uint64(target) > 0xffffffff {
			return nil, fmt.Errorf("32-bit lifecycle thunk address overflow")
		}
		// GNU i386 helpers require a 16-byte call stack even when the
		// incoming MSVC caller promises only 4-byte alignment. Preserve ebp
		// and address the original arguments through it before realigning.
		code = append(code, 0x55, 0x89, 0xe5, 0x83, 0xe4, 0xf0) // push ebp; mov ebp,esp; and esp,-16.
		if args == 1 {
			code = append(code, 0x83, 0xec, 8) // Padding below two outgoing args.
		}
		code = append(code, 0x68)
		code = binary.LittleEndian.AppendUint32(code, uint32(context))
		for i := args - 1; i >= 0; i-- {
			code = append(code, 0xff, 0x75, byte(8+i*4)) // push [ebp+argument].
		}
		code = append(code, 0xb8)
		code = binary.LittleEndian.AppendUint32(code, uint32(target))
		code = append(code, 0xff, 0xd0, 0x89, 0xec, 0x5d, 0xc3) // call eax; mov esp,ebp; pop ebp; ret.
	default:
		return nil, fmt.Errorf("no lifecycle thunk for %s", arch)
	}
	return code, nil
}

func (im *image) prepareRuntimeHelpers() error {
	needed := make(map[string]bool)
	for _, o := range im.objects {
		for _, s := range o.symbols {
			name := strings.TrimPrefix(s.name, "__imp_")
			if s.section == 0 && runtimeHelper(name) {
				if _, defined := im.defs[name]; !defined {
					needed[name] = true
				}
			}
		}
	}
	if len(needed) == 0 {
		return nil
	}
	var err error
	im.lifecycle, err = native.NewLifecycle()
	if err != nil {
		return err
	}
	im.hooks = make(map[string]uintptr)
	// Stable placement independent of symbol-table or map iteration order.
	for _, name := range []string{"__dso_handle", "atexit", "__cxa_atexit", "__cxa_finalize"} {
		if !needed[name] {
			continue
		}
		if name == "__dso_handle" {
			im.hooks[name] = im.lifecycle.Context()
			continue
		}
		args := 1
		if name == "__cxa_atexit" {
			args = 3
		}
		code, err := lifecycleThunk(im.objects[0].info.Arch, im.objects[0].info.OS, im.lifecycle.Context(), im.lifecycle.Helper(name), args)
		if err != nil {
			return err
		}
		if im.stubNext+uint64(len(code)) > uint64(len(im.mem)) {
			return fmt.Errorf("lifecycle thunk capacity exceeded")
		}
		im.hooks[name] = im.base + uintptr(im.stubNext)
		copy(im.mem[im.stubNext:], code)
		im.stubNext += alignUp(uint64(len(code)), 16)
	}
	return nil
}

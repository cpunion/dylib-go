package dylib

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/cpunion/dylib-go/internal/native"
)

// Use a retained libgcc handle for both operations. Looking up the two names
// independently in the process could pair unrelated unwind implementations.
func (im *image) registerELFUnwind() error {
	frames, err := im.elfFrameSections()
	if err != nil || len(frames) == 0 {
		return err
	}
	provider, err := native.Open("libgcc_s.so.1")
	if err != nil {
		return fmt.Errorf("ELF unwind provider: %w", err)
	}
	register := native.Lookup(provider, "__register_frame")
	deregister := native.Lookup(provider, "__deregister_frame")
	if register == 0 || deregister == 0 {
		native.Close(provider)
		return fmt.Errorf("libgcc_s is missing frame registration functions")
	}
	u := &runtimeFunctions{provider: provider, deregister: deregister}
	im.unwind = u
	for _, frame := range frames {
		native.CallPointer(register, frame)
		u.frames = append(u.frames, frame)
	}
	u.registered = true
	return nil
}

func (im *image) elfFrameSections() ([]uintptr, error) {
	var frames []uintptr
	for _, o := range im.objects {
		for _, s := range o.sections {
			if s == nil || !elfUnwindSection(s.name) {
				continue
			}
			if s.exec || s.write || s.tail != 4 || s.offset+s.size+4 > uint64(len(im.mem)) {
				return nil, fmt.Errorf("%s: invalid ELF frame storage", o.info.Name)
			}
			b := im.mem[s.offset : s.offset+s.size]
			count, err := im.validateELFFrames(b, s.offset)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", o.info.Name, s.name, err)
			}
			if binary.LittleEndian.Uint32(im.mem[s.offset+s.size:]) != 0 {
				return nil, fmt.Errorf("%s: missing ELF frame terminator", o.info.Name)
			}
			if count != 0 {
				frames = append(frames, im.base+uintptr(s.offset))
			}
		}
	}
	return frames, nil
}

type frameCIE struct {
	encoding  byte
	augmented bool
}

// DWARF32 CIE versions 1/3 and ordinary C augmentations. Language personality,
// LSDA, signal-frame and target-specific augmentations need runtime adapters.
func (im *image) validateELFFrames(b []byte, imageOffset uint64) (int, error) {
	cies := make(map[int]frameCIE)
	count := 0
	for offset := 0; offset < len(b); {
		if len(b)-offset < 4 {
			return 0, fmt.Errorf("truncated frame length")
		}
		size := uint64(binary.LittleEndian.Uint32(b[offset:]))
		if size == 0 {
			if !allZero(b[offset:]) {
				return 0, fmt.Errorf("data after frame terminator")
			}
			return count, nil
		}
		if size == 0xffffffff {
			return 0, fmt.Errorf("DWARF64 frame records are unsupported")
		}
		if size < 4 || size > uint64(len(b)-offset-4) {
			return 0, fmt.Errorf("invalid frame record length")
		}
		end := offset + 4 + int(size)
		id := binary.LittleEndian.Uint32(b[offset+4:])
		r := frameReader{b: b[offset+8 : end]}
		if id == 0 {
			version, err := r.byte()
			if err != nil || version != 1 && version != 3 {
				return 0, fmt.Errorf("unsupported CIE version")
			}
			zero := bytes.IndexByte(r.b, 0)
			if zero < 0 {
				return 0, fmt.Errorf("unterminated CIE augmentation")
			}
			augmentation := string(r.b[:zero])
			r.b = r.b[zero+1:]
			if augmentation != "" && augmentation != "zR" {
				return 0, fmt.Errorf("unsupported CIE augmentation %q", augmentation)
			}
			code, err := r.leb(false)
			if err != nil || code == 0 {
				return 0, fmt.Errorf("invalid CIE code alignment")
			}
			if _, err := r.leb(true); err != nil {
				return 0, err
			}
			if version == 1 {
				_, err = r.byte()
			} else {
				_, err = r.leb(false)
			}
			if err != nil {
				return 0, err
			}
			cie := frameCIE{augmented: augmentation == "zR"}
			if cie.augmented {
				n, err := r.leb(false)
				if err != nil || n != 1 {
					return 0, fmt.Errorf("invalid CIE augmentation data")
				}
				cie.encoding, err = r.byte()
				if err != nil {
					return 0, err
				}
			}
			if _, err := frameEncodingSize(cie.encoding, im.pointerSize); err != nil {
				return 0, err
			}
			if err := r.cfi(); err != nil {
				return 0, err
			}
			cies[offset] = cie
		} else {
			target := int64(offset) + 4 - int64(id)
			cie, ok := cies[int(target)]
			if target < 0 || !ok {
				return 0, fmt.Errorf("FDE does not reference a preceding CIE")
			}
			addressOffset := imageOffset + uint64(offset+8)
			start, err := r.encoded(cie.encoding, im.pointerSize)
			if err != nil {
				return 0, err
			}
			if cie.encoding&0x70 == 0x10 {
				start += int64(im.base) + int64(addressOffset)
			}
			length, err := r.encoded(cie.encoding&15, im.pointerSize)
			if err != nil || length <= 0 || start < int64(im.base) || !im.unwindRange(uint64(start-int64(im.base)), uint64(length), true) {
				return 0, fmt.Errorf("FDE range is outside an executable section")
			}
			if cie.augmented {
				n, err := r.leb(false)
				if err != nil || n != 0 {
					return 0, fmt.Errorf("unsupported FDE augmentation data")
				}
			}
			if err := r.cfi(); err != nil {
				return 0, err
			}
			count++
		}
		offset = end
	}
	return count, nil
}

type frameReader struct{ b []byte }

func (r *frameReader) take(n int) ([]byte, error) {
	if n < 0 || n > len(r.b) {
		return nil, fmt.Errorf("truncated frame data")
	}
	b := r.b[:n]
	r.b = r.b[n:]
	return b, nil
}
func (r *frameReader) byte() (byte, error) {
	b, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}
func (r *frameReader) leb(signed bool) (uint64, error) {
	var value uint64
	for i := 0; i < 10; i++ {
		b, err := r.byte()
		if err != nil {
			return 0, err
		}
		if i == 9 && (signed && b&0x7f != 0 && b&0x7f != 0x7f || !signed && b&0x7f > 1) {
			return 0, fmt.Errorf("frame LEB128 overflow")
		}
		value |= uint64(b&0x7f) << uint(i*7)
		if b&0x80 == 0 {
			if signed && b&0x40 != 0 && i < 9 {
				value |= ^uint64(0) << uint((i+1)*7)
			}
			return value, nil
		}
	}
	return 0, fmt.Errorf("unterminated frame LEB128")
}
func frameEncodingSize(encoding byte, pointerSize uint64) (int, error) {
	if encoding&0xf0 != 0 && encoding&0xf0 != 0x10 {
		return 0, fmt.Errorf("unsupported frame pointer encoding %#x", encoding)
	}
	switch encoding & 15 {
	case 0:
		return int(pointerSize), nil
	case 3, 11:
		return 4, nil
	case 4, 12:
		return 8, nil
	default:
		return 0, fmt.Errorf("unsupported frame pointer encoding %#x", encoding)
	}
}
func (r *frameReader) encoded(encoding byte, pointerSize uint64) (int64, error) {
	n, err := frameEncodingSize(encoding, pointerSize)
	if err != nil {
		return 0, err
	}
	b, err := r.take(n)
	if err != nil {
		return 0, err
	}
	if n == 4 {
		v := binary.LittleEndian.Uint32(b)
		if encoding&15 == 11 {
			return int64(int32(v)), nil
		}
		return int64(v), nil
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

// Check operand boundaries before handing CFI to libgcc. Native code and its
// CFI remain trusted; validation is not a proof of the machine-code prolog.
func (r *frameReader) cfi() error {
	depth := 0
	for len(r.b) != 0 {
		op, _ := r.byte()
		unsigned, signed, fixed := 0, 0, 0
		switch op & 0xc0 {
		case 0x40, 0xc0:
			continue
		case 0x80:
			unsigned = 1
		default:
			switch op {
			case 0:
				continue
			case 2:
				fixed = 1
			case 3:
				fixed = 2
			case 4:
				fixed = 4
			case 5, 9, 12, 20:
				unsigned = 2
			case 6, 7, 8, 13, 14, 46:
				unsigned = 1
			case 10:
				depth++
				continue
			case 11:
				if depth == 0 {
					return fmt.Errorf("unbalanced CFI restore_state")
				}
				depth--
				continue
			case 17, 18, 21:
				unsigned, signed = 1, 1
			case 19:
				signed = 1
			default:
				return fmt.Errorf("unsupported CFI opcode %#x", op)
			}
		}
		if _, err := r.take(fixed); err != nil {
			return err
		}
		for i := 0; i < unsigned; i++ {
			if _, err := r.leb(false); err != nil {
				return err
			}
		}
		for i := 0; i < signed; i++ {
			if _, err := r.leb(true); err != nil {
				return err
			}
		}
	}
	return nil
}

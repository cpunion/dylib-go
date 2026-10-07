package signature

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// ParseValue checks range and converts a literal to its exact ABI bit pattern.
// Integers accept Go literal bases and underscores. Pointers accept native
// numeric addresses or nil; no storage is allocated or dereferenced here.
func ParseValue(t abi.Type, text string) (abi.Value, error) {
	text = strings.TrimSpace(text)
	switch t {
	case abi.I8, abi.I16, abi.I32, abi.I64:
		width := 64
		if t == abi.I8 {
			width = 8
		} else if t == abi.I16 {
			width = 16
		}
		if t == abi.I32 {
			width = 32
		}
		v, err := strconv.ParseInt(text, 0, width)
		if err != nil {
			return abi.Value{}, fmt.Errorf("invalid signed %d-bit value %q: %w", width, text, err)
		}
		if t == abi.I8 {
			return abi.Int8(int8(v)), nil
		}
		if t == abi.I16 {
			return abi.Int16(int16(v)), nil
		}
		if t == abi.I32 {
			return abi.Int32(int32(v)), nil
		}
		return abi.Int64(v), nil
	case abi.U8, abi.U16, abi.U32, abi.U64, abi.Pointer:
		width := 64
		if t == abi.U8 {
			width = 8
		} else if t == abi.U16 {
			width = 16
		}
		if t == abi.U32 {
			width = 32
		} else if t == abi.Pointer {
			width = strconv.IntSize
			if text == "nil" {
				return abi.Ptr(0), nil
			}
		}
		v, err := strconv.ParseUint(text, 0, width)
		if err != nil {
			return abi.Value{}, fmt.Errorf("invalid unsigned %d-bit value %q: %w", width, text, err)
		}
		if t == abi.U8 {
			return abi.Uint8(uint8(v)), nil
		}
		if t == abi.U16 {
			return abi.Uint16(uint16(v)), nil
		}
		if t == abi.U32 {
			return abi.Uint32(uint32(v)), nil
		}
		if t == abi.Pointer {
			return abi.Ptr(uintptr(v)), nil
		}
		return abi.Uint64(v), nil
	case abi.F32, abi.F64:
		width := 64
		if t == abi.F32 {
			width = 32
		}
		v, err := strconv.ParseFloat(text, width)
		if err != nil {
			return abi.Value{}, fmt.Errorf("invalid %d-bit float %q: %w", width, text, err)
		}
		if t == abi.F32 {
			return abi.Float32(float32(v)), nil
		}
		return abi.Float64(v), nil
	case abi.Bool:
		v, err := strconv.ParseBool(text)
		if err != nil {
			return abi.Value{}, err
		}
		if text != "true" && text != "false" {
			return abi.Value{}, fmt.Errorf("bool requires true or false")
		}
		return abi.Boolean(v), nil
	default:
		return abi.Value{}, fmt.Errorf("type %d cannot be a scalar argument", t)
	}
}

// FormatValue prints the result without losing unsigned high bits or float
// precision. Void is printed as "void" and pointers as hexadecimal addresses.
func FormatValue(v abi.Value) (string, error) {
	if err := v.Description().Validate(); err != nil {
		return "", err
	}
	if err := v.Validate(v.Description()); err != nil {
		return "", err
	}
	return formatValue(v)
}

func formatValue(v abi.Value) (string, error) {
	switch v.Type {
	case abi.Void:
		return "void", nil
	case abi.Bool:
		return strconv.FormatBool(v.Bits != 0), nil
	case abi.I8:
		return strconv.FormatInt(int64(int8(v.Bits)), 10), nil
	case abi.I16:
		return strconv.FormatInt(int64(int16(v.Bits)), 10), nil
	case abi.U8:
		return strconv.FormatUint(uint64(uint8(v.Bits)), 10), nil
	case abi.U16:
		return strconv.FormatUint(uint64(uint16(v.Bits)), 10), nil
	case abi.Struct:
		if v.Aggregate == nil {
			return "", fmt.Errorf("missing struct fields")
		}
		var fields []string
		for i, f := range v.Aggregate.Fields {
			text, err := formatValue(f)
			if err != nil {
				return "", err
			}
			if name := v.Aggregate.Type.Fields[i].Name; name != "" {
				text = name + ":" + text
			}
			fields = append(fields, text)
		}
		return "{" + strings.Join(fields, ",") + "}", nil
	case abi.I32:
		return strconv.FormatInt(int64(int32(v.Bits)), 10), nil
	case abi.U32:
		return strconv.FormatUint(uint64(uint32(v.Bits)), 10), nil
	case abi.I64:
		return strconv.FormatInt(int64(v.Bits), 10), nil
	case abi.U64:
		return strconv.FormatUint(v.Bits, 10), nil
	case abi.F32:
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(v.Bits))), 'g', -1, 32), nil
	case abi.F64:
		return strconv.FormatFloat(math.Float64frombits(v.Bits), 'g', -1, 64), nil
	case abi.Pointer:
		if v.Pointee != nil {
			text, err := formatValue(*v.Pointee)
			return "&" + text, err
		}
		return "0x" + strconv.FormatUint(uint64(uintptr(v.Bits)), 16), nil
	default:
		return "", fmt.Errorf("invalid result type %d", v.Type)
	}
}

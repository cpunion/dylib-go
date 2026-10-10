package abi

import "fmt"

// TypeDesc describes a C scalar, ordinary struct, fixed-length array, or pointer.
// It does not describe packed structs, unions, bitfields, or Go objects.
type TypeDesc struct {
	Type   Type
	Fields []Field
	Elem   *TypeDesc
	Len    int // Array length; zero for all other types.
}

type Field struct {
	Name string
	Type TypeDesc
}

// Limit expanded aggregates before Zero or the native backend allocates them.
// Native storage is additionally limited to 64 KiB per value by the backend.
const maxAggregateElements = 65536

func (d TypeDesc) Validate() error {
	_, err := d.validate(0)
	return err
}
func (d TypeDesc) validate(depth int) (int, error) {
	if depth > 8 {
		return 0, fmt.Errorf("native type nesting exceeds 8 levels")
	}
	if d.Type > Array {
		return 0, fmt.Errorf("invalid native type %d", d.Type)
	}
	if d.Type != Struct && len(d.Fields) != 0 {
		return 0, fmt.Errorf("fields require a struct type")
	}
	if d.Type != Pointer && d.Type != Array && d.Elem != nil {
		return 0, fmt.Errorf("element type requires a pointer or array")
	}
	if d.Type != Array && d.Len != 0 {
		return 0, fmt.Errorf("length requires an array type")
	}
	if d.Type == Array && (d.Elem == nil || d.Len < 1 || d.Len > maxAggregateElements) {
		return 0, fmt.Errorf("arrays require an element type and 1 to %d elements", maxAggregateElements)
	}
	count := 1
	if d.Type == Struct {
		if len(d.Fields) == 0 || len(d.Fields) > 32 {
			return 0, fmt.Errorf("structs require 1 to 32 fields")
		}
		count = 0
		names := make(map[string]bool)
		for _, f := range d.Fields {
			if f.Name != "" && f.Name != "_" && names[f.Name] {
				return 0, fmt.Errorf("duplicate field %q", f.Name)
			}
			names[f.Name] = true
			if f.Type.Type == Void {
				return 0, fmt.Errorf("void struct field")
			}
			child, err := f.Type.validate(depth + 1)
			if err != nil {
				return 0, err
			}
			count += child
			if count > maxAggregateElements {
				return 0, fmt.Errorf("aggregate exceeds %d expanded elements", maxAggregateElements)
			}
		}
	}
	if d.Elem != nil {
		if d.Elem.Type == Void {
			return 0, fmt.Errorf("void element; use an opaque pointer for void pointers")
		}
		child, err := d.Elem.validate(depth + 1)
		if err != nil {
			return 0, err
		}
		if d.Type == Array {
			if d.Len > maxAggregateElements/child {
				return 0, fmt.Errorf("aggregate exceeds %d expanded elements", maxAggregateElements)
			}
			count = d.Len * child
		}
	}
	return count, nil
}

// Clone copies a descriptor that has passed Validate.
func (d TypeDesc) Clone() TypeDesc {
	c := TypeDesc{Type: d.Type, Len: d.Len}
	if d.Elem != nil {
		e := d.Elem.Clone()
		c.Elem = &e
	}
	for _, f := range d.Fields {
		c.Fields = append(c.Fields, Field{Name: f.Name, Type: f.Type.Clone()})
	}
	return c
}

func (s Signature) ArgumentType(i int) TypeDesc {
	if len(s.ArgTypes) != 0 {
		return s.ArgTypes[i]
	}
	return TypeDesc{Type: s.Args[i]}
}
func (s Signature) ReturnType() TypeDesc {
	if s.ResultType != nil {
		return *s.ResultType
	}
	return TypeDesc{Type: s.Result}
}

// Clone copies a signature that has passed Validate.
func (s Signature) Clone() Signature {
	c := s
	c.Args = append([]Type(nil), s.Args...)
	c.ArgTypes = nil
	for _, d := range s.ArgTypes {
		c.ArgTypes = append(c.ArgTypes, d.Clone())
	}
	if s.ResultType != nil {
		d := s.ResultType.Clone()
		c.ResultType = &d
	}
	return c
}

// Aggregate stores C struct fields or array elements as typed Go descriptions,
// never as raw Go memory. The native backend computes all offsets and padding.
type Aggregate struct {
	Type   TypeDesc
	Fields []Value // Struct fields in declaration order, or array elements by index.
}

func StructValue(d TypeDesc, fields ...Value) (Value, error) {
	if d.Type != Struct {
		return Value{}, fmt.Errorf("expected a struct descriptor")
	}
	return aggregateValue(d, fields)
}

// ArrayValue constructs an array for a struct field or temporary pointee.
// Supply exactly d.Len elements. C arrays cannot be passed by value directly.
func ArrayValue(d TypeDesc, elements ...Value) (Value, error) {
	if d.Type != Array {
		return Value{}, fmt.Errorf("expected an array descriptor")
	}
	return aggregateValue(d, elements)
}

func aggregateValue(d TypeDesc, members []Value) (Value, error) {
	if err := d.Validate(); err != nil {
		return Value{}, err
	}
	v := Value{Type: d.Type, Aggregate: &Aggregate{Type: d.Clone(), Fields: append([]Value(nil), members...)}}
	if err := v.Validate(d); err != nil {
		return Value{}, err
	}
	return v, nil
}

// AddressOf passes a temporary native copy and updates v after the call. Native
// code must not retain this address. Use Ptr for caller-managed native storage.
func AddressOf(v *Value) Value { return Value{Type: Pointer, Pointee: v} }

func hasTemporaryPointees(v Value) bool {
	if v.Pointee != nil {
		return true
	}
	if v.Aggregate != nil {
		for _, field := range v.Aggregate.Fields {
			if hasTemporaryPointees(field) {
				return true
			}
		}
	}
	return false
}

// Zero creates a zero value for a descriptor that has passed Validate.
func Zero(d TypeDesc) Value {
	v := Value{Type: d.Type}
	if d.Type == Struct || d.Type == Array {
		v.Aggregate = &Aggregate{Type: d.Clone()}
		for i := 0; i < d.memberCount(); i++ {
			v.Aggregate.Fields = append(v.Aggregate.Fields, Zero(d.memberType(i)))
		}
	}
	return v
}

func (v Value) Description() TypeDesc {
	if (v.Type == Struct || v.Type == Array) && v.Aggregate != nil {
		return v.Aggregate.Type
	}
	return TypeDesc{Type: v.Type}
}
func (v Value) Validate(d TypeDesc) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return v.validate(d, 0)
}
func (v Value) validate(d TypeDesc, depth int) error {
	if depth > 8 {
		return fmt.Errorf("value nesting exceeds 8 levels")
	}
	if v.Type != d.Type {
		return fmt.Errorf("value type mismatch: got %d, want %d", v.Type, d.Type)
	}
	if v.Type != Struct && v.Type != Array && v.Aggregate != nil {
		return fmt.Errorf("aggregate data requires a struct or array")
	}
	if v.Type != Pointer && v.Pointee != nil {
		return fmt.Errorf("pointee data requires a pointer")
	}
	if v.Type == Bool && v.Bits > 1 {
		return fmt.Errorf("boolean value must be 0 or 1")
	}
	if v.Type == Struct || v.Type == Array {
		if v.Aggregate == nil || len(v.Aggregate.Fields) != d.memberCount() {
			return fmt.Errorf("aggregate member count mismatch")
		}
		if err := v.Aggregate.Type.Validate(); err != nil {
			return err
		}
		if !sameLayout(v.Aggregate.Type, d) {
			return fmt.Errorf("aggregate layout mismatch")
		}
		for i, member := range v.Aggregate.Fields {
			if err := member.validate(d.memberType(i), depth+1); err != nil {
				return fmt.Errorf("aggregate member %d: %w", i, err)
			}
		}
	}
	if v.Pointee != nil {
		if v.Bits != 0 {
			return fmt.Errorf("use either a native address or a temporary pointee")
		}
		e := v.Pointee.Description()
		if d.Elem != nil {
			e = *d.Elem
		}
		if err := e.Validate(); err != nil {
			return err
		}
		return v.Pointee.validate(e, depth+1)
	}
	return nil
}

func sameLayout(a, b TypeDesc) bool {
	if a.Type != b.Type || len(a.Fields) != len(b.Fields) || a.Len != b.Len {
		return false
	}
	if a.Type == Array {
		return a.Elem != nil && b.Elem != nil && sameLayout(*a.Elem, *b.Elem)
	}
	for i := range a.Fields {
		if !sameLayout(a.Fields[i].Type, b.Fields[i].Type) {
			return false
		}
	}
	// Pointee annotations do not change the physical pointer layout.
	return true
}

// The same ordered-member representation is used for both aggregate kinds.
func (d TypeDesc) memberCount() int {
	if d.Type == Array {
		return d.Len
	}
	return len(d.Fields)
}
func (d TypeDesc) memberType(i int) TypeDesc {
	if d.Type == Array {
		return *d.Elem
	}
	return d.Fields[i].Type
}

package abi

import "fmt"

// TypeDesc describes an ordinary C struct or a pointer's known element type.
// It does not describe packed structs, unions, bitfields, arrays, or Go objects.
type TypeDesc struct {
	Type   Type
	Fields []Field
	Elem   *TypeDesc
}

type Field struct {
	Name string
	Type TypeDesc
}

func (d TypeDesc) Validate() error { return d.validate(0) }
func (d TypeDesc) validate(depth int) error {
	if depth > 8 {
		return fmt.Errorf("native type nesting exceeds 8 levels")
	}
	if d.Type > Struct {
		return fmt.Errorf("invalid native type %d", d.Type)
	}
	if d.Type != Struct && len(d.Fields) != 0 {
		return fmt.Errorf("fields require a struct type")
	}
	if d.Type != Pointer && d.Elem != nil {
		return fmt.Errorf("element type requires a pointer")
	}
	if d.Type == Struct {
		if len(d.Fields) == 0 || len(d.Fields) > 32 {
			return fmt.Errorf("structs require 1 to 32 fields")
		}
		names := make(map[string]bool)
		for _, f := range d.Fields {
			if f.Name != "" && f.Name != "_" && names[f.Name] {
				return fmt.Errorf("duplicate field %q", f.Name)
			}
			names[f.Name] = true
			if f.Type.Type == Void {
				return fmt.Errorf("void struct field")
			}
			if err := f.Type.validate(depth + 1); err != nil {
				return err
			}
		}
	}
	if d.Elem != nil {
		if d.Elem.Type == Void {
			return fmt.Errorf("void pointee; use an opaque pointer")
		}
		return d.Elem.validate(depth + 1)
	}
	return nil
}

// Clone copies a descriptor that has passed Validate.
func (d TypeDesc) Clone() TypeDesc {
	c := TypeDesc{Type: d.Type}
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

// Aggregate stores C struct fields as typed Go descriptions, never as raw Go
// struct memory. The native backend computes all offsets and padding.
type Aggregate struct {
	Type   TypeDesc
	Fields []Value
}

func StructValue(d TypeDesc, fields ...Value) (Value, error) {
	if d.Type != Struct {
		return Value{}, fmt.Errorf("expected a struct descriptor")
	}
	if err := d.Validate(); err != nil {
		return Value{}, err
	}
	v := Value{Type: Struct, Aggregate: &Aggregate{Type: d.Clone(), Fields: append([]Value(nil), fields...)}}
	if err := v.Validate(d); err != nil {
		return Value{}, err
	}
	return v, nil
}

// AddressOf passes a temporary native copy and updates v after the call. Native
// code must not retain this address. Use Ptr for caller-managed native storage.
func AddressOf(v *Value) Value { return Value{Type: Pointer, Pointee: v} }

// Zero creates a zero value for a descriptor that has passed Validate.
func Zero(d TypeDesc) Value {
	v := Value{Type: d.Type}
	if d.Type == Struct {
		v.Aggregate = &Aggregate{Type: d.Clone()}
		for _, f := range d.Fields {
			v.Aggregate.Fields = append(v.Aggregate.Fields, Zero(f.Type))
		}
	}
	return v
}

func (v Value) Description() TypeDesc {
	if v.Type == Struct && v.Aggregate != nil {
		return v.Aggregate.Type
	}
	return TypeDesc{Type: v.Type}
}
func (v Value) Validate(d TypeDesc) error { return v.validate(d, 0) }
func (v Value) validate(d TypeDesc, depth int) error {
	if depth > 8 {
		return fmt.Errorf("value nesting exceeds 8 levels")
	}
	if v.Type != d.Type {
		return fmt.Errorf("value type mismatch: got %d, want %d", v.Type, d.Type)
	}
	if v.Type != Struct && v.Aggregate != nil {
		return fmt.Errorf("aggregate data requires a struct")
	}
	if v.Type != Pointer && v.Pointee != nil {
		return fmt.Errorf("pointee data requires a pointer")
	}
	if v.Type == Bool && v.Bits > 1 {
		return fmt.Errorf("boolean value must be 0 or 1")
	}
	if v.Type == Struct {
		if v.Aggregate == nil || len(v.Aggregate.Fields) != len(d.Fields) || !sameLayout(v.Aggregate.Type, d) {
			return fmt.Errorf("struct layout or field count mismatch")
		}
		for i, f := range d.Fields {
			if err := v.Aggregate.Fields[i].validate(f.Type, depth+1); err != nil {
				return fmt.Errorf("field %q: %w", f.Name, err)
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
	if a.Type != b.Type || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		if !sameLayout(a.Fields[i].Type, b.Fields[i].Type) {
			return false
		}
	}
	// Pointee annotations do not change the physical pointer layout.
	return true
}

package zon

import (
	"fmt"
	"reflect"
)

// A SyntaxError reports that the input is not valid ZON. Line and Col point at
// the offending token (1-based).
type SyntaxError struct {
	msg  string
	Line int
	Col  int
}

// Error returns a formatted string representation of the syntax error.
func (e *SyntaxError) Error() string {
	return fmt.Sprintf("zon: %s (line %d, column %d)", e.msg, e.Line, e.Col)
}

// An UnmarshalTypeError describes a ZON value that was not appropriate for a
// value of a specific Go type.
type UnmarshalTypeError struct {
	Value string       // description of the ZON value, e.g. "string", "number 42"
	Type  reflect.Type // the Go type it could not be assigned to
}

// Error returns a formatted string describing the unmarshal type error.
func (e *UnmarshalTypeError) Error() string {
	return "zon: cannot unmarshal " + e.Value + " into Go value of type " + e.Type.String()
}

// An UnsupportedTypeError is returned by Marshal when attempting to encode a Go
// value whose type has no ZON representation.
type UnsupportedTypeError struct {
	Type reflect.Type
}

// Error returns a formatted string describing the unsupported type error.
func (e *UnsupportedTypeError) Error() string {
	return "zon: unsupported type: " + e.Type.String()
}

// An InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// The argument to Unmarshal must be a non-nil pointer.
type InvalidUnmarshalError struct {
	Type reflect.Type
}

// Error returns a formatted string describing the invalid unmarshal argument.
func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "zon: Unmarshal(nil)"
	}
	if e.Type.Kind() != reflect.Pointer {
		return "zon: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "zon: Unmarshal(nil " + e.Type.String() + ")"
}

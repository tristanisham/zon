package zon

// Marshaler is implemented by types that can marshal themselves into ZON.
//
// The returned bytes must be a single valid ZON value. They are written into
// the surrounding document verbatim, so a Marshaler is responsible for its own
// escaping and formatting.
type Marshaler interface {
	MarshalZON() ([]byte, error)
}

// Unmarshaler is implemented by types that can unmarshal a ZON description of
// themselves. The input is the ZON encoding of a single value. UnmarshalZON
// must copy the data if it wishes to retain it after returning.
type Unmarshaler interface {
	UnmarshalZON([]byte) error
}

// EnumLiteral represents a ZON enum literal such as .debug.
//
// It marshals to a leading dot followed by the value (for example
// EnumLiteral("debug") encodes as .debug) and is the Go type produced when an
// enum literal is decoded into an any. An enum literal may also be decoded into
// a plain Go string, in which case only the name (without the dot) is stored.
type EnumLiteral string

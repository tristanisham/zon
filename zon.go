// Package zon implements encoding and decoding of ZON (Zig Object Notation),
// the data format used by files such as build.zig.zon.
//
// ZON is a subset of Zig's literal syntax. A value is one of:
//
//   - an anonymous struct, .{ .name = "demo", .version = "0.1.0" }
//   - an anonymous tuple/array, .{ 1, 2, 3 }
//   - a string, "text", or a multiline string written with leading \\
//   - a number: integers (decimal, 0x, 0o, 0b, with _ separators) and floats
//     (including scientific and hex floats, plus inf and nan)
//   - a character literal, 'a', which decodes to its integer codepoint
//   - an enum literal, .debug
//   - true, false, or null
//
// Line comments (// ...) are permitted and ignored when decoding.
//
// The API mirrors encoding/json: use Marshal and Unmarshal for whole values, or
// Encoder and Decoder for streams. Struct fields are mapped using `zon` tags
// (for example `zon:"name,omitempty"`); a tag of "-" omits the field.
//
// Marshaler and Unmarshaler let a type control its own encoding. EnumLiteral
// represents ZON enum literals, which have no encoding/json equivalent.
package zon

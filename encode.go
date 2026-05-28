package zon

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Marshal returns the ZON encoding of v. Output is pretty-printed in the style
// of zig fmt: multi-line, indented with four spaces, with a trailing comma
// after each aggregate element.
//
// Go values map to ZON as follows: structs and string-keyed maps become
// .{ .field = v }; slices and arrays become .{ a, b, c }; strings, bools, and
// numbers map to their ZON literals; EnumLiteral becomes .name; nil pointers
// and interfaces become null. Types implementing Marshaler control their own
// encoding. Map keys are sorted for deterministic output.
func Marshal(v any) ([]byte, error) {
	return marshal(v, "", "    ")
}

// MarshalIndent is like Marshal but applies the given prefix and indent to each
// nested level. An empty indent produces compact, single-line output.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return marshal(v, prefix, indent)
}

func marshal(v any, prefix, indent string) ([]byte, error) {
	e := &encodeState{prefix: prefix, indent: indent}
	if err := e.encode(reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	return e.buf.Bytes(), nil
}

type encodeState struct {
	buf    bytes.Buffer
	prefix string
	indent string
	depth  int
}

var (
	marshalerType   = reflect.TypeFor[Marshaler]()
	enumLiteralType = reflect.TypeFor[EnumLiteral]()
)

func (e *encodeState) encode(rv reflect.Value) error {
	if rv.IsValid() {
		if m, ok := marshalerFor(rv); ok {
			raw, err := m.MarshalZON()
			if err != nil {
				return err
			}
			e.buf.Write(raw)
			return nil
		}
	}

	switch rv.Kind() {
	case reflect.Invalid:
		e.buf.WriteString("null")
		return nil
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			e.buf.WriteString("null")
			return nil
		}
		return e.encode(rv.Elem())
	case reflect.Bool:
		if rv.Bool() {
			e.buf.WriteString("true")
		} else {
			e.buf.WriteString("false")
		}
		return nil
	case reflect.String:
		if rv.Type() == enumLiteralType {
			e.buf.WriteByte('.')
			writeFieldName(&e.buf, rv.String())
			return nil
		}
		writeZonString(&e.buf, rv.String())
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.buf.WriteString(strconv.FormatInt(rv.Int(), 10))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.buf.WriteString(strconv.FormatUint(rv.Uint(), 10))
		return nil
	case reflect.Float32:
		e.encodeFloat(rv.Float(), 32)
		return nil
	case reflect.Float64:
		e.encodeFloat(rv.Float(), 64)
		return nil
	case reflect.Slice, reflect.Array:
		return e.encodeArray(rv)
	case reflect.Map:
		return e.encodeMap(rv)
	case reflect.Struct:
		return e.encodeStruct(rv)
	default:
		return &UnsupportedTypeError{Type: rv.Type()}
	}
}

func (e *encodeState) encodeFloat(f float64, bits int) {
	switch {
	case math.IsInf(f, 1):
		e.buf.WriteString("inf")
	case math.IsInf(f, -1):
		e.buf.WriteString("-inf")
	case math.IsNaN(f):
		e.buf.WriteString("nan")
	default:
		s := strconv.FormatFloat(f, 'g', -1, bits)
		// Ensure the result re-parses as a float rather than an integer.
		if !strings.ContainsAny(s, ".eEpP") {
			s += ".0"
		}
		e.buf.WriteString(s)
	}
}

func (e *encodeState) encodeArray(rv reflect.Value) error {
	n := rv.Len()
	if n == 0 {
		e.buf.WriteString(".{}")
		return nil
	}
	e.buf.WriteString(".{")
	e.depth++
	for i := range n {
		e.writeIndent()
		if err := e.encode(rv.Index(i)); err != nil {
			return err
		}
		e.writeSeparator(i == n-1)
	}
	e.depth--
	e.writeIndent()
	e.buf.WriteByte('}')
	return nil
}

func (e *encodeState) encodeStruct(rv reflect.Value) error {
	info := typeFields(rv.Type())
	type entry struct {
		name string
		val  reflect.Value
	}
	var out []entry
	for _, f := range info.ordered {
		fv := rv.Field(f.index)
		if f.omitEmpty && isEmptyValue(fv) {
			continue
		}
		out = append(out, entry{f.name, fv})
	}
	if len(out) == 0 {
		e.buf.WriteString(".{}")
		return nil
	}
	e.buf.WriteString(".{")
	e.depth++
	for i, p := range out {
		e.writeIndent()
		e.writeKey(p.name)
		if err := e.encode(p.val); err != nil {
			return err
		}
		e.writeSeparator(i == len(out)-1)
	}
	e.depth--
	e.writeIndent()
	e.buf.WriteByte('}')
	return nil
}

func (e *encodeState) encodeMap(rv reflect.Value) error {
	if rv.Type().Key().Kind() != reflect.String {
		return &UnsupportedTypeError{Type: rv.Type()}
	}
	if rv.Len() == 0 {
		e.buf.WriteString(".{}")
		return nil
	}
	keys := rv.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	e.buf.WriteString(".{")
	e.depth++
	for i, k := range keys {
		e.writeIndent()
		e.writeKey(k.String())
		if err := e.encode(rv.MapIndex(k)); err != nil {
			return err
		}
		e.writeSeparator(i == len(keys)-1)
	}
	e.depth--
	e.writeIndent()
	e.buf.WriteByte('}')
	return nil
}

// writeKey writes a struct/map field key (".name =" or ".name=" when compact).
func (e *encodeState) writeKey(name string) {
	e.buf.WriteByte('.')
	writeFieldName(&e.buf, name)
	if e.indent == "" {
		e.buf.WriteByte('=')
	} else {
		e.buf.WriteString(" = ")
	}
}

func (e *encodeState) writeIndent() {
	if e.indent == "" {
		return
	}
	e.buf.WriteByte('\n')
	e.buf.WriteString(e.prefix)
	for range e.depth {
		e.buf.WriteString(e.indent)
	}
}

// writeSeparator writes a comma between elements. In multi-line mode every
// element (including the last) gets a trailing comma; in compact mode only
// interior elements do.
func (e *encodeState) writeSeparator(last bool) {
	if e.indent != "" || !last {
		e.buf.WriteByte(',')
	}
}

func marshalerFor(rv reflect.Value) (Marshaler, bool) {
	t := rv.Type()
	if t.Implements(marshalerType) {
		if (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) && rv.IsNil() {
			return nil, false
		}
		return rv.Interface().(Marshaler), true
	}
	if rv.CanAddr() && reflect.PointerTo(t).Implements(marshalerType) {
		return rv.Addr().Interface().(Marshaler), true
	}
	return nil, false
}

// writeZonString writes s as a double-quoted ZON string literal with escaping.
// It iterates byte-wise so that bytes which are not part of a valid UTF-8
// sequence are emitted as \xNN raw-byte escapes (the decoder reads \xNN back as
// the same byte), keeping the encoding lossless for arbitrary byte content.
func writeZonString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		switch s[i] {
		case '"':
			b.WriteString(`\"`)
			i++
			continue
		case '\\':
			b.WriteString(`\\`)
			i++
			continue
		case '\n':
			b.WriteString(`\n`)
			i++
			continue
		case '\r':
			b.WriteString(`\r`)
			i++
			continue
		case '\t':
			b.WriteString(`\t`)
			i++
			continue
		}
		if c := s[i]; c < 0x20 {
			fmt.Fprintf(b, `\x%02x`, c)
			i++
			continue
		} else if c < utf8.RuneSelf {
			b.WriteByte(c)
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(b, `\x%02x`, s[i])
			i++
			continue
		}
		b.WriteString(s[i : i+size])
		i += size
	}
	b.WriteByte('"')
}

// writeFieldName writes a field name as a bare identifier when possible, or as
// an @"..." quoted identifier otherwise.
func writeFieldName(b *bytes.Buffer, name string) {
	if isValidIdent(name) {
		b.WriteString(name)
		return
	}
	b.WriteByte('@')
	writeZonString(b, name)
}

func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 0 && !isIdentStart(s[i]) {
			return false
		}
		if i > 0 && !isIdentPart(s[i]) {
			return false
		}
	}
	return true
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	}
	return false
}

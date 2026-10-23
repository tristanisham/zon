package zon

import (
	"bytes"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

// Unmarshal parses the ZON-encoded data and stores the result in the value
// pointed to by v. v must be a non-nil pointer.
//
// Unmarshal maps ZON values to Go values much like encoding/json: structs
// (.{ .a = 1 }) into Go structs (by `zon` tag or field name) or maps with
// string keys; tuples (.{ 1, 2 }) into slices or arrays; strings, numbers,
// bools, and null into the corresponding kinds. Enum literals (.debug) decode
// into an EnumLiteral or a string. As in Zig's std.zon, a float literal
// decodes into an integer type only if its value is exactly integral (1.0,
// 1e3). Unknown struct fields are ignored. Into an
// any, ZON decodes to map[string]any, []any, string, int64/uint64/float64,
// bool, EnumLiteral, or nil.
func Unmarshal(data []byte, v any) error {
	root, err := parse(string(data))
	if err != nil {
		return err
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	return decodeNode(root, rv.Elem())
}

// decodeNode recursively decodes an AST node into the target Go reflection value.
func decodeNode(n node, rv reflect.Value) error {
	if _, ok := n.(nullNode); ok {
		if rv.CanSet() {
			rv.Set(reflect.Zero(rv.Type()))
		}
		return nil
	}

	if u, target := indirect(rv); u != nil {
		return u.UnmarshalZON(renderNode(n))
	} else {
		rv = target
	}

	switch nd := n.(type) {
	case boolNode:
		return decodeBool(nd, rv)
	case stringNode:
		return decodeString(nd.value, "string", rv)
	case enumNode:
		return decodeEnum(nd, rv)
	case numberNode:
		return decodeNumber(nd, rv)
	case structNode:
		return decodeStruct(nd, rv)
	case tupleNode:
		return decodeTuple(nd, rv)
	}
	return nil
}

// indirect walks pointers and non-empty interfaces, allocating as needed, until
// it reaches a value implementing Unmarshaler or a non-pointer value. If an
// Unmarshaler is found it is returned and the reflect.Value is unset.
//
// If the starting value is an addressable named type, indirect first takes its
// address so that pointer-receiver Unmarshaler implementations are found.
func indirect(v reflect.Value) (Unmarshaler, reflect.Value) {
	v0 := v
	haveAddr := false
	if v.Kind() != reflect.Pointer && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Pointer && !e.IsNil() {
				haveAddr = false
				v = e
				continue
			}
		}
		if v.Kind() != reflect.Pointer {
			break
		}
		if v.IsNil() {
			if !v.CanSet() {
				break
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 && v.CanInterface() {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, reflect.Value{}
			}
		}
		if haveAddr {
			v = v0
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return nil, v
}

// decodeBool decodes a boolean AST node into the target Go value.
func decodeBool(nd boolNode, rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.Bool:
		rv.SetBool(nd.value)
		return nil
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			rv.Set(reflect.ValueOf(nd.value))
			return nil
		}
	}
	return &UnmarshalTypeError{Value: "bool", Type: rv.Type()}
}

// decodeString decodes a string AST node value into the target Go value.
func decodeString(s, desc string, rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.String:
		rv.SetString(s)
		return nil
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			rv.Set(reflect.ValueOf(s))
			return nil
		}
	}
	return &UnmarshalTypeError{Value: desc, Type: rv.Type()}
}

// decodeEnum decodes an enum literal AST node into the target Go value.
func decodeEnum(nd enumNode, rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.String:
		rv.SetString(nd.name)
		return nil
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			rv.Set(reflect.ValueOf(EnumLiteral(nd.name)))
			return nil
		}
	}
	return &UnmarshalTypeError{Value: "enum literal", Type: rv.Type()}
}

// decodeNumber decodes a number AST node into the target Go numeric type.
func decodeNumber(nd numberNode, rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var i int64
		var err error
		if nd.isFloat {
			i, err = floatToInt(nd.raw)
		} else {
			i, err = parseInt(nd.raw)
		}
		if err != nil || rv.OverflowInt(i) {
			return numErr(nd, rv)
		}
		rv.SetInt(i)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var u uint64
		var err error
		if nd.isFloat {
			u, err = floatToUint(nd.raw)
		} else {
			u, err = parseUint(nd.raw)
		}
		if err != nil || rv.OverflowUint(u) {
			return numErr(nd, rv)
		}
		rv.SetUint(u)
		return nil
	case reflect.Float32, reflect.Float64:
		f, err := parseFloat(nd.raw)
		if err != nil {
			return numErr(nd, rv)
		}
		rv.SetFloat(f)
		return nil
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			return decodeNumberToAny(nd, rv)
		}
	}
	return numErr(nd, rv)
}

// decodeNumberToAny decodes a number AST node into an interface{} (any), deciding the narrowest matching type.
func decodeNumberToAny(nd numberNode, rv reflect.Value) error {
	if nd.isFloat {
		f, err := parseFloat(nd.raw)
		if err != nil {
			return numErr(nd, rv)
		}
		rv.Set(reflect.ValueOf(f))
		return nil
	}
	if i, err := parseInt(nd.raw); err == nil {
		rv.Set(reflect.ValueOf(i))
		return nil
	}
	if u, err := parseUint(nd.raw); err == nil {
		rv.Set(reflect.ValueOf(u))
		return nil
	}
	if f, err := parseFloat(nd.raw); err == nil {
		rv.Set(reflect.ValueOf(f))
		return nil
	}
	return numErr(nd, rv)
}

// numErr returns an UnmarshalTypeError for an invalid or non-matching number literal.
func numErr(nd numberNode, rv reflect.Value) error {
	return &UnmarshalTypeError{Value: "number " + nd.raw, Type: rv.Type()}
}

// decodeStruct decodes a ZON struct node into a Go struct, map, or interface{}.
func decodeStruct(nd structNode, rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.Struct:
		info := typeFields(rv.Type())
		for _, f := range nd.fields {
			sf, ok := info.byName[f.name]
			if !ok {
				continue // ignore unknown fields
			}
			if err := decodeNode(f.value, rv.Field(sf.index)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		return decodeStructToMap(nd, rv)
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			m := make(map[string]any, len(nd.fields))
			for _, f := range nd.fields {
				var elem any
				ev := reflect.ValueOf(&elem).Elem()
				if err := decodeNode(f.value, ev); err != nil {
					return err
				}
				m[f.name] = elem
			}
			rv.Set(reflect.ValueOf(m))
			return nil
		}
	}
	return &UnmarshalTypeError{Value: "struct", Type: rv.Type()}
}

// decodeStructToMap decodes a ZON struct node into a Go map.
func decodeStructToMap(nd structNode, rv reflect.Value) error {
	t := rv.Type()
	if t.Key().Kind() != reflect.String {
		return &UnmarshalTypeError{Value: "struct", Type: t}
	}
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(t))
	}
	for _, f := range nd.fields {
		ev := reflect.New(t.Elem()).Elem()
		if err := decodeNode(f.value, ev); err != nil {
			return err
		}
		kv := reflect.New(t.Key()).Elem()
		kv.SetString(f.name)
		rv.SetMapIndex(kv, ev)
	}
	return nil
}

// decodeTuple decodes a ZON tuple node into a Go slice, array, or interface{}.
func decodeTuple(nd tupleNode, rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.Slice:
		s := reflect.MakeSlice(rv.Type(), len(nd.items), len(nd.items))
		for i, it := range nd.items {
			if err := decodeNode(it, s.Index(i)); err != nil {
				return err
			}
		}
		rv.Set(s)
		return nil
	case reflect.Array:
		n := rv.Len()
		for i, it := range nd.items {
			if i >= n {
				break
			}
			if err := decodeNode(it, rv.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			arr := make([]any, len(nd.items))
			for i, it := range nd.items {
				var elem any
				ev := reflect.ValueOf(&elem).Elem()
				if err := decodeNode(it, ev); err != nil {
					return err
				}
				arr[i] = elem
			}
			rv.Set(reflect.ValueOf(arr))
			return nil
		}
	case reflect.Struct, reflect.Map:
		if len(nd.items) == 0 {
			return nil // empty .{} into a struct or map
		}
	}
	return &UnmarshalTypeError{Value: "array", Type: rv.Type()}
}

// Number literals reach these helpers already validated against Zig's rules by
// the lexer, so digit separators can simply be dropped: Go's own separator
// rules differ from Zig's (Go accepts 0x_1, Zig does not) and must not decide.

// parseInt parses a raw numeric literal string as an int64.
func parseInt(raw string) (int64, error) {
	return strconv.ParseInt(strings.ReplaceAll(raw, "_", ""), 0, 64)
}

// parseUint parses a raw numeric literal string as a uint64.
func parseUint(raw string) (uint64, error) {
	return strconv.ParseUint(strings.ReplaceAll(raw, "_", ""), 0, 64)
}

// parseFloat parses a raw numeric literal string as a float64. Zig allows a
// hex float without a binary exponent (0x1.8), which Go requires, so p0 is
// supplied. A literal beyond the float64 range is not an error in Zig: like a
// float literal coerced to f64 there, it rounds to ±Inf (or to zero).
func parseFloat(raw string) (float64, error) {
	f, err := strconv.ParseFloat(goFloatSyntax(raw), 64)
	if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
		return f, nil
	}
	return f, err
}

// goFloatSyntax rewrites a validated Zig float literal into Go's float syntax.
func goFloatSyntax(raw string) string {
	s := strings.ReplaceAll(raw, "_", "")
	if u := strings.TrimPrefix(s, "-"); strings.HasPrefix(u, "0x") && !strings.ContainsAny(u, "pP") {
		s += "p0"
	}
	return s
}

// f128Prec is the significand precision of Zig's f128, in which ZON float
// literals are held before being converted to a target type.
const f128Prec = 113

// exactFloat returns the float literal raw rounded to f128 precision, or an
// error if it is not finite or not an integer. Zig's std.zon.parse decodes a
// float literal into an integer type only when it is exactly integral.
func exactFloat(raw string) (*big.Int, error) {
	f, _, err := big.ParseFloat(goFloatSyntax(raw), 0, f128Prec, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	if f.IsInf() || !f.IsInt() {
		return nil, strconv.ErrSyntax
	}
	i, _ := f.Int(nil)
	return i, nil
}

// floatToInt converts an integral float literal to an int64.
func floatToInt(raw string) (int64, error) {
	i, err := exactFloat(raw)
	if err != nil {
		return 0, err
	}
	if !i.IsInt64() {
		return 0, strconv.ErrRange
	}
	return i.Int64(), nil
}

// floatToUint converts an integral float literal to a uint64.
func floatToUint(raw string) (uint64, error) {
	i, err := exactFloat(raw)
	if err != nil {
		return 0, err
	}
	if !i.IsUint64() {
		return 0, strconv.ErrRange
	}
	return i.Uint64(), nil
}

// renderNode serializes a parsed node back to compact ZON. It is used to feed
// the original encoding of a value to a custom Unmarshaler.
func renderNode(n node) []byte {
	var b bytes.Buffer
	renderNodeTo(&b, n)
	return b.Bytes()
}

// renderNodeTo serializes a parsed node recursively to a compact bytes buffer.
func renderNodeTo(b *bytes.Buffer, n node) {
	switch nd := n.(type) {
	case nullNode:
		b.WriteString("null")
	case boolNode:
		if nd.value {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case numberNode:
		b.WriteString(nd.raw)
	case stringNode:
		writeZonString(b, nd.value)
	case enumNode:
		b.WriteByte('.')
		writeFieldName(b, nd.name)
	case structNode:
		b.WriteString(".{")
		for i, f := range nd.fields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('.')
			writeFieldName(b, f.name)
			b.WriteByte('=')
			renderNodeTo(b, f.value)
		}
		b.WriteByte('}')
	case tupleNode:
		b.WriteString(".{")
		for i, it := range nd.items {
			if i > 0 {
				b.WriteByte(',')
			}
			renderNodeTo(b, it)
		}
		b.WriteByte('}')
	}
}

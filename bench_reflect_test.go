package zon

import (
	"strconv"
	"testing"
)

// Reflection-heavy paths: maps, pointers, decode-into-any, and custom
// Marshaler/Unmarshaler implementations.

const reflectMapDoc = `.{ .one = 1, .two = 2, .three = 3, .four = 4, .five = 5 }`

const reflectAnyDoc = `.{ .name = "x", .nums = .{ 1, 2, 3 }, .nested = .{ .ok = true, .val = 3.5 } }`

// reflectColor is a custom Marshaler/Unmarshaler encoding an int as ".#RRGGBB".
type reflectColor struct{ rgb int }

func (c reflectColor) MarshalZON() ([]byte, error) {
	return []byte(`"#` + strconv.FormatInt(int64(c.rgb), 16) + `"`), nil
}

func (c *reflectColor) UnmarshalZON(data []byte) error {
	s := string(data)
	s = s[2 : len(s)-1] // strip "# prefix and trailing "
	v, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return err
	}
	c.rgb = int(v)
	return nil
}

func BenchmarkReflect_DecodeMap(b *testing.B) {
	data := []byte(reflectMapDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v map[string]int
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReflect_EncodeMap(b *testing.B) {
	v := map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReflect_DecodePointers(b *testing.B) {
	data := []byte(`.{ .a = 1, .b = 2 }`)
	type inner struct {
		A *int `zon:"a"`
		B *int `zon:"b"`
	}
	b.ReportAllocs()
	for b.Loop() {
		var v *inner
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReflect_DecodeAny(b *testing.B) {
	data := []byte(reflectAnyDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReflect_CustomMarshaler(b *testing.B) {
	v := reflectColor{rgb: 0xAABBCC}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReflect_CustomUnmarshaler(b *testing.B) {
	data := []byte(`"#aabbcc"`)
	b.ReportAllocs()
	for b.Loop() {
		var v reflectColor
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

package zon

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestUnmarshalStruct(t *testing.T) {
	type Dep struct {
		URL  string `zon:"url"`
		Hash string `zon:"hash"`
	}
	type Manifest struct {
		Name    string         `zon:"name"`
		Version string         `zon:"version"`
		Deps    map[string]Dep `zon:"dependencies"`
		Paths   []string       `zon:"paths"`
	}
	src := `.{
        .name = "demo",
        .version = "0.1.0",
        .dependencies = .{
            .foo = .{ .url = "https://example.com", .hash = "abc123" },
        },
        .paths = .{ "", "src" },
    }`
	var m Manifest
	if err := Unmarshal([]byte(src), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Name != "demo" || m.Version != "0.1.0" {
		t.Errorf("name/version = %q/%q", m.Name, m.Version)
	}
	if m.Deps["foo"].URL != "https://example.com" || m.Deps["foo"].Hash != "abc123" {
		t.Errorf("deps = %+v", m.Deps)
	}
	if !reflect.DeepEqual(m.Paths, []string{"", "src"}) {
		t.Errorf("paths = %+v", m.Paths)
	}
}

func TestUnmarshalNumbers(t *testing.T) {
	var v struct {
		I   int     `zon:"i"`
		U   uint64  `zon:"u"`
		F   float64 `zon:"f"`
		Hex int     `zon:"hex"`
		Bin int     `zon:"bin"`
		Sep int     `zon:"sep"`
	}
	src := `.{ .i = -7, .u = 42, .f = 3.5, .hex = 0xFF, .bin = 0b101, .sep = 1_000 }`
	if err := Unmarshal([]byte(src), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.I != -7 || v.U != 42 || v.F != 3.5 || v.Hex != 255 || v.Bin != 5 || v.Sep != 1000 {
		t.Errorf("got %+v", v)
	}
}

func TestUnmarshalSpecialFloats(t *testing.T) {
	var v struct {
		A float64 `zon:"a"`
		B float64 `zon:"b"`
		C float64 `zon:"c"`
	}
	if err := Unmarshal([]byte(`.{ .a = inf, .b = -inf, .c = nan }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !math.IsInf(v.A, 1) || !math.IsInf(v.B, -1) || !math.IsNaN(v.C) {
		t.Errorf("got %+v", v)
	}
}

func TestUnmarshalEnumLiteral(t *testing.T) {
	var v struct {
		Mode EnumLiteral `zon:"mode"`
		Name string      `zon:"name"`
	}
	if err := Unmarshal([]byte(`.{ .mode = .debug, .name = .release }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.Mode != "debug" {
		t.Errorf("mode = %q, want debug", v.Mode)
	}
	if v.Name != "release" {
		t.Errorf("name = %q, want release (enum into string)", v.Name)
	}
}

func TestUnmarshalIntoAny(t *testing.T) {
	var v any
	if err := Unmarshal([]byte(`.{ .a = 1, .b = .{ "x", true }, .c = .tag }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map[string]any", v)
	}
	if m["a"].(int64) != 1 {
		t.Errorf("a = %v", m["a"])
	}
	b := m["b"].([]any)
	if b[0].(string) != "x" || b[1].(bool) != true {
		t.Errorf("b = %v", b)
	}
	if m["c"].(EnumLiteral) != "tag" {
		t.Errorf("c = %v", m["c"])
	}
}

func TestUnmarshalPointers(t *testing.T) {
	var v struct {
		P *int    `zon:"p"`
		N *string `zon:"n"`
	}
	if err := Unmarshal([]byte(`.{ .p = 5, .n = null }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.P == nil || *v.P != 5 {
		t.Errorf("p = %v", v.P)
	}
	if v.N != nil {
		t.Errorf("n = %v, want nil", v.N)
	}
}

func TestUnmarshalUnknownFieldIgnored(t *testing.T) {
	var v struct {
		Known int `zon:"known"`
	}
	if err := Unmarshal([]byte(`.{ .known = 1, .unknown = 2 }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.Known != 1 {
		t.Errorf("known = %d", v.Known)
	}
}

func TestUnmarshalQuotedFieldName(t *testing.T) {
	var v map[string]int
	if err := Unmarshal([]byte(`.{ .@"weird name" = 7 }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v["weird name"] != 7 {
		t.Errorf("got %+v", v)
	}
}

func TestUnmarshalArray(t *testing.T) {
	var v [3]int
	if err := Unmarshal([]byte(`.{ 1, 2, 3 }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v != [3]int{1, 2, 3} {
		t.Errorf("got %v", v)
	}
}

// rgb implements Unmarshaler by parsing a hex string into a packed integer.
type rgb uint32

func (c *rgb) UnmarshalZON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	var v uint32
	for _, ch := range s {
		v = v*16 + uint32(hexVal(byte(ch)))
	}
	*c = rgb(v)
	return nil
}

func TestUnmarshalerInterface(t *testing.T) {
	var v struct {
		Color rgb `zon:"color"`
	}
	if err := Unmarshal([]byte(`.{ .color = "ff8800" }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.Color != 0xff8800 {
		t.Errorf("color = %x, want ff8800", uint32(v.Color))
	}
}

func TestUnmarshalTypeError(t *testing.T) {
	var v struct {
		N int `zon:"n"`
	}
	err := Unmarshal([]byte(`.{ .n = "not a number" }`), &v)
	if err == nil {
		t.Fatal("expected type error")
	}
	if _, ok := err.(*UnmarshalTypeError); !ok {
		t.Errorf("error type = %T, want *UnmarshalTypeError", err)
	}
}

func TestUnmarshalInvalidArg(t *testing.T) {
	var v int
	if err := Unmarshal([]byte(`1`), v); err == nil {
		t.Fatal("expected InvalidUnmarshalError for non-pointer")
	}
}

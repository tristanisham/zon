package zon

import (
	"fmt"
	"strings"
	"testing"
)

func TestMarshalScalars(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{-7, "-7"},
		{uint8(255), "255"},
		{3.5, "3.5"},
		{5.0, "5.0"}, // must stay a float, not "5"
		{"hi", `"hi"`},
		{EnumLiteral("debug"), ".debug"},
		{(*int)(nil), "null"},
	}
	for _, tc := range cases {
		got, err := Marshal(tc.v)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tc.v, err)
		}
		if string(got) != tc.want {
			t.Errorf("Marshal(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestMarshalStringEscaping(t *testing.T) {
	got, err := Marshal("a\nb\t\"c\\")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `"a\nb\t\"c\\"`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarshalStructPretty(t *testing.T) {
	v := struct {
		Name  string `zon:"name"`
		Count int    `zon:"count"`
	}{"demo", 3}
	got, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := ".{\n    .name = \"demo\",\n    .count = 3,\n}"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarshalCompact(t *testing.T) {
	v := struct {
		A int `zon:"a"`
		B int `zon:"b"`
	}{1, 2}
	got, err := MarshalIndent(v, "", "")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	want := ".{.a=1,.b=2}"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarshalOmitEmpty(t *testing.T) {
	v := struct {
		A int `zon:"a"`
		B int `zon:"b,omitempty"`
		C int `zon:"-"`
	}{A: 1, B: 0, C: 99}
	got, err := MarshalIndent(v, "", "")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if string(got) != ".{.a=1}" {
		t.Errorf("got %q, want %q", got, ".{.a=1}")
	}
}

func TestMarshalSlice(t *testing.T) {
	got, err := MarshalIndent([]int{1, 2, 3}, "", "")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if string(got) != ".{1,2,3}" {
		t.Errorf("got %q", got)
	}
	empty, _ := Marshal([]int{})
	if string(empty) != ".{}" {
		t.Errorf("empty slice = %q, want .{}", empty)
	}
}

func TestMarshalMapSorted(t *testing.T) {
	got, err := MarshalIndent(map[string]int{"b": 2, "a": 1, "c": 3}, "", "")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if string(got) != ".{.a=1,.b=2,.c=3}" {
		t.Errorf("got %q, want sorted keys", got)
	}
}

func TestMarshalQuotedFieldName(t *testing.T) {
	got, err := MarshalIndent(map[string]int{"weird name": 1}, "", "")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if !strings.Contains(string(got), `.@"weird name"=1`) {
		t.Errorf("got %q, want quoted field name", got)
	}
}

// version implements Marshaler by emitting a quoted semver string.
type version struct {
	major, minor, patch int
}

func (v version) MarshalZON() ([]byte, error) {
	return fmt.Appendf(nil, "%q", fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)), nil
}

func TestMarshalerInterface(t *testing.T) {
	got, err := Marshal(struct {
		V version `zon:"version"`
	}{version{1, 2, 3}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := ".{\n    .version = \"1.2.3\",\n}"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarshalUnsupportedType(t *testing.T) {
	_, err := Marshal(make(chan int))
	if err == nil {
		t.Fatal("expected UnsupportedTypeError")
	}
	if _, ok := err.(*UnsupportedTypeError); !ok {
		t.Errorf("error type = %T, want *UnsupportedTypeError", err)
	}
}

type CircularNode struct {
	Self *CircularNode `zon:"self"`
}

func TestMarshalMaxDepth(t *testing.T) {
	node := CircularNode{}
	node.Self = &node

	_, err := Marshal(node)
	if err == nil {
		t.Fatal("expected error for marshaling circular reference, got none")
	}
	if !strings.Contains(err.Error(), "exceeded maximum encoding depth limit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

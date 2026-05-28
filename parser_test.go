package zon

import (
	"strings"
	"testing"
)

func TestParseStruct(t *testing.T) {
	n, err := parse(`.{ .name = "demo", .count = 3 }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, ok := n.(structNode)
	if !ok {
		t.Fatalf("got %T, want structNode", n)
	}
	if len(s.fields) != 2 || s.fields[0].name != "name" || s.fields[1].name != "count" {
		t.Fatalf("fields = %+v", s.fields)
	}
	if sv, ok := s.fields[0].value.(stringNode); !ok || sv.value != "demo" {
		t.Errorf("field 0 value = %+v", s.fields[0].value)
	}
}

func TestParseTuple(t *testing.T) {
	n, err := parse(`.{ 1, 2, 3 }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tup, ok := n.(tupleNode)
	if !ok {
		t.Fatalf("got %T, want tupleNode", n)
	}
	if len(tup.items) != 3 {
		t.Fatalf("items = %d, want 3", len(tup.items))
	}
}

// A tuple whose elements are enum literals must not be mistaken for a struct.
func TestParseTupleOfEnums(t *testing.T) {
	n, err := parse(`.{ .a, .b }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tup, ok := n.(tupleNode)
	if !ok {
		t.Fatalf("got %T, want tupleNode", n)
	}
	if len(tup.items) != 2 {
		t.Fatalf("items = %d, want 2", len(tup.items))
	}
	if e, ok := tup.items[0].(enumNode); !ok || e.name != "a" {
		t.Errorf("item 0 = %+v, want enum a", tup.items[0])
	}
}

func TestParseEmptyAggregate(t *testing.T) {
	n, err := parse(`.{}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tup, ok := n.(tupleNode); !ok || len(tup.items) != 0 {
		t.Fatalf("got %+v, want empty tupleNode", n)
	}
}

func TestParseScalars(t *testing.T) {
	cases := map[string]node{
		"true":  boolNode{value: true},
		"false": boolNode{value: false},
		"null":  nullNode{},
		".foo":  enumNode{name: "foo"},
		"-5":    numberNode{raw: "-5"},
		"-2.5":  numberNode{raw: "-2.5", isFloat: true},
		"inf":   numberNode{raw: "inf", isFloat: true},
		"-inf":  numberNode{raw: "-inf", isFloat: true},
	}
	for src, want := range cases {
		got, err := parse(src)
		if err != nil {
			t.Fatalf("parse(%q): %v", src, err)
		}
		if got != want {
			t.Errorf("parse(%q) = %+v, want %+v", src, got, want)
		}
	}
}

func TestParseNested(t *testing.T) {
	n, err := parse(`.{ .deps = .{ .foo = .{ .url = "u" } }, .paths = .{ "" } }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s := n.(structNode)
	deps := s.fields[0].value.(structNode)
	foo := deps.fields[0].value.(structNode)
	if foo.fields[0].name != "url" {
		t.Errorf("nested field = %q, want url", foo.fields[0].name)
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{
		`.{ .a = }`,
		`.{ .a 1 }`,
		`.{ 1 2 }`,
		`.{ .a = 1 `,
		`.{ .a = 1 } extra`,
	} {
		if _, err := parse(src); err == nil {
			t.Errorf("parse(%q) succeeded, want error", src)
		}
	}
}

func TestParseMaxDepth(t *testing.T) {
	// Generate a deeply nested structure, e.g., 1001 levels of ".{"
	var sb strings.Builder
	for range 1001 {
		sb.WriteString(".{")
	}
	for range 1001 {
		sb.WriteString("}")
	}
	_, err := parse(sb.String())
	if err == nil {
		t.Fatal("expected error for parsing deep aggregate structure, got none")
	}
	if !strings.Contains(err.Error(), "exceeded maximum parsing depth") {
		t.Errorf("unexpected error message: %v", err)
	}
}

package zon

import "testing"

// Scalars: bool and null across lex, parse, decode, encode.

func BenchmarkScalars_LexBool(b *testing.B) {
	src := "true"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := lex(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalars_ParseBool(b *testing.B) {
	src := "false"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalars_DecodeBool(b *testing.B) {
	data := []byte("true")
	b.ReportAllocs()
	for b.Loop() {
		var v bool
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalars_DecodeNull(b *testing.B) {
	data := []byte("null")
	b.ReportAllocs()
	for b.Loop() {
		v := 1
		p := &v
		if err := Unmarshal(data, &p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalars_DecodeBoolAny(b *testing.B) {
	data := []byte("false")
	b.ReportAllocs()
	for b.Loop() {
		var v any
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalars_EncodeBool(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(true); err != nil {
			b.Fatal(err)
		}
	}
}

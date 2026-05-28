package zon

import "testing"

// Strings: quoted strings, escape sequences, and unicode content.

var stringsInputs = []struct {
	name string
	src  string
}{
	{"Plain", `"the quick brown fox jumps over the lazy dog"`},
	{"Escapes", `"line1\nline2\ttabbed\r\n\"quoted\" and \\slash\\"`},
	{"HexEscape", `"\x41\x42\x43\x44"`},
	{"UnicodeEscape", `"snowman \u{2603} and emoji \u{1F600}"`},
	{"Unicode", `"héllo wörld — 日本語 — Ω≈ç√∫"`},
	{"Empty", `""`},
}

func BenchmarkStrings_Lex(b *testing.B) {
	for _, in := range stringsInputs {
		b.Run(in.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := lex(in.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStrings_DecodeString(b *testing.B) {
	for _, in := range stringsInputs {
		b.Run(in.name, func(b *testing.B) {
			data := []byte(in.src)
			b.ReportAllocs()
			for b.Loop() {
				var v string
				if err := Unmarshal(data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStrings_Encode(b *testing.B) {
	vals := []string{
		"the quick brown fox",
		"line1\nline2\ttabbed\r\n\"quoted\"",
		"héllo wörld — 日本語",
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, s := range vals {
			if _, err := Marshal(s); err != nil {
				b.Fatal(err)
			}
		}
	}
}

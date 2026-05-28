package zon

import "testing"

// Multiline strings (\\ lines joined with \n) and character literals.

const multilineCharsDoc = `\\fn main() void {
\\    const greeting = "hello";
\\    std.debug.print("{s}\n", .{greeting});
\\}`

var multilineCharsCharInputs = []struct {
	name string
	src  string
}{
	{"ASCII", "'a'"},
	{"Escape", `'\n'`},
	{"HexEscape", `'\x7f'`},
	{"Unicode", "'☃'"},
}

func BenchmarkMultilineChars_LexMultiline(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := lex(multilineCharsDoc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultilineChars_DecodeMultiline(b *testing.B) {
	data := []byte(multilineCharsDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v string
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultilineChars_LexChar(b *testing.B) {
	for _, in := range multilineCharsCharInputs {
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

func BenchmarkMultilineChars_DecodeChar(b *testing.B) {
	data := []byte("'☃'")
	b.ReportAllocs()
	for b.Loop() {
		var v int32
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

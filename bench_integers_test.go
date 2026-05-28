package zon

import "testing"

// Integers: decimal, hex, octal, binary, and underscore separators.

var integersInputs = []struct {
	name string
	src  string
}{
	{"Decimal", "1234567"},
	{"Hex", "0xDEADBEEF"},
	{"Octal", "0o755"},
	{"Binary", "0b1010_1010"},
	{"Underscores", "1_000_000_000"},
	{"Negative", "-987654321"},
}

func BenchmarkIntegers_Lex(b *testing.B) {
	for _, in := range integersInputs {
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

func BenchmarkIntegers_Parse(b *testing.B) {
	for _, in := range integersInputs {
		b.Run(in.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := parse(in.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkIntegers_DecodeInt64(b *testing.B) {
	for _, in := range integersInputs {
		b.Run(in.name, func(b *testing.B) {
			data := []byte(in.src)
			b.ReportAllocs()
			for b.Loop() {
				var v int64
				if err := Unmarshal(data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkIntegers_DecodeUint64(b *testing.B) {
	data := []byte("0xDEADBEEF")
	b.ReportAllocs()
	for b.Loop() {
		var v uint64
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegers_EncodeInt(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(int64(-987654321)); err != nil {
			b.Fatal(err)
		}
	}
}

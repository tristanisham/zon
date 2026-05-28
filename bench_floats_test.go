package zon

import (
	"math"
	"testing"
)

// Floats: decimal, scientific, hex floats, and the ZON special values inf/nan.

var floatsInputs = []struct {
	name string
	src  string
}{
	{"Decimal", "3.14159265358979"},
	{"Scientific", "6.022e23"},
	{"NegExponent", "1.602e-19"},
	{"HexFloat", "0x1.fp10"},
	{"Inf", "inf"},
	{"NegInf", "-inf"},
	{"Nan", "nan"},
}

func BenchmarkFloats_Lex(b *testing.B) {
	for _, in := range floatsInputs {
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

func BenchmarkFloats_Parse(b *testing.B) {
	for _, in := range floatsInputs {
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

func BenchmarkFloats_DecodeFloat64(b *testing.B) {
	for _, in := range floatsInputs {
		b.Run(in.name, func(b *testing.B) {
			data := []byte(in.src)
			b.ReportAllocs()
			for b.Loop() {
				var v float64
				if err := Unmarshal(data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFloats_Encode(b *testing.B) {
	vals := []float64{3.14159265358979, 6.022e23, math.Inf(1), math.Inf(-1), math.NaN()}
	b.ReportAllocs()
	for b.Loop() {
		for _, f := range vals {
			if _, err := Marshal(f); err != nil {
				b.Fatal(err)
			}
		}
	}
}

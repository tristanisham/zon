package zon

import "testing"

// Structs: flat and wide structs, zon tags, and omitempty encoding.

type structsFlat struct {
	Name    string `zon:"name"`
	Version string `zon:"version"`
	Count   int    `zon:"count"`
	Enabled bool   `zon:"enabled"`
}

type structsWide struct {
	F0 int     `zon:"f0"`
	F1 int     `zon:"f1"`
	F2 int     `zon:"f2"`
	F3 int     `zon:"f3"`
	F4 string  `zon:"f4"`
	F5 string  `zon:"f5"`
	F6 bool    `zon:"f6"`
	F7 bool    `zon:"f7"`
	F8 float64 `zon:"f8"`
	F9 float64 `zon:"f9"`
}

type structsOmit struct {
	Name string   `zon:"name,omitempty"`
	Desc string   `zon:"desc,omitempty"`
	N    int      `zon:"n,omitempty"`
	Tags []string `zon:"tags,omitempty"`
}

const structsFlatDoc = `.{ .name = "example", .version = "1.2.3", .count = 42, .enabled = true }`

const structsWideDoc = `.{ .f0 = 0, .f1 = 1, .f2 = 2, .f3 = 3, .f4 = "four", .f5 = "five", .f6 = true, .f7 = false, .f8 = 8.8, .f9 = 9.9 }`

func BenchmarkStructs_DecodeFlat(b *testing.B) {
	data := []byte(structsFlatDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v structsFlat
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructs_DecodeWide(b *testing.B) {
	data := []byte(structsWideDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v structsWide
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructs_EncodeFlat(b *testing.B) {
	v := structsFlat{Name: "example", Version: "1.2.3", Count: 42, Enabled: true}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructs_EncodeWideCompact(b *testing.B) {
	v := structsWide{F4: "four", F5: "five", F6: true, F8: 8.8, F9: 9.9}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := MarshalIndent(v, "", ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructs_EncodeOmitEmpty(b *testing.B) {
	v := structsOmit{Name: "only-name"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

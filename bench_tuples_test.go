package zon

import "testing"

// Tuples/arrays: flat tuples, nested arrays, and decode into slices/arrays.

const tuplesFlatDoc = `.{ 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 }`

const tuplesNestedDoc = `.{ .{ 1, 2, 3 }, .{ 4, 5, 6 }, .{ 7, 8, 9 } }`

const tuplesStringsDoc = `.{ "alpha", "beta", "gamma", "delta", "epsilon" }`

func BenchmarkTuples_Parse(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parse(tuplesFlatDoc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTuples_DecodeSlice(b *testing.B) {
	data := []byte(tuplesFlatDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v []int
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTuples_DecodeArray(b *testing.B) {
	data := []byte(tuplesFlatDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v [10]int
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTuples_DecodeNested(b *testing.B) {
	data := []byte(tuplesNestedDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v [][]int
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTuples_DecodeStrings(b *testing.B) {
	data := []byte(tuplesStringsDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v []string
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTuples_EncodeSlice(b *testing.B) {
	v := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTuples_EncodeNested(b *testing.B) {
	v := [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

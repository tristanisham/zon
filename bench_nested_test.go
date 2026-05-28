package zon

import (
	"os"
	"strings"
	"testing"
)

// Deeply nested documents and a realistic build.zig.zon round-trip.

// nestedDeepDoc builds a left-nested chain .{ .child = .{ .child = ... } }.
func nestedDeepDoc(depth int) string {
	var b strings.Builder
	for range depth {
		b.WriteString(".{ .child = ")
	}
	b.WriteString(".{ .leaf = 1 }")
	for range depth {
		b.WriteString(" }")
	}
	return b.String()
}

func BenchmarkNested_ParseDeep(b *testing.B) {
	for _, depth := range []int{4, 16, 64} {
		src := nestedDeepDoc(depth)
		b.Run("depth"+itoaNested(depth), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := parse(src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkNested_DecodeDeepAny(b *testing.B) {
	src := []byte(nestedDeepDoc(32))
	b.ReportAllocs()
	for b.Loop() {
		var v any
		if err := Unmarshal(src, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNested_BuildZigZonDecode(b *testing.B) {
	data, err := os.ReadFile("testdata/build.zig.zon")
	if err != nil {
		b.Fatalf("read testdata: %v", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var m Manifest
		if err := Unmarshal(data, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNested_BuildZigZonRoundTrip(b *testing.B) {
	data, err := os.ReadFile("testdata/build.zig.zon")
	if err != nil {
		b.Fatalf("read testdata: %v", err)
	}
	var m Manifest
	if err := Unmarshal(data, &m); err != nil {
		b.Fatalf("seed unmarshal: %v", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		out, err := Marshal(m)
		if err != nil {
			b.Fatal(err)
		}
		var back Manifest
		if err := Unmarshal(out, &back); err != nil {
			b.Fatal(err)
		}
	}
}

// itoaNested keeps this file free of fmt while labeling sub-benchmarks.
func itoaNested(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

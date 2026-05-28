package zon

import "testing"

// Enum literals (.debug) and EnumLiteral round-trips.

const enumsDoc = `.{ .a = .debug, .b = .ReleaseFast, .c = .some_long_enum_name }`

type enumsTarget struct {
	A EnumLiteral `zon:"a"`
	B EnumLiteral `zon:"b"`
	C EnumLiteral `zon:"c"`
}

func BenchmarkEnums_LexLiteral(b *testing.B) {
	src := ".debug"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := lex(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEnums_ParseLiteral(b *testing.B) {
	src := ".ReleaseFast"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEnums_DecodeToEnumLiteral(b *testing.B) {
	data := []byte(".debug")
	b.ReportAllocs()
	for b.Loop() {
		var v EnumLiteral
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEnums_DecodeStruct(b *testing.B) {
	data := []byte(enumsDoc)
	b.ReportAllocs()
	for b.Loop() {
		var v enumsTarget
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEnums_Encode(b *testing.B) {
	v := enumsTarget{A: "debug", B: "ReleaseFast", C: "some_long_enum_name"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEnums_RoundTrip(b *testing.B) {
	v := enumsTarget{A: "debug", B: "ReleaseFast", C: "some_long_enum_name"}
	b.ReportAllocs()
	for b.Loop() {
		out, err := Marshal(v)
		if err != nil {
			b.Fatal(err)
		}
		var back enumsTarget
		if err := Unmarshal(out, &back); err != nil {
			b.Fatal(err)
		}
	}
}

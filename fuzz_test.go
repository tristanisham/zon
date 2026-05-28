package zon

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fuzzUnmarshalSeeds are valid and adversarial ZON inputs used to seed the
// Unmarshal/Lex/Parse fuzz corpora.
var fuzzUnmarshalSeeds = []string{
	// scalars
	"true",
	"false",
	"null",
	// strings
	`"hello"`,
	`"with \"quotes\" and \n\t escapes"`,
	`"unicode \u{1F600} and \x41"`,
	// numbers
	"0",
	"42",
	"-7",
	"1_000_000",
	"0xFF",
	"0o755",
	"0b1010",
	"-0x10",
	// floats
	"3.14",
	"-2.5e10",
	"0x1.8p3",
	"inf",
	"-inf",
	"nan",
	// enum literals
	".debug",
	".release_fast",
	// char literals (decode to codepoint)
	"'a'",
	`'\n'`,
	"'é'",
	// aggregates
	".{}",
	".{ 1, 2, 3 }",
	".{ .name = \"demo\", .version = \"0.1.0\" }",
	".{ .nested = .{ .a = 1, .b = .{ 2, 3 } } }",
	".{ .mode = .debug, .opt = true }",
	// multiline string
	"\\\\line one\n\\\\line two",
	// comments
	"// a comment\n42",
	".{ // inline\n .a = 1 }",
	// quoted field names
	`.{ .@"weird key" = 1 }`,
	// adversarial / malformed
	"",
	" ",
	"\"unterminated",
	".{",
	".{ .a = }",
	"0123",       // leading-zero decimal, must be rejected
	".{ 1 2 3 }", // missing commas
	strings.Repeat(".{", 200) + strings.Repeat("}", 200), // deep nesting
	"\\",
	"@",
	"'",
	".{,}",
	"--5",
	"1.2.3",
}

// FuzzUnmarshalNoPanic feeds arbitrary bytes into Unmarshal against several
// target types. The property is simply that Unmarshal never panics; returning
// an error is an acceptable outcome.
func FuzzUnmarshalNoPanic(f *testing.F) {
	for _, s := range fuzzUnmarshalSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var anyV any
		_ = Unmarshal(data, &anyV)

		var s string
		_ = Unmarshal(data, &s)

		var i int64
		_ = Unmarshal(data, &i)

		var fl float64
		_ = Unmarshal(data, &fl)

		var b bool
		_ = Unmarshal(data, &b)

		type fuzzTarget struct {
			Name    string         `zon:"name"`
			Version string         `zon:"version"`
			Count   int            `zon:"count"`
			Ratio   float64        `zon:"ratio"`
			Items   []int          `zon:"items"`
			Extra   map[string]any `zon:"extra"`
			Mode    EnumLiteral    `zon:"mode"`
		}
		var tgt fuzzTarget
		_ = Unmarshal(data, &tgt)
	})
}

// FuzzLexNoPanic feeds arbitrary strings into the internal lexer. It must never
// panic; an error return is fine.
func FuzzLexNoPanic(f *testing.F) {
	for _, s := range fuzzUnmarshalSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		_, _ = lex(src)
	})
}

// FuzzParseNoPanic feeds arbitrary strings into the internal parser. It must
// never panic; an error return is fine.
func FuzzParseNoPanic(f *testing.F) {
	for _, s := range fuzzUnmarshalSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		_, _ = parse(src)
	})
}

// fuzzContainsNaN reports whether a decoded value contains a NaN float, which
// breaks reflect.DeepEqual (NaN != NaN) and so must be excluded from the
// round-trip property.
func fuzzContainsNaN(v any) bool {
	switch x := v.(type) {
	case float64:
		return math.IsNaN(x)
	case float32:
		return math.IsNaN(float64(x))
	case map[string]any:
		for _, e := range x {
			if fuzzContainsNaN(e) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(x, fuzzContainsNaN)
	}
	return false
}

// FuzzMarshalUnmarshalRoundTrip asserts the decode -> encode -> decode round
// trip is stable: if an input decodes into `any`, re-marshalling it and
// decoding again must yield a reflect.DeepEqual value. Inputs that fail the
// first decode are skipped (they are not round-trip candidates), as are inputs
// whose value contains NaN.
func FuzzMarshalUnmarshalRoundTrip(f *testing.F) {
	for _, s := range fuzzUnmarshalSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var first any
		if err := Unmarshal(data, &first); err != nil {
			return // not a valid ZON document; nothing to round-trip
		}
		if fuzzContainsNaN(first) {
			return // NaN is never DeepEqual to itself
		}

		out, err := Marshal(first)
		if err != nil {
			// A value that decoded must be re-encodable; if not, that is a real
			// asymmetry worth surfacing.
			t.Fatalf("Marshal failed for value decoded from %q: %v", data, err)
		}

		var second any
		if err := Unmarshal(out, &second); err != nil {
			t.Fatalf("re-Unmarshal of marshalled output failed\ninput:  %q\noutput: %s\nerror:  %v", data, out, err)
		}

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("round-trip mismatch\ninput:  %q\noutput: %s\nfirst:  %#v\nsecond: %#v", data, out, first, second)
		}
	})
}

// TestNonUTF8StringRoundTrips pins the fix for the encoder-lossiness bug found
// by FuzzMarshalUnmarshalRoundTrip. The lexer preserves arbitrary (non-UTF-8)
// bytes inside string and multiline-string literals; the encoder must emit those
// bytes as \xNN raw-byte escapes (not U+FFFD) so the value survives a
// Marshal/Unmarshal round trip. The decoder reads \xNN inside a string back as
// the same raw byte, matching Zig's byte-escape semantics.
func TestNonUTF8StringRoundTrips(t *testing.T) {
	inputs := []string{
		"\\\\\xff",             // multiline string whose single byte is 0xFF
		`.{.@"\x98"}`,          // enum literal with a non-UTF-8 byte in a quoted name
		"\"raw \x80\xfe end\"", // quoted string with raw invalid bytes
	}
	for _, input := range inputs {
		var first any
		if err := Unmarshal([]byte(input), &first); err != nil {
			t.Fatalf("Unmarshal(%q): %v", input, err)
		}

		out, err := Marshal(first)
		if err != nil {
			t.Fatalf("Marshal of value from %q: %v", input, err)
		}

		var second any
		if err := Unmarshal(out, &second); err != nil {
			t.Fatalf("re-Unmarshal of %q (from input %q): %v", out, input, err)
		}

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("non-UTF-8 round trip mismatch for input %q\noutput: %s\nfirst:  %#v\nsecond: %#v",
				input, out, first, second)
		}
	}
}

// TestStringHexEscapeIsRawByte pins that \xNN inside a string decodes to a single
// raw byte (Zig byte-escape semantics), distinct from \u{...} which is a codepoint.
func TestStringHexEscapeIsRawByte(t *testing.T) {
	var s string
	if err := Unmarshal([]byte(`"\xff"`), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s != "\xff" {
		t.Fatalf(`"\xff" decoded to %q (% x), want single byte 0xFF`, s, s)
	}
}

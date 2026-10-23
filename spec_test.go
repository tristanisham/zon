package zon

import (
	"math"
	"testing"
)

// inf and nan are valid ZON float literals (the one thing ZON adds on top of
// Zig's literal subset), and must round-trip.
func TestSpecInfNanRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		text string
		want float64
	}{
		{"inf", math.Inf(1)},
		{"-inf", math.Inf(-1)},
	} {
		var f float64
		if err := Unmarshal([]byte(tc.text), &f); err != nil {
			t.Fatalf("Unmarshal(%q): %v", tc.text, err)
		}
		if f != tc.want {
			t.Errorf("Unmarshal(%q) = %v, want %v", tc.text, f, tc.want)
		}
		out, err := Marshal(tc.want)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tc.want, err)
		}
		if string(out) != tc.text {
			t.Errorf("Marshal(%v) = %q, want %q", tc.want, out, tc.text)
		}
	}

	var nan float64
	if err := Unmarshal([]byte("nan"), &nan); err != nil {
		t.Fatalf("Unmarshal(nan): %v", err)
	}
	if !math.IsNaN(nan) {
		t.Errorf("Unmarshal(nan) = %v, want NaN", nan)
	}
}

// Character literals are valid ZON values and decode to their integer codepoint.
func TestSpecCharLiteralValue(t *testing.T) {
	var v struct {
		A int   `zon:"a"`
		B rune  `zon:"b"`
		C int32 `zon:"c"`
	}
	if err := Unmarshal([]byte(`.{ .a = 'A', .b = '\n', .c = '😀' }`), &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.A != 65 || v.B != '\n' || v.C != 0x1F600 {
		t.Errorf("got %+v", v)
	}
}

// Leading zeros are fine within hex/octal/binary; only decimal forbids them.
func TestSpecLeadingZeroDigitsInOtherBases(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"0x0F", 15},
		{"0o07", 7},
		{"0b0010", 2},
		{"-0x10", -16},
	} {
		var n int
		if err := Unmarshal([]byte(tc.text), &n); err != nil {
			t.Fatalf("Unmarshal(%q): %v", tc.text, err)
		}
		if n != tc.want {
			t.Errorf("Unmarshal(%q) = %d, want %d", tc.text, n, tc.want)
		}
	}
}

// Enum literals round-trip through EnumLiteral.
func TestSpecEnumLiteralRoundTrip(t *testing.T) {
	in := struct {
		Mode EnumLiteral `zon:"mode"`
	}{Mode: "release_fast"}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back struct {
		Mode EnumLiteral `zon:"mode"`
	}
	if err := Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal of %s: %v", out, err)
	}
	if back.Mode != "release_fast" {
		t.Errorf("mode = %q, want release_fast", back.Mode)
	}
}

// Zig (and therefore ZON) forbids leading zeros on decimal integer literals:
// `0123` is a syntax error, not octal. Octal must be written `0o123`.
func TestLeadingZeroIntegerRejected(t *testing.T) {
	var n int
	if err := Unmarshal([]byte(`0123`), &n); err == nil {
		t.Fatalf("expected error for leading-zero integer, got n=%d", n)
	}
	// A lone zero is still valid.
	if err := Unmarshal([]byte(`0`), &n); err != nil {
		t.Fatalf("Unmarshal(0): unexpected error %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, want 0", n)
	}
}

// std.zon.parse decodes a float literal into an integer type when its value is
// exactly integral (intFromFloatExact), and rejects it otherwise.
func TestSpecIntegralFloatIntoInt(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int64
	}{
		{"1.0", 1}, {"-2.0", -2}, {"1e3", 1000}, {"0x1p4", 16}, {"0x1.8p1", 3},
		{"1_000.0", 1000}, {"-0.0", 0}, {"9007199254740993.0", 9007199254740993},
		{"9223372036854775807.0", math.MaxInt64},
	} {
		var i int64
		if err := Unmarshal([]byte(tc.src), &i); err != nil {
			t.Errorf("Unmarshal(%q, *int64): %v", tc.src, err)
		} else if i != tc.want {
			t.Errorf("Unmarshal(%q, *int64) = %d, want %d", tc.src, i, tc.want)
		}
	}
	var u uint64
	if err := Unmarshal([]byte("18446744073709551615.0"), &u); err != nil || u != math.MaxUint64 {
		t.Errorf("Unmarshal(18446744073709551615.0, *uint64) = %d, %v", u, err)
	}
	for _, src := range []string{"1.5", "1e-1", "inf", "nan", "-1.0", "256.0", "9223372036854775808.0"} {
		var i8 int8
		var u8 uint8
		var i64 int64
		errI8, errU8, errI64 := Unmarshal([]byte(src), &i8), Unmarshal([]byte(src), &u8), Unmarshal([]byte(src), &i64)
		switch src {
		case "-1.0":
			if errI8 != nil || errU8 == nil {
				t.Errorf("%q: int8 err %v (want nil), uint8 err %v (want error)", src, errI8, errU8)
			}
		case "256.0":
			if errI8 == nil || errU8 == nil || errI64 != nil {
				t.Errorf("%q: int8 %v, uint8 %v (want errors), int64 %v (want nil)", src, errI8, errU8, errI64)
			}
		case "9223372036854775808.0":
			if errI64 == nil {
				t.Errorf("%q into int64: want overflow error", src)
			}
		default:
			if errI8 == nil || errU8 == nil || errI64 == nil {
				t.Errorf("%q: want errors for every integer type, got %v %v %v", src, errI8, errU8, errI64)
			}
		}
	}
}

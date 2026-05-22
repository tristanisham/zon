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

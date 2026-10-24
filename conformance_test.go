package zon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These cases exercise the syntax rules in Zig's Ast, ZonGen, tokenizer,
// number_literal, and string_literal, rather than Go's literal grammar.
var conformanceCases = []struct {
	name, source string
	valid        bool
}{
	{"spaced aggregate", ". /* no */ { }", false},
	{"whitespace between dot and brace", ". \n { .a = 1 }", true},
	{"comment between dot and brace", ". // comment\n { 1, 2, }", true},
	{"duplicate fields", `.{.a=1,.a=2}`, false},
	{"escaped duplicate fields", `.{.a=1,.@"\x61"=2}`, false},
	{"keyword field", `.{.type=1}`, true},
	{"reserved field", `.{.struct=1}`, false},
	{"reserved enum", `.while`, false},
	{"quoted keyword", `.{.@"struct"=.@"while"}`, true},
	{"quoted boolean is not boolean", `@"true"`, false},
	{"quoted infinity is not infinity", `@"inf"`, false},
	{"empty enum identifier", `.@""`, false},
	{"empty field identifier", `.{.@""=1}`, false},
	{"null identifier", `.@"\x00"`, false},
	{"boolean enum", `.true`, true},
	{"negative integer zero", `-0`, false},
	{"negative hex zero", `-0x00`, false},
	{"negative float zero", `-0.0`, true},
	{"negative character", `-'a'`, false},
	{"negative nan", `-nan`, false},
	{"uppercase hex prefix", `0Xff`, false},
	{"uppercase octal prefix", `0O77`, false},
	{"uppercase binary prefix", `0B11`, false},
	{"underscore after base", `0x_FF`, false},
	{"double underscore", `1__2`, false},
	{"trailing underscore", `12_`, false},
	{"underscore before period", `1_.5`, false},
	{"hex underscore before period", `0x1_.5`, false},
	{"period before decimal exponent", `1.e5`, true},
	{"period before hex exponent", `0x1.p0`, true},
	{"underscore after period", `1._0`, false},
	{"underscore before exponent", `1_e2`, false},
	{"leading zero float", `01.0`, false},
	{"leading zero exponent", `00e1`, false},
	{"hex float without exponent", `0x1.8`, true},
	{"hex exponent", `0x1p4`, true},
	{"hex leading period", `0x.8`, true},
	{"valid separators", `.{1_000,0xff_ff,0o7_7,0b1_0,1.0_5e1_0}`, true},
	{"bad number in ignored field", `.{.ignored=0b2}`, false},
	{"bad exponent in ignored field", `.{.ignored=1e}`, false},
	{"unicode above maximum", `"\u{110000}"`, false},
	{"unicode overflow", `"\u{100000041}"`, false},
	{"surrogate string", `"\u{d800}"`, false},
	{"valid unicode escape", `"\u{0000001f600}"`, true},
	{"raw tab string", "\"a\tb\"", false},
	{"raw carriage return string", "\"a\rb\"", false},
	{"raw nul string", "\"a\x00b\"", false},
	{"raw delete string", "\"a\x7fb\"", false},
	{"raw tab character", "'\t'", false},
	{"empty character", "''", false},
	{"quote character", "'''", false},
	{"byte escape", `"\xff"`, true},
	{"raw non UTF8 string", "\"\xff\"", true},
	{"raw non UTF8 multiline", "\\\\\xff", true},
	{"multiline across blank line", "\\\\one\n\n  \\\\two\n", true},
	{"multiline across comment", "\\\\one\n// comment\n\\\\two\n", true},
	{"multiline CRLF", "\\\\one\r\n\\\\two\r\n", true},
	{"raw tab multiline", "\\\\a\tb\n", false},
	{"nul comment", "//\x00\n0", false},
	{"doc comment", "/// doc\n0", false},
	{"container doc comment", "//! doc\n0", false},
	{"arbitrary precision integer", `1234567890123456789012345678901234567890`, true},
	{"trailing document", `1 2`, false},
	{"block comment", `/* comment */ 0`, false},
	{"grouped value", `(1)`, false},
}

func TestConformanceSyntax(t *testing.T) {
	for _, tc := range conformanceCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(tc.source)
			if (err == nil) != tc.valid {
				t.Errorf("parse(%q): %v; valid=%v", tc.source, err, tc.valid)
			}
			// Invalid literals must also fail when the destination ignores their field.
			if !tc.valid {
				var discard struct{}
				if err := Unmarshal([]byte(tc.source), &discard); err == nil {
					t.Error("Unmarshal accepted invalid document")
				}
			}
		})
	}
}

// Opt in with ZON_TEST_ZIG=/path/to/zig. Zig checks the same corpus, plus Go
// encoder output, independently of the Go parser. No Zig dependency for users.
func TestZigConformance(t *testing.T) {
	zig := os.Getenv("ZON_TEST_ZIG")
	if zig == "" {
		t.Skip("set ZON_TEST_ZIG to run the Zig reference parser")
	}
	var source bytes.Buffer
	source.WriteString("const std = @import(\"std\");\n")
	cases := append([]struct {
		name, source string
		valid        bool
	}(nil), conformanceCases...)
	for _, value := range []any{map[string]any{"struct": EnumLiteral("while"), "name": "\x7f\xff"}, []any{1, 1.5, true, nil}, EnumLiteral("fn")} {
		encoded, err := Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, struct {
			name, source string
			valid        bool
		}{fmt.Sprintf("encoder %d", len(cases)), string(encoded), true})
	}
	for i, tc := range cases {
		fmt.Fprintf(&source, "test \"case %d\" {\n const src = \"", i)
		for _, b := range []byte(tc.source) {
			fmt.Fprintf(&source, "\\x%02x", b)
		}
		fmt.Fprintf(&source, "\";\n var tree = try std.zig.Ast.parse(std.testing.allocator, src, .{ .mode = .zon });\n defer tree.deinit(std.testing.allocator);\n var zoir = try std.zig.ZonGen.generate(std.testing.allocator, tree, .{});\n defer zoir.deinit(std.testing.allocator);\n try std.testing.expectEqual(%v, zoir.compile_errors.len == 0);\n}\n", tc.valid)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "conformance.zig")
	if err := os.WriteFile(path, source.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zig, "test", path, "--cache-dir", filepath.Join(dir, "cache"), "--global-cache-dir", filepath.Join(os.TempDir(), "zon-zig-cache"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		for i, tc := range cases {
			t.Logf("case %d: %s (%q)", i, tc.name, tc.source)
		}
		t.Fatalf("Zig reference: %v\n%s", err, out)
	}
	t.Log(strings.TrimSpace(string(out)))
}

func TestConformanceValues(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   any
	}{
		{`0x1.8`, float64(1.5)}, {`0x.8`, float64(0.5)},
		{"\\\\one\n\n// comment\n  \\\\two\r\n", "one\ntwo"},
	} {
		var got any
		if err := Unmarshal([]byte(tc.source), &got); err != nil {
			t.Errorf("%q: %v", tc.source, err)
		} else if got != tc.want {
			t.Errorf("%q = %#v, want %#v", tc.source, got, tc.want)
		}
	}
	var f float64
	if err := Unmarshal([]byte(`0xff`), &f); err != nil || f != 255 {
		t.Errorf("hex integer to float: %v, %v", f, err)
	}
	const integer = "1234567890123456789012345678901234567890"
	var n any
	if err := Unmarshal([]byte(integer), &n); err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(n)
	if err != nil || string(out) != integer {
		t.Errorf("integer lost precision: %s, %v", out, err)
	}
	for _, value := range []any{EnumLiteral(""), EnumLiteral("a\x00b"), map[string]int{"": 1}, map[string]int{"\x00": 1}} {
		if out, err := Marshal(value); err == nil {
			t.Errorf("Marshal(%#v) accepted invalid identifier: %s", value, out)
		}
	}
}

func TestEncoderRejectsDuplicateTags(t *testing.T) {
	value := struct {
		A int `zon:"x"`
		B int `zon:"x"`
	}{1, 2}
	if out, err := Marshal(value); err == nil {
		t.Fatalf("encoder emitted duplicate fields: %s", out)
	}
}

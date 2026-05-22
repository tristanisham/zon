package zon

import "testing"

func kinds(toks []token) []tokenKind {
	ks := make([]tokenKind, 0, len(toks))
	for _, t := range toks {
		ks = append(ks, t.kind)
	}
	return ks
}

func TestLexStructTokens(t *testing.T) {
	toks, err := lex(`.{ .name = "x" }`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	want := []tokenKind{tokenDotBrace, tokenDot, tokenIdent, tokenEquals, tokenString, tokenRBrace, tokenEOF}
	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %v, want %v", i, got[i], want[i])
		}
	}
	if toks[2].value != "name" {
		t.Errorf("field name = %q, want %q", toks[2].value, "name")
	}
	if toks[4].value != "x" {
		t.Errorf("string value = %q, want %q", toks[4].value, "x")
	}
}

func TestLexNumbers(t *testing.T) {
	tests := []struct {
		src   string
		kind  tokenKind
		value string
	}{
		{"42", tokenInt, "42"},
		{"0x1F", tokenInt, "0x1F"},
		{"0o17", tokenInt, "0o17"},
		{"0b1010", tokenInt, "0b1010"},
		{"1_000_000", tokenInt, "1_000_000"},
		{"3.14", tokenFloat, "3.14"},
		{"1e9", tokenFloat, "1e9"},
		{"2.5e-3", tokenFloat, "2.5e-3"},
		{"0x1.8p4", tokenFloat, "0x1.8p4"},
	}
	for _, tc := range tests {
		toks, err := lex(tc.src)
		if err != nil {
			t.Fatalf("lex(%q): %v", tc.src, err)
		}
		if toks[0].kind != tc.kind || toks[0].value != tc.value {
			t.Errorf("lex(%q) = {%v %q}, want {%v %q}", tc.src, toks[0].kind, toks[0].value, tc.kind, tc.value)
		}
	}
}

func TestLexStringEscapes(t *testing.T) {
	toks, err := lex(`"a\nb\t\"c\\\x41\u{1F600}"`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	want := "a\nb\t\"c\\A\U0001F600"
	if toks[0].kind != tokenString || toks[0].value != want {
		t.Errorf("string = %q, want %q", toks[0].value, want)
	}
}

func TestLexMultilineString(t *testing.T) {
	src := "\\\\line one\n    \\\\line two\n"
	toks, err := lex(src)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	want := "line one\nline two"
	if toks[0].kind != tokenString || toks[0].value != want {
		t.Errorf("multiline = %q, want %q", toks[0].value, want)
	}
}

func TestLexCharLiteral(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"'a'", "97"},
		{`'\n'`, "10"},
		{"'😀'", "128512"},
	} {
		toks, err := lex(tc.src)
		if err != nil {
			t.Fatalf("lex(%q): %v", tc.src, err)
		}
		if toks[0].kind != tokenInt || toks[0].value != tc.want {
			t.Errorf("lex(%q) = {%v %q}, want int %q", tc.src, toks[0].kind, toks[0].value, tc.want)
		}
	}
}

func TestLexSkipsComments(t *testing.T) {
	src := "// leading comment\n.{ .a = 1 } // trailing\n"
	toks, err := lex(src)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	want := []tokenKind{tokenDotBrace, tokenDot, tokenIdent, tokenEquals, tokenInt, tokenRBrace, tokenEOF}
	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLexQuotedIdent(t *testing.T) {
	toks, err := lex(`@"weird name"`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if toks[0].kind != tokenIdent || toks[0].value != "weird name" {
		t.Errorf("quoted ident = {%v %q}", toks[0].kind, toks[0].value)
	}
}

func TestLexUnterminatedStringError(t *testing.T) {
	_, err := lex(`"oops`)
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
	if _, ok := err.(*SyntaxError); !ok {
		t.Errorf("error type = %T, want *SyntaxError", err)
	}
}

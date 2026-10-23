package zon

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// lex tokenizes an entire ZON document. The returned slice always ends with a
// tokenEOF. Comments and whitespace are discarded.
//
// The lexer follows Zig's own tokenizer (std.zig.tokenizer) and literal
// parsers (std.zig.number_literal, std.zig.string_literal) for the subset of
// tokens ZON uses; the conformance suite checks it against them.
func lex(src string) ([]token, error) {
	l := &lexer{src: src, line: 1, col: 1}
	// Zig skips a UTF-8 byte order mark at the very start of a source file.
	if strings.HasPrefix(src, "\xEF\xBB\xBF") {
		l.pos = 3
	}
	var toks []token
	for {
		tok, err := l.scan()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.kind == tokenEOF {
			return toks, nil
		}
	}
}

type lexer struct {
	src  string
	pos  int
	line int
	col  int
}

// atEnd reports whether the lexer has reached the end of the source string.
func (l *lexer) atEnd() bool { return l.pos >= len(l.src) }

// cur returns the current byte under the lexer's cursor, or 0 if at the end.
func (l *lexer) cur() byte {
	if l.atEnd() {
		return 0
	}
	return l.src[l.pos]
}

// at returns the byte at the specified offset from the current cursor position, or 0 if out of bounds.
func (l *lexer) at(off int) byte {
	p := l.pos + off
	if p >= len(l.src) {
		return 0
	}
	return l.src[p]
}

// advance consumes the current byte, updates position, line, and column numbers, and returns it.
func (l *lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

// errorf returns a SyntaxError with the current position and formatted message.
func (l *lexer) errorf(line, col int, format string, args ...any) error {
	return &SyntaxError{msg: fmt.Sprintf(format, args...), Line: line, Col: col}
}

// isControl reports whether c is a control character that Zig rejects inside
// string literals and character literals: everything below 0x20 except '\n'
// (which ends the literal instead), plus DEL. NUL is included.
func isControl(c byte) bool { return (c < 0x20 && c != '\n') || c == 0x7f }

// isCommentControl reports whether c is a control character Zig rejects in a
// comment or a multiline string line. Unlike isControl it excludes '\r', which
// is allowed as part of a "\r\n" line ending. Note that '\t' is rejected.
func isCommentControl(c byte) bool { return c != '\r' && isControl(c) }

// skipTrivia consumes whitespace and // line comments. Doc comments (/// and
// //!) are tokens in Zig, and no ZON production accepts them, so they are
// rejected here.
func (l *lexer) skipTrivia() error {
	for !l.atEnd() {
		c := l.cur()
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.advance()
		case c == '/' && l.at(1) == '/':
			line, col := l.line, l.col
			if l.at(2) == '!' || (l.at(2) == '/' && l.at(3) != '/') {
				return l.errorf(line, col, "doc comments are not allowed in ZON")
			}
			l.advance()
			l.advance()
			for !l.atEnd() && l.cur() != '\n' {
				if err := l.lineByte(line, col, "comment"); err != nil {
					return err
				}
				l.advance()
			}
		default:
			return nil
		}
	}
	return nil
}

// lineByte validates the current byte of a comment or multiline string line.
func (l *lexer) lineByte(line, col int, what string) error {
	c := l.cur()
	if c == '\r' && l.at(1) != '\n' {
		return l.errorf(l.line, l.col, "carriage return must be followed by a newline in a %s", what)
	}
	if isCommentControl(c) {
		return l.errorf(l.line, l.col, "invalid control character %q in %s", c, what)
	}
	return nil
}

// scan scans and returns the next token from the source input.
func (l *lexer) scan() (token, error) {
	if err := l.skipTrivia(); err != nil {
		return token{}, err
	}
	line, col := l.line, l.col
	if l.atEnd() {
		return token{kind: tokenEOF, line: line, col: col}, nil
	}
	c := l.cur()
	switch {
	case c == '.':
		l.advance()
		// In Zig '.' and '{' are separate tokens, so whitespace and comments
		// may sit between them (". {}" is an empty aggregate).
		save, sl, sc := l.pos, l.line, l.col
		if err := l.skipTrivia(); err != nil {
			return token{}, err
		}
		if l.cur() == '{' {
			l.advance()
			return token{kind: tokenDotBrace, line: line, col: col}, nil
		}
		l.pos, l.line, l.col = save, sl, sc
		return token{kind: tokenDot, line: line, col: col}, nil
	case c == '}':
		l.advance()
		return token{kind: tokenRBrace, line: line, col: col}, nil
	case c == '=':
		l.advance()
		return token{kind: tokenEquals, line: line, col: col}, nil
	case c == ',':
		l.advance()
		return token{kind: tokenComma, line: line, col: col}, nil
	case c == '-':
		l.advance()
		return token{kind: tokenMinus, line: line, col: col}, nil
	case c == '"':
		s, err := l.scanStringBody(line, col)
		if err != nil {
			return token{}, err
		}
		return token{kind: tokenString, value: s, line: line, col: col}, nil
	case c == '\\' && l.at(1) == '\\':
		return l.scanMultilineString(line, col)
	case c == '\'':
		return l.scanChar(line, col)
	case c == '@' && l.at(1) == '"':
		l.advance() // @
		s, err := l.scanStringBody(line, col)
		if err != nil {
			return token{}, err
		}
		if s == "" {
			return token{}, l.errorf(line, col, "identifier cannot be empty")
		}
		if strings.IndexByte(s, 0) >= 0 {
			return token{}, l.errorf(line, col, "identifier cannot contain null bytes")
		}
		return token{kind: tokenIdent, value: s, quoted: true, line: line, col: col}, nil
	case isDigit(c):
		return l.scanNumber(line, col)
	case isIdentStart(c):
		start := l.pos
		for !l.atEnd() && isIdentPart(l.cur()) {
			l.advance()
		}
		name := l.src[start:l.pos]
		if isKeyword(name) {
			return token{}, l.errorf(line, col, "keyword %q cannot be used as an identifier; write @%q", name, name)
		}
		return token{kind: tokenIdent, value: name, line: line, col: col}, nil
	default:
		return token{}, l.errorf(line, col, "unexpected character %q", string(rune(c)))
	}
}

// scanStringBody reads a double-quoted string starting at the current quote and
// returns the decoded contents. Raw bytes are kept as is (they need not be
// valid UTF-8), but control characters must be escaped.
func (l *lexer) scanStringBody(line, col int) (string, error) {
	l.advance() // opening "
	var b strings.Builder
	for {
		if l.atEnd() || l.cur() == '\n' {
			return "", l.errorf(line, col, "unterminated string literal")
		}
		c := l.cur()
		if isControl(c) {
			return "", l.errorf(l.line, l.col, "invalid control character %q in string literal", c)
		}
		if c == '"' {
			l.advance()
			return b.String(), nil
		}
		if c == '\\' {
			// In a string, \xNN is a raw byte escape (it may produce bytes that
			// are not valid UTF-8). Only \u{...} introduces a codepoint. This
			// differs from a character literal, where \xNN is an integer
			// codepoint, so it is handled here rather than in scanEscape.
			if l.at(1) == 'x' {
				l.advance() // backslash
				l.advance() // x
				v, err := l.readHex(2)
				if err != nil {
					return "", err
				}
				b.WriteByte(byte(v))
				continue
			}
			r, err := l.scanEscape(line, col)
			if err != nil {
				return "", err
			}
			// A \u{...} escape is UTF-8 encoded, which rules out surrogate
			// halves; Zig rejects them rather than substituting U+FFFD.
			if !utf8.ValidRune(r) {
				return "", l.errorf(line, col, "unicode escape does not correspond to a valid unicode scalar value")
			}
			b.WriteRune(r)
			continue
		}
		b.WriteByte(c)
		l.advance()
	}
}

// scanEscape consumes a backslash escape sequence and returns its rune value.
func (l *lexer) scanEscape(line, col int) (rune, error) {
	l.advance() // backslash
	if l.atEnd() || l.cur() == '\n' {
		return 0, l.errorf(line, col, "unterminated escape sequence")
	}
	if isControl(l.cur()) {
		return 0, l.errorf(l.line, l.col, "invalid control character %q in escape sequence", l.cur())
	}
	e := l.advance()
	switch e {
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case '\\':
		return '\\', nil
	case '"':
		return '"', nil
	case '\'':
		return '\'', nil
	case 'x':
		return l.readHex(2)
	case 'u':
		return l.readUnicodeEscape()
	default:
		return 0, l.errorf(l.line, l.col, "invalid escape sequence \\%s", string(rune(e)))
	}
}

// readHex reads n hexadecimal digits and returns their combined numeric rune value.
func (l *lexer) readHex(n int) (rune, error) {
	var v rune
	for range n {
		if l.atEnd() {
			return 0, l.errorf(l.line, l.col, "incomplete hex escape")
		}
		d := hexVal(l.advance())
		if d < 0 {
			return 0, l.errorf(l.line, l.col, "invalid hex digit")
		}
		v = v*16 + rune(d)
	}
	return v, nil
}

// readUnicodeEscape reads the \u{...} body (the leading \u has been consumed).
// Any number of hex digits is allowed as long as the value never exceeds
// U+10FFFF, matching std.zig.string_literal.parseEscapeSequence.
func (l *lexer) readUnicodeEscape() (rune, error) {
	if l.cur() != '{' {
		return 0, l.errorf(l.line, l.col, "expected { after \\u")
	}
	l.advance()
	var v rune
	count := 0
	for !l.atEnd() && l.cur() != '}' {
		d := hexVal(l.advance())
		if d < 0 {
			return 0, l.errorf(l.line, l.col, "invalid hex digit in \\u escape")
		}
		v = v*16 + rune(d)
		if v > utf8.MaxRune {
			return 0, l.errorf(l.line, l.col, "unicode escape does not correspond to a valid unicode scalar value")
		}
		count++
	}
	if l.atEnd() {
		return 0, l.errorf(l.line, l.col, "unterminated \\u escape")
	}
	l.advance() // }
	if count == 0 {
		return 0, l.errorf(l.line, l.col, "empty \\u escape")
	}
	return v, nil
}

// scanChar reads a 'c' character literal and returns it as a tokenChar whose
// value is the decimal codepoint. The literal holds exactly one escape
// sequence or one UTF-8 encoded codepoint.
func (l *lexer) scanChar(line, col int) (token, error) {
	l.advance() // opening '
	if l.atEnd() || l.cur() == '\n' {
		return token{}, l.errorf(line, col, "unterminated character literal")
	}
	if l.cur() == '\'' {
		return token{}, l.errorf(line, col, "empty character literal")
	}
	if isControl(l.cur()) {
		return token{}, l.errorf(l.line, l.col, "invalid control character %q in character literal", l.cur())
	}
	var r rune
	if l.cur() == '\\' {
		e, err := l.scanEscape(line, col)
		if err != nil {
			return token{}, err
		}
		r = e
	} else {
		rr, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if rr == utf8.RuneError && size <= 1 {
			return token{}, l.errorf(line, col, "invalid UTF-8 in character literal")
		}
		r = rr
		for range size {
			l.advance()
		}
	}
	if l.atEnd() || l.cur() != '\'' {
		return token{}, l.errorf(line, col, "character literal must contain exactly one character")
	}
	l.advance() // closing '
	return token{kind: tokenChar, value: strconv.Itoa(int(r)), line: line, col: col}, nil
}

// scanMultilineString reads one or more \\ lines and joins their contents with
// newlines. In Zig each line is its own token and consecutive line tokens form
// one literal, so lines separated only by whitespace (blank lines included)
// and comments continue the same string. A "\r\n" ending is not content.
func (l *lexer) scanMultilineString(line, col int) (token, error) {
	var b strings.Builder
	first := true
	for {
		l.advance() // first backslash
		l.advance() // second backslash
		if !first {
			b.WriteByte('\n')
		}
		first = false
		for !l.atEnd() && l.cur() != '\n' {
			if err := l.lineByte(line, col, "multiline string"); err != nil {
				return token{}, err
			}
			if l.cur() == '\r' { // followed by '\n', checked by lineByte
				l.advance()
				continue
			}
			b.WriteByte(l.advance())
		}
		save, sl, sc := l.pos, l.line, l.col
		if err := l.skipTrivia(); err != nil {
			return token{}, err
		}
		if l.cur() == '\\' && l.at(1) == '\\' {
			continue
		}
		l.pos, l.line, l.col = save, sl, sc
		return token{kind: tokenString, value: b.String(), line: line, col: col}, nil
	}
}

// scanNumber reads a number literal the way Zig's tokenizer does: greedily
// over digits, letters, underscores, at most one '.' that is followed by a
// digit or letter, and a sign directly after an exponent letter. The token is
// then validated by validateNumber, so malformed text such as 1__0 or 0x is
// reported as one bad literal rather than split into several tokens.
func (l *lexer) scanNumber(line, col int) (token, error) {
	start := l.pos
	l.advance() // first digit
	isFloat := false
	for !l.atEnd() {
		c := l.cur()
		switch {
		case c == 'e' || c == 'E' || c == 'p' || c == 'P':
			l.advance()
			if l.cur() == '+' || l.cur() == '-' {
				l.advance()
				isFloat = true
			}
		case c == '_' || isDigit(c) || isLetter(c):
			l.advance()
		case c == '.' && !isFloat && (l.at(1) == '_' || isDigit(l.at(1)) || isLetter(l.at(1))):
			l.advance()
			isFloat = true
		default:
			return l.numberToken(l.src[start:l.pos], line, col)
		}
	}
	return l.numberToken(l.src[start:l.pos], line, col)
}

// numberToken validates the raw text of a number literal and wraps it in a token.
func (l *lexer) numberToken(raw string, line, col int) (token, error) {
	isFloat, msg := validateNumber(raw)
	if msg != "" {
		return token{}, l.errorf(line, col, "invalid number literal %q: %s", raw, msg)
	}
	kind := tokenInt
	if isFloat {
		kind = tokenFloat
	}
	return token{kind: kind, value: raw, line: line, col: col}, nil
}

// validateNumber is a port of std.zig.number_literal.parseNumberLiteral. It
// reports whether raw is a float literal, or a non-empty error message if raw
// is not a valid Zig number literal.
func validateNumber(raw string) (isFloat bool, msg string) {
	base := 10
	i := 0
	if len(raw) >= 2 && raw[0] == '0' {
		switch raw[1] {
		case 'b':
			base, i = 2, 2
		case 'o':
			base, i = 8, 2
		case 'x':
			base, i = 16, 2
		case 'B', 'O', 'X':
			return false, "base prefix must be lowercase"
		case '.', 'e', 'E':
		default:
			return false, "leading zeros are not allowed"
		}
	}
	if len(raw) == 2 && base != 10 {
		return false, "expected a digit after base prefix"
	}
	var underscore, period, exponent, float bool
	var special byte
	for ; i < len(raw); i++ {
		c := raw[i]
		switch c {
		case '_':
			if (i == 2 && base != 10) || special != 0 {
				return false, "expected digit before digit separator"
			}
			if underscore {
				return false, "repeated digit separator"
			}
			underscore = true
			continue
		case 'e', 'E':
			if base == 10 {
				float = true
				if exponent {
					return false, "duplicate exponent"
				}
				if underscore {
					return false, "expected digit before exponent"
				}
				special = c
				exponent = true
				continue
			}
		case 'p', 'P':
			if base == 16 {
				if i == 2 {
					return false, "expected a digit after base prefix"
				}
				float = true
				if exponent {
					return false, "duplicate exponent"
				}
				if underscore {
					return false, "expected digit before exponent"
				}
				special = c
				exponent = true
				continue
			}
		case '.':
			if exponent {
				return false, "unexpected period after exponent"
			}
			float = true
			if base != 10 && base != 16 {
				return false, "invalid base for float literal"
			}
			if period {
				return false, "duplicate period"
			}
			period = true
			if underscore {
				return false, "expected digit before '.'"
			}
			special = c
			continue
		case '+', '-':
			switch special {
			case 'p', 'P':
			case 'e', 'E':
				if base != 10 {
					return false, fmt.Sprintf("sign '%c' cannot follow digit '%c' in hex base", c, raw[i-1])
				}
			default:
				return false, fmt.Sprintf("invalid sign '%c'", c)
			}
			special = c
			continue
		}
		var digit int
		switch {
		case isDigit(c):
			digit = int(c - '0')
		case c >= 'a' && c <= 'z':
			digit = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			digit = int(c-'A') + 10
		default:
			return false, fmt.Sprintf("invalid character %q", c)
		}
		if digit >= base {
			return false, fmt.Sprintf("invalid digit '%c' for base %d", c, base)
		}
		if exponent && digit >= 10 {
			return false, fmt.Sprintf("invalid digit '%c' in exponent", c)
		}
		underscore = false
		special = 0
	}
	if underscore {
		return false, "trailing digit separator"
	}
	if special != 0 {
		return false, fmt.Sprintf("expected digit after '%c'", raw[len(raw)-1])
	}
	return float, ""
}

// keywords are Zig's reserved words (std.zig.Token.keywords). A bare
// identifier may not be one of them; @"..." must be used instead. Note that
// true, false, null, inf, nan, type names such as u8, and undefined are not
// keywords.
var keywords = map[string]bool{
	"addrspace": true, "align": true, "allowzero": true, "and": true,
	"anyframe": true, "anytype": true, "asm": true, "break": true,
	"callconv": true, "catch": true, "comptime": true, "const": true,
	"continue": true, "defer": true, "else": true, "enum": true,
	"errdefer": true, "error": true, "export": true, "extern": true,
	"fn": true, "for": true, "if": true, "inline": true, "noalias": true,
	"noinline": true, "nosuspend": true, "opaque": true, "or": true,
	"orelse": true, "packed": true, "pub": true, "resume": true,
	"return": true, "linksection": true, "struct": true, "suspend": true,
	"switch": true, "test": true, "threadlocal": true, "try": true,
	"union": true, "unreachable": true, "var": true, "volatile": true,
	"while": true,
}

// isKeyword reports whether s is a Zig keyword.
func isKeyword(s string) bool { return keywords[s] }

// isDigit reports whether c is a decimal digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isLetter reports whether c is an ASCII letter.
func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// isIdentStart reports whether c is a valid starting character for a bare identifier.
func isIdentStart(c byte) bool { return c == '_' || isLetter(c) }

// isIdentPart reports whether c is a valid interior character for a bare identifier.
func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

// hexVal returns the integer value of a hexadecimal digit character, or -1 if invalid.
func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

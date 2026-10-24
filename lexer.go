package zon

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// lex tokenizes an entire ZON document. The returned slice always ends with a
// tokenEOF. Comments and whitespace are discarded.
func lex(src string) ([]token, error) {
	l := &lexer{src: src, line: 1, col: 1}
	if strings.HasPrefix(src, "\xef\xbb\xbf") {
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

// skipTrivia consumes whitespace and // line comments.
func (l *lexer) skipTrivia() error {
	for !l.atEnd() {
		c := l.cur()
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.advance()
		case c == '/' && l.at(1) == '/':
			if err := l.scanComment(); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return nil
}

// scanComment validates and consumes an ordinary line comment.
func (l *lexer) scanComment() error {
	if l.at(2) == '!' || (l.at(2) == '/' && l.at(3) != '/') {
		return l.errorf(l.line, l.col, "documentation comments are not allowed in ZON")
	}
	for !l.atEnd() && l.cur() != '\n' {
		if invalidLineByte(l.cur()) && !(l.cur() == '\r' && l.at(1) == '\n') {
			return l.errorf(l.line, l.col, "invalid byte in comment")
		}
		l.advance()
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
	case c == '.' && l.at(1) == '{':
		l.advance()
		l.advance()
		return token{kind: tokenDotBrace, line: line, col: col}, nil
	case c == '.':
		l.advance()
		return token{kind: tokenDot, line: line, col: col}, nil
	case c == '{':
		l.advance()
		return token{kind: tokenLBrace, line: line, col: col}, nil
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
		return l.scanQuotedIdentifier(line, col)
	case isDigit(c):
		return l.scanNumber(line, col)
	case isIdentStart(c):
		start := l.pos
		for !l.atEnd() && isIdentPart(l.cur()) {
			l.advance()
		}
		return token{kind: tokenIdent, value: l.src[start:l.pos], line: line, col: col}, nil
	default:
		return token{}, l.errorf(line, col, "unexpected character %q", string(rune(c)))
	}
}

func (l *lexer) scanQuotedIdentifier(line, col int) (token, error) {
	l.advance() // @
	name, err := l.scanStringBody(line, col)
	if err != nil {
		return token{}, err
	}
	if err := checkIdentifier(name); err != nil {
		return token{}, l.errorf(line, col, "%s", err)
	}
	return token{kind: tokenQuotedIdent, value: name, line: line, col: col}, nil
}

// scanStringBody reads a double-quoted string starting at the current quote and
// returns the decoded contents.
func (l *lexer) scanStringBody(line, col int) (string, error) {
	l.advance() // opening "
	var b strings.Builder
	for {
		if l.atEnd() || l.cur() == '\n' {
			return "", l.errorf(line, col, "unterminated string literal")
		}
		c := l.cur()
		if c < 0x20 || c == 0x7f {
			return "", l.errorf(l.line, l.col, "unescaped control byte in string")
		}
		if c == '"' {
			l.advance()
			return b.String(), nil
		}
		if c == '\\' {
			if err := l.scanStringEscape(&b, line, col); err != nil {
				return "", err
			}
			continue
		}
		b.WriteByte(c)
		l.advance()
	}
}

// String hex escapes are bytes; other escapes must encode Unicode scalars.
func (l *lexer) scanStringEscape(b *strings.Builder, line, col int) error {
	if l.at(1) == 'x' {
		l.advance()
		l.advance()
		value, err := l.readHex(2)
		if err != nil {
			return err
		}
		b.WriteByte(byte(value))
		return nil
	}
	r, err := l.scanEscape(line, col)
	if err != nil {
		return err
	}
	if !utf8.ValidRune(r) {
		return l.errorf(line, col, "invalid Unicode scalar value")
	}
	b.WriteRune(r)
	return nil
}

// scanEscape consumes a backslash escape sequence and returns its rune value.
func (l *lexer) scanEscape(line, col int) (rune, error) {
	l.advance() // backslash
	if l.atEnd() {
		return 0, l.errorf(line, col, "unterminated escape sequence")
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
		if v > (utf8.MaxRune-rune(d))/16 {
			return 0, l.errorf(l.line, l.col, "Unicode escape out of range")
		}
		v = v*16 + rune(d)
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

// scanChar reads a 'c' character literal and returns it as an integer token
// whose value is the decimal codepoint.
func (l *lexer) scanChar(line, col int) (token, error) {
	l.advance() // opening '
	if l.atEnd() {
		return token{}, l.errorf(line, col, "unterminated character literal")
	}
	var r rune
	if l.cur() == '\\' {
		e, err := l.scanEscape(line, col)
		if err != nil {
			return token{}, err
		}
		r = e
	} else {
		if l.cur() < 0x20 || l.cur() == 0x7f || l.cur() == '\'' {
			return token{}, l.errorf(line, col, "invalid character literal")
		}
		rr, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if rr == utf8.RuneError && size <= 1 {
			return token{}, l.errorf(line, col, "invalid character literal")
		}
		r = rr
		for range size {
			l.advance()
		}
	}
	if l.atEnd() || l.cur() != '\'' {
		return token{}, l.errorf(line, col, "unterminated character literal")
	}
	l.advance() // closing '
	return token{kind: tokenChar, value: strconv.Itoa(int(r)), line: line, col: col}, nil
}

// scanMultilineString reads one or more consecutive \\ lines and joins their
// contents with newlines.
func (l *lexer) scanMultilineString(line, col int) (token, error) {
	var b strings.Builder
	first := true
	for {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		if err := l.scanMultilineLine(&b); err != nil {
			return token{}, err
		}
		// Whitespace and ordinary comments do not split adjacent multiline tokens.
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

// scanMultilineLine consumes a single \\ line, including its line ending.
func (l *lexer) scanMultilineLine(b *strings.Builder) error {
	l.advance()
	l.advance() // \\
	for !l.atEnd() && l.cur() != '\n' {
		if l.cur() == '\r' && l.at(1) == '\n' {
			l.advance()
			break
		}
		if invalidLineByte(l.cur()) {
			return l.errorf(l.line, l.col, "invalid byte in multiline string")
		}
		b.WriteByte(l.advance())
	}
	if !l.atEnd() {
		l.advance()
	}
	return nil
}

// scanNumber reads an integer or floating-point literal and preserves its raw
// text. It recognizes hex/octal/binary prefixes, underscores, decimal and hex
// exponents. Leading zeros on decimal integers are rejected, matching Zig.
func (l *lexer) scanNumber(line, col int) (token, error) {
	start := l.pos
	for !l.atEnd() {
		c := l.cur()
		if isIdentPart(c) || c == '.' {
			l.advance()
			continue
		}
		if (c == '+' || c == '-') && l.pos > start && strings.ContainsRune("eEpP", rune(l.src[l.pos-1])) {
			l.advance()
			continue
		}
		break
	}
	raw := l.src[start:l.pos]
	isFloat, valid := numberSyntax(raw)
	if !valid {
		return token{}, l.errorf(line, col, "invalid number literal %q", raw)
	}
	kind := tokenInt
	if isFloat {
		kind = tokenFloat
	}
	return token{kind: kind, value: raw, line: line, col: col}, nil
}

// invalidLineByte identifies bytes forbidden verbatim in comments and multiline strings.
func invalidLineByte(c byte) bool { return c < 0x20 && c != '\n' || c == 0x7f }

// isDigit reports whether c is a decimal digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isIdentStart reports whether c is a valid starting character for a bare identifier.
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

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

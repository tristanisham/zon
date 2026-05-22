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

func (l *lexer) atEnd() bool { return l.pos >= len(l.src) }

func (l *lexer) cur() byte {
	if l.atEnd() {
		return 0
	}
	return l.src[l.pos]
}

func (l *lexer) at(off int) byte {
	p := l.pos + off
	if p >= len(l.src) {
		return 0
	}
	return l.src[p]
}

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

func (l *lexer) errorf(line, col int, format string, args ...any) error {
	return &SyntaxError{msg: fmt.Sprintf(format, args...), Line: line, Col: col}
}

// skipTrivia consumes whitespace and // line comments.
func (l *lexer) skipTrivia() {
	for !l.atEnd() {
		c := l.cur()
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.advance()
		case c == '/' && l.at(1) == '/':
			for !l.atEnd() && l.cur() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

func (l *lexer) scan() (token, error) {
	l.skipTrivia()
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
		return token{kind: tokenIdent, value: s, line: line, col: col}, nil
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
		if c == '"' {
			l.advance()
			return b.String(), nil
		}
		if c == '\\' {
			r, err := l.scanEscape(line, col)
			if err != nil {
				return "", err
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
	return token{kind: tokenInt, value: strconv.Itoa(int(r)), line: line, col: col}, nil
}

// scanMultilineString reads one or more consecutive \\ lines and joins their
// contents with newlines.
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
			b.WriteByte(l.advance())
		}
		if !l.atEnd() {
			l.advance() // consume newline
		}
		// A continuation line may begin after leading spaces/tabs, but a blank
		// line ends the literal.
		save, sl, sc := l.pos, l.line, l.col
		for l.cur() == ' ' || l.cur() == '\t' || l.cur() == '\r' {
			l.advance()
		}
		if l.cur() == '\\' && l.at(1) == '\\' {
			continue
		}
		l.pos, l.line, l.col = save, sl, sc
		return token{kind: tokenString, value: b.String(), line: line, col: col}, nil
	}
}

// scanNumber reads an integer or floating-point literal and preserves its raw
// text. It recognizes hex/octal/binary prefixes, underscores, decimal and hex
// exponents. Leading zeros on decimal integers are rejected, matching Zig.
func (l *lexer) scanNumber(line, col int) (token, error) {
	start := l.pos
	isFloat := false
	switch {
	case l.cur() == '0' && (l.at(1) == 'x' || l.at(1) == 'X'):
		l.advance()
		l.advance()
		l.consumeWhile(isHexOrUnderscore)
		if l.cur() == '.' {
			isFloat = true
			l.advance()
			l.consumeWhile(isHexOrUnderscore)
		}
		if l.cur() == 'p' || l.cur() == 'P' {
			isFloat = true
			l.advance()
			if l.cur() == '+' || l.cur() == '-' {
				l.advance()
			}
			l.consumeWhile(isDigitOrUnderscore)
		}
	case l.cur() == '0' && (l.at(1) == 'o' || l.at(1) == 'O'):
		l.advance()
		l.advance()
		l.consumeWhile(isDigitOrUnderscore)
	case l.cur() == '0' && (l.at(1) == 'b' || l.at(1) == 'B'):
		l.advance()
		l.advance()
		l.consumeWhile(isDigitOrUnderscore)
	default:
		l.consumeWhile(isDigitOrUnderscore)
		if l.cur() == '.' && isDigit(l.at(1)) {
			isFloat = true
			l.advance()
			l.consumeWhile(isDigitOrUnderscore)
		}
		if l.cur() == 'e' || l.cur() == 'E' {
			isFloat = true
			l.advance()
			if l.cur() == '+' || l.cur() == '-' {
				l.advance()
			}
			l.consumeWhile(isDigitOrUnderscore)
		}
	}
	raw := l.src[start:l.pos]
	kind := tokenInt
	if isFloat {
		kind = tokenFloat
	} else if len(raw) > 1 && raw[0] == '0' && (isDigit(raw[1]) || raw[1] == '_') {
		// Decimal integer with a leading zero. Zig rejects these; octal must be
		// written 0o.... The raw[1] guard excludes the 0x/0o/0b prefixes.
		return token{}, l.errorf(line, col, "leading zeros are not allowed in integer literals")
	}
	return token{kind: kind, value: raw, line: line, col: col}, nil
}

func (l *lexer) consumeWhile(pred func(byte) bool) {
	for !l.atEnd() && pred(l.cur()) {
		l.advance()
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

func isDigitOrUnderscore(c byte) bool { return isDigit(c) || c == '_' }

func isHexOrUnderscore(c byte) bool { return hexVal(c) >= 0 || c == '_' }

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

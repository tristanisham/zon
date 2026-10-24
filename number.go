package zon

import "strings"

// numberSyntax validates Zig's literal spelling without imposing a Go numeric
// range. Conversion and destination overflow checks happen during decoding.
func numberSyntax(raw string) (isFloat, valid bool) {
	base, pos := 10, 0
	if len(raw) > 1 && raw[0] == '0' {
		switch raw[1] {
		case 'x':
			base, pos = 16, 2
		case 'o':
			base, pos = 8, 2
		case 'b':
			base, pos = 2, 2
		case '.', 'e', 'E':
		default:
			return false, false
		}
	}
	badSeparator := false
	digits := func(radix int) bool {
		start := pos
		for pos < len(raw) && hexVal(raw[pos]) >= 0 && hexVal(raw[pos]) < radix {
			pos++
			if pos < len(raw) && raw[pos] == '_' {
				pos++
				if pos == len(raw) || hexVal(raw[pos]) < 0 || hexVal(raw[pos]) >= radix {
					badSeparator = true
					return false
				}
			}
		}
		return pos > start
	}
	integerDigits := digits(base)
	if badSeparator {
		return false, false
	}
	if (base == 10 || base == 16) && pos < len(raw) && raw[pos] == '.' {
		isFloat = true
		pos++
		fractionDigits := digits(base)
		if badSeparator || (!fractionDigits && !integerDigits) || (!fractionDigits && pos == len(raw)) {
			return false, false
		}
	} else if !integerDigits {
		return false, false
	}
	if pos < len(raw) && ((base == 10 && strings.ContainsRune("eE", rune(raw[pos]))) || (base == 16 && strings.ContainsRune("pP", rune(raw[pos])))) {
		isFloat = true
		pos++
		if pos < len(raw) && (raw[pos] == '+' || raw[pos] == '-') {
			pos++
		}
		if !digits(10) {
			return false, false
		}
	}
	return isFloat, pos == len(raw)
}

package zon

import (
	"flag"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

var (
	diffN    = flag.Int("zon.diff.n", 20000, "documents TestDifferentialFuzz generates")
	diffSeed = flag.Uint64("zon.diff.seed", 1, "seed for TestDifferentialFuzz")
)

// TestDifferentialFuzz generates documents near the edges of the grammar (a
// random ZON generator whose output is then mutated at the token and byte
// level) and checks that the library and Zig's reference front end agree on
// every one: same accept/reject verdict, same value. It needs the oracle:
//
//	go test -run TestDifferentialFuzz -zon.oracle=./oracle -zon.diff.n=500000
func TestDifferentialFuzz(t *testing.T) {
	if *oracleFlag == "" {
		t.Skip("needs -zon.oracle (see TestConformance)")
	}
	g := &docGen{r: rand.New(rand.NewPCG(*diffSeed, 0x5a4f4e))}
	inputs := make([]string, *diffN)
	for i := range inputs {
		inputs[i] = g.doc()
	}
	verdicts := runOracle(t, *oracleFlag, inputs)
	var ok, rejected, crashed, direct, failed int
	for i, z := range verdicts {
		if strings.HasPrefix(z, "CRASH") {
			crashed++
			t.Logf("oracle crashed on %q: %s (library says %s)", inputs[i], z, canonical(inputs[i]))
			continue
		}
		got := canonical(inputs[i])
		match, exact := agree(got, z)
		if match && !exact {
			direct++
			t.Logf("%q: Zig's f128 float parse differs from its direct f64 parse; the library matches the latter:\n  zig: %s\n  go:  %s", inputs[i], z, got)
			continue
		}
		if !match {
			failed++
			if failed <= 50 {
				t.Errorf("%q:\n  zig: %s\n  go:  %s", inputs[i], z, got)
			}
			continue
		}
		if strings.HasPrefix(got, "OK") {
			ok++
		} else {
			rejected++
		}
	}
	t.Logf("%d documents: %d accepted and equal, %d rejected by both, %d equal only to Zig's direct f64 parse, %d oracle crashes, %d mismatches",
		len(inputs), ok, rejected, direct, crashed, failed)
}

type docGen struct{ r *rand.Rand }

func (g *docGen) pick(xs ...string) string { return xs[g.r.IntN(len(xs))] }

// doc returns a generated value, mutated about half of the time.
func (g *docGen) doc() string {
	s := g.trivia() + g.value(0) + g.trivia()
	for g.r.IntN(2) == 0 {
		s = g.mutate(s)
	}
	return s
}

func (g *docGen) trivia() string {
	switch g.r.IntN(12) {
	case 0:
		return "\n"
	case 1:
		return " // " + g.pick("c", "", "/x", "\t", "é", "\xff", "\r") + "\n"
	case 2:
		return g.pick("\r\n", "\t", "  ", "\r", "////\n", "///\n", "//!\n")
	}
	return g.pick("", " ")
}

func (g *docGen) value(depth int) string {
	n := 10
	if depth > 4 {
		n = 8 // no more aggregates
	}
	switch g.r.IntN(n) {
	case 0:
		return g.pick("true", "false", "null", "inf", "-inf", "nan", "-nan", "@\"true\"", "undefined")
	case 1, 2:
		return g.number()
	case 3:
		return g.str()
	case 4:
		return g.multiline()
	case 5:
		return g.char()
	case 6:
		return "." + g.ident()
	case 7:
		return g.number()
	case 8:
		return g.tuple(depth)
	default:
		return g.structLit(depth)
	}
}

func (g *docGen) number() string {
	var b strings.Builder
	if g.r.IntN(4) == 0 {
		b.WriteString(g.pick("-", "- ", "-"))
	}
	base := g.pick("", "", "", "0x", "0o", "0b", "0X")
	b.WriteString(base)
	digits := "0123456789"
	switch base {
	case "0x", "0X":
		digits = "0123456789abcdefABCDEF"
	case "0o":
		digits = "01234567"
	case "0b":
		digits = "01"
	}
	nd := 1 + g.r.IntN(4)
	if g.r.IntN(8) == 0 {
		nd = 15 + g.r.IntN(30) // past 64 bits
	}
	for i := range nd {
		if i > 0 && g.r.IntN(6) == 0 {
			b.WriteByte('_')
		}
		b.WriteByte(digits[g.r.IntN(len(digits))])
	}
	if g.r.IntN(3) == 0 {
		b.WriteString(".")
		for range g.r.IntN(4) {
			b.WriteByte(digits[g.r.IntN(len(digits))])
		}
	}
	if g.r.IntN(3) == 0 {
		b.WriteString(g.pick("e", "E", "p", "P"))
		b.WriteString(g.pick("", "+", "-"))
		for range g.r.IntN(4) {
			b.WriteByte("0123456789"[g.r.IntN(10)])
		}
	}
	return b.String()
}

func (g *docGen) escape() string {
	switch g.r.IntN(6) {
	case 0:
		return g.pick(`\n`, `\r`, `\t`, `\\`, `\"`, `\'`, `\a`, `\0`, `\`)
	case 1:
		return `\x` + g.hex(g.r.IntN(3))
	case 2:
		return `\u{` + g.hex(g.r.IntN(8)) + g.pick("}", "}", "")
	}
	return g.pick("a", " ", "é", "😀", "\xff", "\xc3", "\x7f", "\t", "\x00", "'", `"`, "\r")
}

func (g *docGen) hex(n int) string {
	var b strings.Builder
	for range n {
		b.WriteByte("0123456789abcdefDF"[g.r.IntN(18)])
	}
	return b.String()
}

func (g *docGen) str() string {
	var b strings.Builder
	b.WriteByte('"')
	for range g.r.IntN(5) {
		b.WriteString(g.escape())
	}
	b.WriteByte('"')
	return b.String()
}

func (g *docGen) char() string {
	s := "'"
	for range 1 + g.r.IntN(2)*g.r.IntN(2) {
		s += g.escape()
	}
	return s + "'"
}

func (g *docGen) multiline() string {
	var b strings.Builder
	for i := range 1 + g.r.IntN(3) {
		if i > 0 {
			b.WriteString(g.pick("\n", "\n", "\r\n", "\n\n", "\n// c\n", "\n  "))
		}
		b.WriteString(`\\`)
		for range g.r.IntN(3) {
			b.WriteString(g.pick("a", " ", `\`, `"`, "\t", "\r", "\x7f", "é", "\xff", "//"))
		}
	}
	return b.String() + "\n"
}

func (g *docGen) ident() string {
	switch g.r.IntN(5) {
	case 0:
		return `@"` + g.pick("", "a b", `\x00`, "x", "fn", `\u{61}`, "\xff", `\n`) + `"`
	case 1:
		return g.pick("fn", "error", "type", "u8", "true", "null", "inf", "async", "test", "_", "and")
	}
	return g.pick("a", "b", "c", "name", "A1", "_x")
}

func (g *docGen) tuple(depth int) string {
	var b strings.Builder
	b.WriteString(".{")
	n := g.r.IntN(4)
	for i := range n {
		if i > 0 {
			b.WriteString("," + g.trivia())
		}
		b.WriteString(g.value(depth + 1))
	}
	if n > 0 && g.r.IntN(2) == 0 {
		b.WriteString(",")
	}
	b.WriteString(g.trivia() + "}")
	return b.String()
}

func (g *docGen) structLit(depth int) string {
	var b strings.Builder
	b.WriteString(".{")
	n := 1 + g.r.IntN(3)
	for i := range n {
		if i > 0 {
			b.WriteString("," + g.trivia())
		}
		b.WriteString("." + g.ident() + g.pick(" = ", "=", " =\n") + g.value(depth+1))
	}
	if g.r.IntN(2) == 0 {
		b.WriteString(",")
	}
	b.WriteString(g.trivia() + "}")
	return b.String()
}

// mutate applies one random token- or byte-level edit to s.
func (g *docGen) mutate(s string) string {
	frags := []string{
		".{", "}", ".", ",", "=", "-", " ", "\n", "\r", "\t", "//c\n", "///", "//!",
		`\\`, ".a", `.@"x"`, `@"y"`, "a", "fn", "true", "inf", "nan", "null",
		"0", "1", "0x", "0o", "0b", "_", "e", "p", "+", ".5", "1.", "'", `"`, `\`,
		`\x`, `\u{`, "\x00", "\x7f", "\xff", "é", "\r\n", "9", "f", "(", ")", "[", "]",
		";", ":", "!", "&", "*", "?", "\xef\xbb\xbf",
	}
	i := 0
	if len(s) > 0 {
		i = g.r.IntN(len(s) + 1)
	}
	switch g.r.IntN(4) {
	case 0: // insert a fragment
		return s[:i] + frags[g.r.IntN(len(frags))] + s[i:]
	case 1: // delete a short run
		j := min(len(s), i+1+g.r.IntN(3))
		return s[:i] + s[j:]
	case 2: // replace a byte
		if i < len(s) {
			return s[:i] + string([]byte{byte(g.r.IntN(256))}) + s[i+1:]
		}
		return s
	default: // duplicate a run
		j := min(len(s), i+1+g.r.IntN(8))
		return s[:j] + s[i:j] + s[j:]
	}
}

// TestDifferentialEncoder marshals random Go values (odd field names and
// enum literals, arbitrary bytes in strings, floats from random bit patterns)
// and checks that Zig accepts every document and reads the same value the
// library reads back.
func TestDifferentialEncoder(t *testing.T) {
	if *oracleFlag == "" {
		t.Skip("needs -zon.oracle (see TestConformance)")
	}
	r := rand.New(rand.NewPCG(*diffSeed, 0x656e63))
	names := []string{"a", "fn", "error", "true", "inf", "u8", "_", "a b", "1", "é", "\xff", "\x7f", "\t", `"`, `\`, "type", "async"}
	randString := func() string {
		b := make([]byte, r.IntN(6))
		for i := range b {
			b[i] = byte(r.IntN(256))
		}
		return string(b)
	}
	var value func(depth int) any
	value = func(depth int) any {
		switch r.IntN(9) {
		case 0:
			return r.IntN(2) == 0
		case 1:
			return int64(r.Uint64())
		case 2:
			return r.Uint64()
		case 3:
			return math.Float64frombits(r.Uint64())
		case 4:
			return math.Float32frombits(r.Uint32())
		case 5:
			return randString()
		case 6:
			return EnumLiteral(names[r.IntN(len(names))])
		}
		if depth > 3 {
			return nil
		}
		if r.IntN(2) == 0 {
			s := make([]any, r.IntN(4))
			for i := range s {
				s[i] = value(depth + 1)
			}
			return s
		}
		m := map[string]any{}
		for range r.IntN(4) {
			m[names[r.IntN(len(names))]] = value(depth + 1)
		}
		return m
	}
	var docs []string
	for range *diffN / 10 {
		v := value(0)
		out, err := MarshalIndent(v, "", []string{"    ", ""}[r.IntN(2)])
		if err != nil {
			t.Fatalf("Marshal(%#v): %v", v, err)
		}
		docs = append(docs, string(out))
	}
	failed := 0
	for i, z := range runOracle(t, *oracleFlag, docs) {
		if got := canonical(docs[i]); got != verdict(z) {
			failed++
			if failed <= 50 {
				t.Errorf("Marshal output %q:\n  zig: %s\n  go:  %s", docs[i], z, got)
			}
		}
	}
	t.Logf("%d marshaled documents, %d mismatches", len(docs), failed)
}

package zon

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// canonical renders the library's view of a ZON document in the same textual
// form the Zig reference oracle (testdata/oracle/oracle.zig) prints, so the two
// can be compared byte for byte:
//
//	OK <value>   the document is accepted and decodes to <value>
//	ERR          the document is rejected
//
// Values: true, false, null, inf, -inf, nan, i:<decimal>, f:<float64 bits as
// 16 hex digits>, e:<hex name>, s:<hex bytes>, {} (empty), [a,b], {<hex name>=v,...}.
// Integers render with their exact value, as Zig keeps them (arbitrary
// precision); canonical also checks that decode's int64/uint64 helpers agree
// with that value whenever it fits. Floats are interpreted by decode's
// float64 helper; the oracle narrows Zig's f128 value to f64 likewise.
func canonical(src string) string {
	n, err := parse(src)
	if err != nil {
		return "ERR"
	}
	var b strings.Builder
	if err := canonicalNode(&b, n); err != nil {
		return "ERR"
	}
	return "OK " + b.String()
}

func canonicalNode(b *strings.Builder, n node) error {
	switch nd := n.(type) {
	case nullNode:
		b.WriteString("null")
	case boolNode:
		fmt.Fprint(b, nd.value)
	case stringNode:
		b.WriteString("s:" + hex.EncodeToString([]byte(nd.value)))
	case enumNode:
		b.WriteString("e:" + hex.EncodeToString([]byte(nd.name)))
	case numberNode:
		if !nd.isFloat {
			// The literal's exact value, which may exceed 64 bits as in Zig.
			v, ok := new(big.Int).SetString(strings.ReplaceAll(nd.raw, "_", ""), 0)
			if !ok {
				return fmt.Errorf("unparseable integer %q", nd.raw)
			}
			// The helpers decode uses must agree with it whenever it fits.
			if i, err := parseInt(nd.raw); err == nil {
				if !v.IsInt64() || v.Int64() != i {
					return fmt.Errorf("parseInt(%q) = %d, want %v", nd.raw, i, v)
				}
			} else if u, err := parseUint(nd.raw); err == nil {
				if !v.IsUint64() || v.Uint64() != u {
					return fmt.Errorf("parseUint(%q) = %d, want %v", nd.raw, u, v)
				}
			} else if v.IsInt64() || v.IsUint64() {
				return fmt.Errorf("%q fits in 64 bits but parseInt/parseUint fail", nd.raw)
			}
			b.WriteString("i:" + v.String())
			return nil
		}
		switch nd.raw {
		case "inf", "-inf", "nan":
			b.WriteString(nd.raw)
			return nil
		}
		f, err := parseFloat(nd.raw)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "f:%016x", math.Float64bits(f))
	case tupleNode:
		if len(nd.items) == 0 {
			b.WriteString("{}")
			return nil
		}
		b.WriteByte('[')
		for i, it := range nd.items {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := canonicalNode(b, it); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case structNode:
		b.WriteByte('{')
		for i, f := range nd.fields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(hex.EncodeToString([]byte(f.name)) + "=")
			if err := canonicalNode(b, f.value); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	}
	return nil
}

// The conformance suite compares the library against Zig's own ZON front end
// (std.zig.Ast in .zon mode + std.zig.ZonGen) on conformanceCases. The Zig
// verdicts live in testdata/conformance.golden so the suite runs without a
// Zig toolchain. To regenerate them, build the oracle and pass it in:
//
//	zig build-exe -OReleaseSafe testdata/oracle/oracle.zig
//	go test -run TestConformance -zon.oracle=./oracle -zon.update
var (
	oracleFlag = flag.String("zon.oracle", "", "path to the Zig ZON oracle binary (testdata/oracle/oracle.zig)")
	updateFlag = flag.Bool("zon.update", false, "rewrite testdata/conformance.golden from -zon.oracle")
)

const goldenPath = "testdata/conformance.golden"

// runOracle feeds inputs to the Zig oracle in batches and returns its verdict
// line for each. Some std.zig code paths panic on malformed input; the case
// that crashed the oracle gets the verdict "CRASH <first stderr line>" and the
// remaining cases are fed to a fresh oracle process.
func runOracle(t testing.TB, oracle string, inputs []string) []string {
	t.Helper()
	dir := t.TempDir()
	var verdicts []string
	for len(verdicts) < len(inputs) {
		rest := inputs[len(verdicts):]
		var buf bytes.Buffer
		for _, in := range rest {
			var n [4]byte
			binary.LittleEndian.PutUint32(n[:], uint32(len(in)))
			buf.Write(n[:])
			buf.WriteString(in)
		}
		f := filepath.Join(dir, "cases.bin")
		if err := os.WriteFile(f, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(oracle, f)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if s := strings.TrimSuffix(string(out), "\n"); s != "" {
			verdicts = append(verdicts, strings.Split(s, "\n")...)
		}
		if err == nil {
			break
		}
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("oracle: %v", err)
		}
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		verdicts = append(verdicts, "CRASH "+msg)
	}
	if len(verdicts) != len(inputs) {
		t.Fatalf("oracle returned %d verdicts for %d inputs", len(verdicts), len(inputs))
	}
	return verdicts
}

// floatAlt matches the oracle's "f:<f128 path>|<direct f64>" float form.
var floatAlt = regexp.MustCompile(`f:([0-9a-f]{16})\|([0-9a-f]{16})`)

// verdict reduces an oracle line ("OK <v>" or "ERR <message>") to the form
// canonical produces, taking for each float the value Zig's ZON front end
// produces (parsed as f128, then narrowed to f64).
func verdict(line string) string {
	if strings.HasPrefix(line, "ERR") {
		return "ERR"
	}
	return floatAlt.ReplaceAllString(line, "f:$1")
}

// verdictDirectF64 is verdict, but taking for each float the literal parsed
// directly as f64. The two differ through double rounding or through bugs in
// Zig's f128 float parser; the library rounds once, directly to float64.
func verdictDirectF64(line string) string {
	if strings.HasPrefix(line, "ERR") {
		return "ERR"
	}
	return floatAlt.ReplaceAllString(line, "f:$2")
}

// agree compares the library's canonical form with an oracle line. exact is
// false when they only agree on the directly parsed f64 value of a float.
func agree(got, zig string) (ok, exact bool) {
	if got == verdict(zig) {
		return true, true
	}
	return got == verdictDirectF64(zig), false
}

type goldenCase struct {
	area, input, zig string
}

func readGolden(t *testing.T) []goldenCase {
	t.Helper()
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%v (regenerate with -zon.oracle and -zon.update)", err)
	}
	var out []goldenCase
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("%s:%d: want 3 tab-separated fields", goldenPath, i+1)
		}
		var c goldenCase
		for j, p := range []*string{&c.area, &c.input, &c.zig} {
			s, err := strconv.Unquote(parts[j])
			if err != nil {
				t.Fatalf("%s:%d: field %d: %v", goldenPath, i+1, j+1, err)
			}
			*p = s
		}
		out = append(out, c)
	}
	return out
}

func writeGolden(t *testing.T, cases []goldenCase) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Generated by `go test -run TestConformance -zon.oracle=... -zon.update`. Do not edit.\n")
	b.WriteString("# Columns: area, input, verdict of Zig's std.zig.Ast(.zon) + std.zig.ZonGen.\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", strconv.Quote(c.area), strconv.Quote(c.input), strconv.Quote(c.zig))
	}
	if err := os.WriteFile(goldenPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestConformance checks that the library accepts exactly the documents Zig
// accepts and decodes each to the same value.
func TestConformance(t *testing.T) {
	var cases []goldenCase
	if *oracleFlag != "" {
		var inputs []string
		for _, g := range conformanceCases {
			for _, in := range g.cases {
				cases = append(cases, goldenCase{area: g.area, input: in})
				inputs = append(inputs, in)
			}
		}
		for i, v := range runOracle(t, *oracleFlag, inputs) {
			cases[i].zig = v
		}
		if *updateFlag {
			writeGolden(t, cases)
		}
	} else {
		cases = readGolden(t)
		if n := countCases(); n != len(cases) {
			t.Fatalf("golden has %d cases, corpus has %d: regenerate with -zon.oracle and -zon.update", len(cases), n)
		}
	}

	for _, c := range cases {
		if strings.HasPrefix(c.zig, "CRASH") {
			t.Logf("[%s] %q: no reference verdict, the Zig oracle crashed: %s (library says %s)", c.area, c.input, c.zig, canonical(c.input))
			continue
		}
		got := canonical(c.input)
		ok, exact := agree(got, c.zig)
		if !ok {
			t.Errorf("[%s] %q:\n  zig: %s\n  go:  %s", c.area, c.input, c.zig, got)
		} else if !exact {
			t.Logf("[%s] %q: Zig's f128 float parse differs from its direct f64 parse; the library matches the latter:\n  zig: %s\n  go:  %s", c.area, c.input, c.zig, got)
		}
	}
}

func countCases() int {
	n := 0
	for _, g := range conformanceCases {
		n += len(g.cases)
	}
	return n
}

// TestConformanceEncoder checks that Marshal only produces documents Zig
// accepts, and that they decode back to the value that was encoded. Without
// -zon.oracle the library's parser stands in for Zig (TestConformance and
// TestDifferentialFuzz hold it to Zig's verdicts).
func TestConformanceEncoder(t *testing.T) {
	type dupTags struct {
		A int `zon:"x"`
		B int `zon:"x"`
	}
	valid := []any{
		map[string]int{"fn": 1, "error": 2, "true": 3, "u8": 4, "a b": 5, "1": 6, "é": 7},
		struct {
			Fn  int `zon:"fn"`
			Ok  int `zon:"type"`
			Nul int `zon:"null"`
		}{1, 2, 3},
		[]EnumLiteral{"fn", "while", "true", "inf", "a-b", "_", "x\xff"},
		[]string{"\x7f", "\x00", "\t", "\r\n", "\xff\xfe", "é😀", "\u2028", `\"'`},
		[]float64{0, -0.0, 1, -1.5, 1e300, 5e-324, math.MaxFloat64, math.SmallestNonzeroFloat64},
		[]float32{1.5, math.MaxFloat32, 1e-45},
		[]int64{0, -1, math.MinInt64, math.MaxInt64},
		[]uint64{math.MaxUint64},
	}
	invalid := []any{
		map[string]int{"": 1},
		map[string]int{"a\x00b": 1},
		EnumLiteral(""),
		EnumLiteral("a\x00"),
		struct {
			E int `zon:""`
			F int `zon:"a\x00"`
		}{},
		dupTags{},
	}

	var docs []string
	for _, v := range valid {
		for _, indent := range []string{"    ", ""} {
			out, err := MarshalIndent(v, "", indent)
			if err != nil {
				t.Errorf("Marshal(%#v): %v", v, err)
				continue
			}
			docs = append(docs, string(out))
			if got := canonical(string(out)); !strings.HasPrefix(got, "OK") {
				t.Errorf("Marshal(%#v) produced a document the parser rejects:\n%s", v, out)
				continue
			}
			p := reflect.New(reflect.TypeOf(v))
			if err := Unmarshal(out, p.Interface()); err != nil {
				t.Errorf("Unmarshal(Marshal(%#v)): %v\n%s", v, err, out)
				continue
			}
			if got := p.Elem().Interface(); !reflect.DeepEqual(got, v) && !floatsEqualBits(got, v) {
				t.Errorf("round trip of %#v = %#v\n%s", v, got, out)
			}
		}
	}
	for _, v := range invalid {
		if out, err := Marshal(v); err == nil {
			t.Errorf("Marshal(%#v) = %q, want an error: ZON cannot represent it", v, out)
		}
	}

	if *oracleFlag != "" {
		for i, z := range runOracle(t, *oracleFlag, docs) {
			if want := canonical(docs[i]); verdict(z) != want {
				// Exact agreement is required here: Marshal's own floats
				// never use digit separators.
				t.Errorf("Marshal output %q:\n  zig: %s\n  go:  %s", docs[i], z, want)
			}
		}
	}
}

// floatsEqualBits compares float slices bit for bit, so that -0.0 round trips
// are told apart from 0.0 and DeepEqual's NaN rule does not get in the way.
func floatsEqualBits(a, b any) bool {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.Kind() != reflect.Slice || av.Len() != bv.Len() {
		return false
	}
	for i := range av.Len() {
		x, y := av.Index(i), bv.Index(i)
		if x.Kind() != reflect.Float32 && x.Kind() != reflect.Float64 {
			return false
		}
		if math.Float64bits(x.Float()) != math.Float64bits(y.Float()) {
			return false
		}
	}
	return true
}

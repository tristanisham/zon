package zon

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

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

// Preserved from the earlier Delta work. Revalidate with ZON_TEST_ZIG set;
// the checked-in verdicts also run without a Zig installation.
type referenceCase struct{ area, input, verdict string }

func referenceCases(t *testing.T) []referenceCase {
	t.Helper()
	data, err := os.ReadFile("testdata/zig-conformance.tsv")
	if err != nil {
		t.Fatal(err)
	}
	var cases []referenceCase
	for i, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("fixture line %d", i+1)
		}
		var c referenceCase
		for j, field := range []*string{&c.area, &c.input, &c.verdict} {
			*field, err = strconv.Unquote(parts[j])
			if err != nil {
				t.Fatal(err)
			}
		}
		cases = append(cases, c)
	}
	return cases
}

var referenceFloat = regexp.MustCompile(`f:([0-9a-f]{16})\|([0-9a-f]{16})`)

func referenceVerdict(line string, direct bool) string {
	if strings.HasPrefix(line, "ERR") {
		return "ERR"
	}
	if direct {
		return referenceFloat.ReplaceAllString(line, "f:$2")
	}
	return referenceFloat.ReplaceAllString(line, "f:$1")
}

func TestReferenceCorpus(t *testing.T) {
	cases := referenceCases(t)
	if zig := os.Getenv("ZON_TEST_ZIG"); zig != "" {
		revalidateReference(t, zig, cases)
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("%s/%d", c.area, i), func(t *testing.T) {
			if strings.HasPrefix(c.verdict, "CRASH") {
				t.Skip("reference compiler panics on this malformed input; covered by Go fuzzing")
			}
			got := canonical(c.input)
			if got != referenceVerdict(c.verdict, false) && got != referenceVerdict(c.verdict, true) {
				t.Errorf("%q: got %s; Zig %s", c.input, got, c.verdict)
			}
		})
	}
}

func revalidateReference(t *testing.T, zig string, cases []referenceCase) {
	t.Helper()
	dir := t.TempDir()
	oracle := filepath.Join(dir, "oracle")
	cmd := exec.Command(zig, "build-exe", "testdata/oracle/oracle.zig", "-OReleaseSafe", "-femit-bin="+oracle, "--cache-dir", filepath.Join(dir, "cache"), "--global-cache-dir", filepath.Join(os.TempDir(), "zon-zig-cache"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build reference oracle: %v\n%s", err, out)
	}
	var records bytes.Buffer
	var known []referenceCase
	for _, c := range cases {
		if strings.HasPrefix(c.verdict, "CRASH") {
			continue
		}
		if err := binary.Write(&records, binary.LittleEndian, uint32(len(c.input))); err != nil {
			t.Fatal(err)
		}
		records.WriteString(c.input)
		known = append(known, c)
	}
	path := filepath.Join(dir, "inputs.bin")
	if err := os.WriteFile(path, records.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(oracle, path).Output()
	if err != nil {
		t.Fatalf("reference oracle: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(known) {
		t.Fatalf("oracle returned %d results for %d cases", len(lines), len(known))
	}
	for i, line := range lines {
		c := known[i]
		if referenceVerdict(line, false) != referenceVerdict(c.verdict, false) || referenceVerdict(line, true) != referenceVerdict(c.verdict, true) {
			t.Errorf("Zig reference changed for %q: recorded %s; current %s", c.input, c.verdict, line)
		}
	}
	t.Logf("revalidated %d recorded verdicts against %s", len(known), zig)
}

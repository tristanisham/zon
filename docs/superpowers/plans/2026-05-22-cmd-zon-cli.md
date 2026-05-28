# cmd/zon CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `zon`, a jq-clone command-line tool that queries/transforms ZON (and JSON) input using gojq and emits ZON (default), JSON, YAML, or TOML.

**Architecture:** A single `package main` under `cmd/zon/` living in its own nested Go module so the library's `go.mod` stays zero-dependency. The pipeline is: read → decode (ZON/JSON → `any`) → normalize to gojq's value model → run the compiled gojq filter → denormalize → encode. All logic is reachable through a testable `run(args, stdin, stdout, stderr) int` entry point.

**Tech Stack:** Go 1.26, `github.com/itchyny/gojq` (jq engine), `gopkg.in/yaml.v3`, `github.com/pelletier/go-toml/v2`, and the local `github.com/tristanisham/zon` library.

**Reference spec:** `docs/superpowers/specs/2026-05-22-cmd-zon-cli-design.md`

---

## File Structure

- `cmd/zon/go.mod` — nested module manifest; isolates CLI dependencies.
- `cmd/zon/main.go` — flag parsing, the testable `run(...)` entry point, input loading, gojq compile + execution loop.
- `cmd/zon/input.go` — input format type, format auto-detection, ZON/JSON decoding.
- `cmd/zon/normalize.go` — `toQuery` (decoded value → gojq model) and `fromQuery` (gojq result → encoder-friendly value).
- `cmd/zon/output.go` — output format type, encode options, the four encoders + raw mode.
- `cmd/zon/input_test.go`, `cmd/zon/normalize_test.go`, `cmd/zon/output_test.go`, `cmd/zon/main_test.go` — tests.
- `cmd/zon/testdata/build.zig.zon` — fixture copied from the repo's top-level `testdata/`.

---

### Task 1: Scaffold the nested module

**Files:**
- Create: `cmd/zon/go.mod`
- Create: `cmd/zon/main.go` (temporary minimal version, replaced in Task 5)
- Create: `cmd/zon/testdata/build.zig.zon`

- [ ] **Step 1: Create the module manifest**

Create `cmd/zon/go.mod`:

```
module github.com/tristanisham/zon/cmd/zon

go 1.26

require (
	github.com/itchyny/gojq v0.12.16
	github.com/pelletier/go-toml/v2 v2.2.3
	github.com/tristanisham/zon v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/tristanisham/zon => ../..
```

The `replace` directive points the CLI at the local library so builds and tests work offline without a published tag. (See "Release prep" at the end of this plan — the replace must be removed and the require pinned to a real tag before `go install ...@latest` will work.)

- [ ] **Step 2: Create a minimal buildable main**

Create `cmd/zon/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("zon cli")
}
```

- [ ] **Step 3: Copy the test fixture**

Run:

```bash
mkdir -p cmd/zon/testdata
cp testdata/build.zig.zon cmd/zon/testdata/build.zig.zon
```

- [ ] **Step 4: Resolve dependencies and verify the build**

Run:

```bash
cd cmd/zon && go mod tidy && go build ./...
```

Expected: `go mod tidy` downloads gojq, go-toml/v2, yaml.v3 (and gojq's transitive `github.com/itchyny/timefmt-go`), writes `cmd/zon/go.sum`, and the build succeeds with no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/zon/go.mod cmd/zon/go.sum cmd/zon/main.go cmd/zon/testdata/build.zig.zon
git commit -m "feat(cli): scaffold cmd/zon nested module"
```

---

### Task 2: Input decoding and format detection

**Files:**
- Create: `cmd/zon/input.go`
- Test: `cmd/zon/input_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/zon/input_test.go`:

```go
package main

import (
	"reflect"
	"testing"
)

func TestDecodeZON(t *testing.T) {
	v, err := decode([]byte(`.{ .name = "x", .n = 1 }`), inZON)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map", v)
	}
	if m["name"] != "x" {
		t.Errorf("name = %v", m["name"])
	}
}

func TestDecodeJSONPreservesInt(t *testing.T) {
	v, err := decode([]byte(`{"n": 3}`), inJSON)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	m := v.(map[string]any)
	// JSON numbers are decoded with UseNumber, so n stays a json.Number "3".
	if got := m["n"].(json.Number).String(); got != "3" {
		t.Errorf("n = %q, want \"3\"", got)
	}
}

func TestDecodeAuto(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any
	}{
		{"zon-struct", `.{ .a = 1 }`, map[string]any{"a": int64(1)}},
		{"json-object", `{"a": 1}`, nil}, // checked separately below
		{"scalar-number", `42`, int64(42)},
		{"scalar-string", `"hi"`, "hi"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := decode([]byte(c.in), inAuto)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if c.want != nil && !reflect.DeepEqual(v, c.want) {
				t.Errorf("got %#v, want %#v", v, c.want)
			}
		})
	}
}
```

Add the `encoding/json` import to the test file's import block (the `json.Number` reference needs it):

```go
import (
	"encoding/json"
	"reflect"
	"testing"
)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd cmd/zon && go test ./... -run 'TestDecode' -v`
Expected: FAIL — `undefined: decode`, `undefined: inZON`, etc.

- [ ] **Step 3: Implement input.go**

Create `cmd/zon/input.go`:

```go
package main

import (
	"bytes"
	"encoding/json"

	"github.com/tristanisham/zon"
)

// inFormat selects how raw input bytes are decoded.
type inFormat int

const (
	inAuto inFormat = iota
	inZON
	inJSON
)

// decode parses raw input into a generic Go value (map[string]any, []any,
// scalars). With inAuto the format is sniffed from the first meaningful byte.
func decode(data []byte, format inFormat) (any, error) {
	switch format {
	case inZON:
		return decodeZON(data)
	case inJSON:
		return decodeJSON(data)
	default:
		return decodeAuto(data)
	}
}

func decodeZON(data []byte) (any, error) {
	var v any
	if err := zon.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep the int/float distinction; resolved in toQuery
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// decodeAuto picks ZON or JSON from the first non-whitespace byte: '.' is ZON,
// '{'/'[' is JSON. Bare scalars parse the same in both, so it tries ZON first
// and falls back to JSON.
func decodeAuto(data []byte) (any, error) {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) > 0 {
		switch trimmed[0] {
		case '{', '[':
			return decodeJSON(data)
		case '.':
			return decodeZON(data)
		}
	}
	if v, err := decodeZON(data); err == nil {
		return v, nil
	}
	return decodeJSON(data)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd cmd/zon && go test ./... -run 'TestDecode' -v`
Expected: PASS for all `TestDecode*` cases.

- [ ] **Step 5: Commit**

```bash
git add cmd/zon/input.go cmd/zon/input_test.go
git commit -m "feat(cli): input decoding with ZON/JSON auto-detection"
```

---

### Task 3: Normalize to and from the gojq value model

**Files:**
- Create: `cmd/zon/normalize.go`
- Test: `cmd/zon/normalize_test.go`

gojq operates on `nil | bool | int | float64 | *big.Int | string | []any | map[string]any`. `toQuery` converts our decoded values into that model; `fromQuery` converts gojq results back into values our encoders accept.

- [ ] **Step 1: Write the failing tests**

Create `cmd/zon/normalize_test.go`:

```go
package main

import (
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/tristanisham/zon"
)

func TestToQueryScalars(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{int64(5), 5},
		{uint64(7), 7},
		{uint64(math.MaxUint64), new(big.Int).SetUint64(math.MaxUint64)},
		{float64(1.5), 1.5},
		{zon.EnumLiteral("debug"), "debug"},
		{"s", "s"},
		{true, true},
		{nil, nil},
		{json.Number("42"), 42},
		{json.Number("3.5"), 3.5},
	}
	for _, c := range cases {
		got := toQuery(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("toQuery(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestToQueryNested(t *testing.T) {
	in := map[string]any{
		"items": []any{int64(1), zon.EnumLiteral("a")},
	}
	got := toQuery(in)
	want := map[string]any{"items": []any{1, "a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toQuery nested = %#v, want %#v", got, want)
	}
}

func TestFromQueryBigInt(t *testing.T) {
	// A *big.Int that fits in int64 collapses back to int64.
	got := fromQuery(big.NewInt(123))
	if got != int64(123) {
		t.Errorf("fromQuery(big 123) = %#v, want int64(123)", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd cmd/zon && go test ./... -run 'TestToQuery|TestFromQuery' -v`
Expected: FAIL — `undefined: toQuery`, `undefined: fromQuery`.

- [ ] **Step 3: Implement normalize.go**

Create `cmd/zon/normalize.go`:

```go
package main

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"

	"github.com/tristanisham/zon"
)

// toQuery converts a value decoded from ZON or JSON into gojq's value model:
// nil, bool, int, float64, *big.Int, string, []any, map[string]any.
func toQuery(v any) any {
	switch x := v.(type) {
	case nil, bool, string, float64:
		return x
	case int64:
		return int(x)
	case uint64:
		if x <= math.MaxInt64 {
			return int(x)
		}
		return new(big.Int).SetUint64(x)
	case zon.EnumLiteral:
		return string(x)
	case json.Number:
		return numberToQuery(x)
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = toQuery(e)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, e := range x {
			s[i] = toQuery(e)
		}
		return s
	default:
		return v
	}
}

func numberToQuery(n json.Number) any {
	s := n.String()
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}
	if bi, ok := new(big.Int).SetString(s, 10); ok {
		return bi
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// fromQuery converts a gojq result into a value the output encoders accept.
// gojq emits int/float64/*big.Int/string/bool/nil and []any/map[string]any;
// the only conversion needed is collapsing *big.Int back to int64/uint64 when
// it fits so the ZON/JSON encoders can render it as a plain number.
func fromQuery(v any) any {
	switch x := v.(type) {
	case *big.Int:
		if x.IsInt64() {
			return x.Int64()
		}
		if x.IsUint64() {
			return x.Uint64()
		}
		return x
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = fromQuery(e)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, e := range x {
			s[i] = fromQuery(e)
		}
		return s
	default:
		return x
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd cmd/zon && go test ./... -run 'TestToQuery|TestFromQuery' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/zon/normalize.go cmd/zon/normalize_test.go
git commit -m "feat(cli): normalize values to/from gojq model"
```

---

### Task 4: Output encoders

**Files:**
- Create: `cmd/zon/output.go`
- Test: `cmd/zon/output_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/zon/output_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func encodeToString(t *testing.T, v any, opt encodeOptions) string {
	t.Helper()
	var buf bytes.Buffer
	if err := encode(&buf, v, opt); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

func TestEncodeZON(t *testing.T) {
	out := encodeToString(t, map[string]any{"a": 1}, encodeOptions{format: outZON})
	if !strings.Contains(out, ".a = 1") {
		t.Errorf("zon output = %q", out)
	}
}

func TestEncodeJSONCompact(t *testing.T) {
	out := encodeToString(t, map[string]any{"a": 1}, encodeOptions{format: outJSON, compact: true})
	if strings.TrimSpace(out) != `{"a":1}` {
		t.Errorf("json compact = %q", out)
	}
}

func TestEncodeRawString(t *testing.T) {
	out := encodeToString(t, "hello", encodeOptions{format: outZON, raw: true})
	if out != "hello\n" {
		t.Errorf("raw = %q, want \"hello\\n\"", out)
	}
}

func TestEncodeRawNonStringFallsBack(t *testing.T) {
	out := encodeToString(t, 1, encodeOptions{format: outJSON, raw: true, compact: true})
	if strings.TrimSpace(out) != "1" {
		t.Errorf("raw fallback = %q, want \"1\"", out)
	}
}

func TestEncodeTOMLRequiresTable(t *testing.T) {
	var buf bytes.Buffer
	err := encode(&buf, 1, encodeOptions{format: outTOML})
	if err == nil {
		t.Fatal("expected error encoding a scalar as TOML")
	}
	if !strings.Contains(err.Error(), "top-level table") {
		t.Errorf("error = %v, want it to mention top-level table", err)
	}
}

func TestEncodeYAML(t *testing.T) {
	out := encodeToString(t, map[string]any{"a": 1}, encodeOptions{format: outYAML})
	if !strings.Contains(out, "a: 1") {
		t.Errorf("yaml = %q", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd cmd/zon && go test ./... -run 'TestEncode' -v`
Expected: FAIL — `undefined: encode`, `undefined: encodeOptions`, `undefined: outZON`.

- [ ] **Step 3: Implement output.go**

Create `cmd/zon/output.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	gotoml "github.com/pelletier/go-toml/v2"
	"github.com/tristanisham/zon"
	yaml "gopkg.in/yaml.v3"
)

// outFormat selects the serialization used for results.
type outFormat int

const (
	outZON outFormat = iota
	outJSON
	outYAML
	outTOML
)

type encodeOptions struct {
	format  outFormat
	compact bool
	raw     bool
}

// encode writes a single gojq result to w in the configured format, followed by
// a newline. With raw set, string results are written verbatim (no quotes) and
// non-strings fall back to the configured format.
func encode(w io.Writer, v any, opt encodeOptions) error {
	v = fromQuery(v)

	if opt.raw {
		if s, ok := v.(string); ok {
			_, err := io.WriteString(w, s+"\n")
			return err
		}
	}

	var (
		out []byte
		err error
	)
	switch opt.format {
	case outZON:
		if opt.compact {
			out, err = zon.MarshalIndent(v, "", "")
		} else {
			out, err = zon.Marshal(v)
		}
	case outJSON:
		out, err = marshalJSON(v, opt.compact)
	case outYAML:
		out, err = yaml.Marshal(v)
	case outTOML:
		out, err = marshalTOML(v)
	default:
		return fmt.Errorf("unknown output format")
	}
	if err != nil {
		return err
	}

	out = bytes.TrimRight(out, "\n")
	_, err = w.Write(append(out, '\n'))
	return err
}

func marshalJSON(v any, compact bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if !compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func marshalTOML(v any) ([]byte, error) {
	if _, ok := v.(map[string]any); !ok {
		return nil, fmt.Errorf("TOML output requires a top-level table (object), got %T", v)
	}
	return gotoml.Marshal(v)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd cmd/zon && go test ./... -run 'TestEncode' -v`
Expected: PASS for all `TestEncode*` cases.

- [ ] **Step 5: Commit**

```bash
git add cmd/zon/output.go cmd/zon/output_test.go
git commit -m "feat(cli): output encoders for zon/json/yaml/toml + raw"
```

---

### Task 5: CLI wiring and the run() entry point

**Files:**
- Modify: `cmd/zon/main.go` (replace the Task 1 placeholder entirely)
- Test: `cmd/zon/main_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/zon/main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

// runString invokes run() with the given args and stdin, returning stdout,
// stderr, and the exit code.
func runString(args []string, stdin string) (string, string, int) {
	var out, errBuf bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestRunIdentityZON(t *testing.T) {
	out, errStr, code := runString([]string{"."}, `.{ .a = 1 }`)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errStr)
	}
	if !strings.Contains(out, ".a = 1") {
		t.Errorf("out = %q", out)
	}
}

func TestRunExtractFieldToJSON(t *testing.T) {
	out, _, code := runString([]string{"--to", "json", "-c", ".a"}, `.{ .a = 1 }`)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(out) != "1" {
		t.Errorf("out = %q, want 1", out)
	}
}

func TestRunDefaultFilterIsIdentity(t *testing.T) {
	out, _, code := runString([]string{}, `42`)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(out) != "42" {
		t.Errorf("out = %q, want 42 (default filter '.')", out)
	}
}

func TestRunNullInput(t *testing.T) {
	out, _, code := runString([]string{"-n", "--to", "json", "1 + 1"}, "")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(out) != "2" {
		t.Errorf("out = %q, want 2", out)
	}
}

func TestRunRawOutput(t *testing.T) {
	out, _, code := runString([]string{"-r", ".v"}, `.{ .v = "x" }`)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out != "x\n" {
		t.Errorf("out = %q, want \"x\\n\"", out)
	}
}

func TestRunParseErrorExit2(t *testing.T) {
	_, errStr, code := runString([]string{"."}, `.{ .a = `)
	if code != 2 {
		t.Fatalf("exit %d, want 2; stderr=%q", code, errStr)
	}
}

func TestRunCompileErrorExit3(t *testing.T) {
	_, _, code := runString([]string{"this is not jq |"}, `1`)
	if code != 3 {
		t.Fatalf("exit %d, want 3", code)
	}
}

func TestRunBadFlagExit1(t *testing.T) {
	_, _, code := runString([]string{"--to", "xml", "."}, `1`)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestRunFileInput(t *testing.T) {
	out, errStr, code := runString([]string{"--to", "json", "-c", ".name", "testdata/build.zig.zon"}, "")
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errStr)
	}
	if strings.TrimSpace(out) != `"example"` {
		t.Errorf("out = %q, want \"example\"", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd cmd/zon && go test ./... -run 'TestRun' -v`
Expected: FAIL — `undefined: run`.

- [ ] **Step 3: Replace main.go with the full implementation**

Replace the entire contents of `cmd/zon/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the testable entry point. It returns the process exit code:
// 0 success, 1 usage error, 2 input parse error, 3 filter compile error,
// 4 runtime or output-encode error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("zon", flag.ContinueOnError)
	fs.SetOutput(stderr)

	inStr := fs.String("in", "auto", "input format: auto|zon|json")
	toStr := fs.String("to", "zon", "output format: zon|json|yaml|toml")

	var compact, raw, nullInput, sortKeys, showVersion bool
	fs.BoolVar(&compact, "c", false, "compact output")
	fs.BoolVar(&compact, "compact", false, "compact output")
	fs.BoolVar(&raw, "r", false, "raw string output")
	fs.BoolVar(&raw, "raw-output", false, "raw string output")
	fs.BoolVar(&nullInput, "n", false, "use null as the single input")
	fs.BoolVar(&nullInput, "null-input", false, "use null as the single input")
	fs.BoolVar(&sortKeys, "S", false, "sort object keys (maps are always emitted sorted)")
	fs.BoolVar(&sortKeys, "sort-keys", false, "sort object keys (maps are always emitted sorted)")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")

	fs.Usage = func() {
		fmt.Fprint(stderr, usageText)
	}

	if err := fs.Parse(args); err != nil {
		return 1 // flag package already printed the error
	}

	if showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}

	inFmt, err := parseInFormat(*inStr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	opt, err := parseEncodeOptions(*toStr, compact, raw)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	rest := fs.Args()
	filterSrc := "."
	var files []string
	if len(rest) > 0 {
		filterSrc = rest[0]
		files = rest[1:]
	}

	query, err := gojq.Parse(filterSrc)
	if err != nil {
		fmt.Fprintf(stderr, "filter compile error: %v\n", err)
		return 3
	}
	code, err := gojq.Compile(query)
	if err != nil {
		fmt.Fprintf(stderr, "filter compile error: %v\n", err)
		return 3
	}

	var inputs []any
	if nullInput {
		inputs = []any{nil}
	} else {
		inputs, err = loadInputs(stdin, files, inFmt)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 2
		}
	}

	for _, in := range inputs {
		iter := code.Run(toQuery(in))
		for {
			res, ok := iter.Next()
			if !ok {
				break
			}
			if e, isErr := res.(error); isErr {
				fmt.Fprintf(stderr, "runtime error: %v\n", e)
				return 4
			}
			if err := encode(stdout, res, opt); err != nil {
				fmt.Fprintf(stderr, "output error: %v\n", err)
				return 4
			}
		}
	}
	return 0
}

// loadInputs reads each file (or stdin when no files are given) and decodes it
// into a single value, returning the values in order.
func loadInputs(stdin io.Reader, files []string, format inFormat) ([]any, error) {
	type source struct {
		name string
		data []byte
	}
	var sources []source
	if len(files) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source{"<stdin>", data})
	} else {
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			sources = append(sources, source{f, data})
		}
	}

	var out []any
	for _, s := range sources {
		v, err := decode(s.data, format)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseInFormat(s string) (inFormat, error) {
	switch s {
	case "auto":
		return inAuto, nil
	case "zon":
		return inZON, nil
	case "json":
		return inJSON, nil
	default:
		return 0, fmt.Errorf("invalid --in %q: want auto, zon, or json", s)
	}
}

func parseEncodeOptions(to string, compact, raw bool) (encodeOptions, error) {
	var f outFormat
	switch to {
	case "zon":
		f = outZON
	case "json":
		f = outJSON
	case "yaml":
		f = outYAML
	case "toml":
		f = outTOML
	default:
		return encodeOptions{}, fmt.Errorf("invalid --to %q: want zon, json, yaml, or toml", to)
	}
	return encodeOptions{format: f, compact: compact, raw: raw}, nil
}

const usageText = `zon - a jq-clone for ZON data

Usage:
    zon [flags] FILTER [files...]

FILTER is a jq expression (default "."). Input is read from the given files in
order, or from stdin when no files are given.

Flags:
    --in auto|zon|json     input format (default auto)
    --to zon|json|yaml|toml output format (default zon)
    -c, --compact          compact, single-line output
    -r, --raw-output       print bare strings without quotes
    -n, --null-input       run the filter once with null input
    -S, --sort-keys        sort object keys
    --version              print version
    -h, --help             print this help

Notes:
    Enum literals decode to plain strings (.debug -> "debug").
    TOML output requires a top-level table (object).
`
```

Note: the `sortKeys` flag is accepted for jq compatibility. Because gojq's value
model uses unordered `map[string]any`, objects are always emitted in sorted-key
order regardless; the flag is therefore effectively always on. This is documented
in the usage text and the spec.

- [ ] **Step 4: Run the full test suite**

Run: `cd cmd/zon && go test ./... -v`
Expected: PASS — all Task 2–5 tests green.

- [ ] **Step 5: Manual smoke test**

Run:

```bash
cd cmd/zon
go run . . testdata/build.zig.zon
go run . --to json -c '.dependencies | keys' testdata/build.zig.zon
echo '{"a":1,"b":2}' | go run . --in json '.a'
```

Expected: first prints the file reformatted as ZON; second prints a compact JSON array of dependency names like `["known_folders","mecha"]`; third prints `1` (as ZON).

- [ ] **Step 6: Commit**

```bash
git add cmd/zon/main.go cmd/zon/main_test.go
git commit -m "feat(cli): wire run() entry point with gojq pipeline"
```

---

### Task 6: Final verification (vet, fmt, full build)

**Files:** none (verification only)

- [ ] **Step 1: Format and vet**

Run:

```bash
cd cmd/zon && gofmt -l . && go vet ./...
```

Expected: `gofmt -l` prints nothing; `go vet` reports nothing.

- [ ] **Step 2: Confirm the library module is untouched and still green**

Run (from repo root):

```bash
gofmt -l *.go && go vet ./... && go test ./...
```

Expected: no formatting issues, vet clean, library tests PASS. Crucially, the top-level `go.mod` still has no `require` block (the CLI deps live only in `cmd/zon/go.mod`).

- [ ] **Step 3: Commit any formatting fixes (if needed)**

```bash
git add -A
git commit -m "chore(cli): gofmt + vet cleanup"
```

(Skip if there is nothing to commit.)

---

## Release prep (manual, before publishing — NOT part of the build)

`go install github.com/tristanisham/zon/cmd/zon@latest` will not work while
`cmd/zon/go.mod` contains the `replace github.com/tristanisham/zon => ../..`
directive, because `go install ...@version` rejects modules with replace
directives. When you cut a release:

1. Tag the library (e.g. `git tag v0.1.0 && git push --tags`).
2. In `cmd/zon/go.mod`, remove the `replace` line and pin the require to that
   tag: `require github.com/tristanisham/zon v0.1.0`.
3. `cd cmd/zon && go mod tidy` and commit.

This step is intentionally left out of the implementation tasks above so local
development and testing stay offline and tag-free.

---

## Self-Review

**Spec coverage:**
- Query engine (gojq) → Task 5 (`gojq.Parse`/`Compile`/`Run`). ✓
- Input ZON + JSON, auto-detect → Task 2. ✓
- Output ZON/JSON/YAML/TOML + raw → Task 4. ✓
- Default output ZON → Task 5 (`--to` default `"zon"`). ✓
- jq-clone single command, `run()` entry point → Task 5. ✓
- Nested module, library stays zero-dep → Task 1 + Task 6 Step 2. ✓
- Normalization (int64/uint64/EnumLiteral/json.Number, *big.Int) → Task 3. ✓
- Exit codes 0/1/2/3/4 → Task 5 tests cover 0, 1, 2, 3 explicitly; 4 covered by the TOML-table error path in Task 4 and the encode/runtime branches in Task 5. ✓
- Flags (`--in`, `--to`, `-c`, `-r`, `-n`, `-S`, `--version`, `-h`) → Task 5. ✓
- TOML top-level table limitation → Task 4 (`marshalTOML`). ✓
- Enum-to-string limitation → documented in usage text (Task 5) and spec. ✓
- Testing (unit + golden via build.zig.zon) → Tasks 2–5; fixture copied in Task 1. ✓

**Placeholder scan:** No TBD/TODO; every code step contains complete code; the Task 1 minimal `main.go` is explicitly replaced in Task 5.

**Type consistency:** `inFormat`/`inAuto`/`inZON`/`inJSON`, `outFormat`/`outZON…`, `encodeOptions{format,compact,raw}`, `decode`, `toQuery`, `fromQuery`, `encode`, `run`, `loadInputs`, `parseInFormat`, `parseEncodeOptions` are used consistently across tasks. `encodeOptions` carries no `sortKeys` field (the flag is accepted in `run` but intentionally not threaded into encoding — documented), so there is no signature mismatch.

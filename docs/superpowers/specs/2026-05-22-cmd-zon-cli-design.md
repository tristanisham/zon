# cmd/zon — a jq-clone for ZON (design)

Date: 2026-05-22
Status: Approved (design); ready for implementation planning.

## Goal

Ship `zon`, a command-line tool that queries, filters, and transforms ZON data
using jq filter syntax. It mirrors `jq`'s ergonomics, reads ZON or JSON, runs a
gojq filter, and writes ZON (default), JSON, YAML, or TOML.

## Decisions (locked)

- **Query engine:** full jq, via `github.com/itchyny/gojq`. jq filter syntax is
  the centerpiece, not a bespoke path mini-language.
- **Input formats:** ZON and JSON (auto-detected by default; overridable).
- **Output formats:** ZON, JSON, YAML, TOML, plus a `-r` raw-string mode.
- **Default output:** **ZON**, regardless of input. This is a ZON-first tool;
  other formats are opt-in via `--to`.
- **CLI shape:** single jq-clone command, `zon [flags] FILTER [files...]`.
- **Module layout:** **nested module** at `cmd/zon/go.mod`. The library's own
  `go.mod` stays zero-dependency (stdlib only); all CLI dependencies (gojq, yaml,
  toml) are isolated to the CLI module so library consumers do not inherit them.

## Invocation

```
zon [flags] FILTER [files...]
```

- `FILTER` is a jq expression. Defaults to `.` (identity) when omitted.
- `files...` are read in order; with no files, input is read from stdin.

### Flags (v1)

| Flag | Default | Meaning |
|------|---------|---------|
| `--in auto\|zon\|json` | `auto` | Input format. |
| `--to zon\|json\|yaml\|toml` | `zon` | Output format. |
| `-c, --compact` | off | Compact (single-line) output. |
| `-r, --raw-output` | off | Print bare strings without quotes; non-strings fall back to `--to`. |
| `-n, --null-input` | off | Do not read input; run FILTER once with `null`. |
| `-S, --sort-keys` | off | Sort object keys in output. |
| `--version` | — | Print version and exit. |
| `-h, --help` | — | Print usage and exit. |

Deferred to a later version (explicitly out of scope for v1): `-s/--slurp`,
`--arg`/`--argjson`, `-e/--exit-status`, `--tab`/custom indent.

## Pipeline

1. **Read & detect.** Read each file (or stdin). With `--in auto`, sniff the
   first non-whitespace byte: `.` → ZON, `{`/`[` → JSON. Bare scalars (numbers,
   strings, `true`/`false`/`null`) parse identically in both, so auto tries ZON
   then JSON. `--in zon|json` forces the decoder.
2. **Decode** to `any` via `zon.Unmarshal` or `encoding/json.Unmarshal`.
3. **Normalize to gojq's value model.** gojq expects
   `nil | bool | int | float64 | *big.Int | string | []any | map[string]any`.
   Conversions:
   - `int64` / `uint64` → `int` when it fits, otherwise `*big.Int`.
   - `float64` → `float64`.
   - `zon.EnumLiteral` → `string` (**lossy**; see Limitations).
   - `map[string]any`, `[]any` → recurse.
   - `string`, `bool`, `nil` → pass through.
4. **Run** the compiled gojq filter, iterating over all produced results.
5. **Encode** each result with the chosen encoder:
   - **ZON:** `zon.Marshal` (pretty) or `zon.MarshalIndent("", "")` (`-c`).
   - **JSON:** `encoding/json` (indented, or compact with `-c`).
   - **YAML:** `gopkg.in/yaml.v3`.
   - **TOML:** `github.com/pelletier/go-toml/v2`.
   - **Raw (`-r`):** string results printed verbatim; non-strings use `--to`.

## Package layout

Single `package main` under `cmd/zon/`, split into focused files:

- `main.go` — flag parsing and orchestration. Real entry point delegates to a
  testable `run(args []string, stdin io.Reader, stdout, stderr io.Writer) int`
  so the whole CLI can be exercised in tests.
- `input.go` — format detection and decoding (ZON/JSON → `any`).
- `normalize.go` — to/from the gojq value model.
- `output.go` — the four encoders plus raw mode.

The library module (`github.com/tristanisham/zon`) is unchanged and remains
zero-dependency. `cmd/zon/go.mod` declares its own module requiring the library;
local development uses a `replace` directive pointing at `../..`.

## Error handling & exit codes

| Code | Condition |
|------|-----------|
| 0 | Success. |
| 1 | Usage error (bad flags/args). |
| 2 | Input parse error. ZON `SyntaxError` carries `Line`/`Col`, reported as `file:line:col: message`. |
| 3 | Filter compile error (invalid jq). |
| 4 | Runtime or output-encode error (e.g. TOML rejecting a non-table top-level value). |

All diagnostics go to stderr; data goes to stdout.

## Testing

- **Unit:** format auto-detection; normalization of `int64`/`uint64`/`EnumLiteral`
  (including `uint64` overflow → `*big.Int`); each output encoder; raw mode; the
  TOML non-table top-level error.
- **End-to-end / golden:** table-driven tests calling `run(...)` with sample
  inputs and asserting stdout/stderr/exit code, reusing `testdata/build.zig.zon`.
  Golden output files under `cmd/zon/testdata/`.

## Limitations (intentional, v1)

- **Enum literals become strings.** `.debug` decodes to `"debug"`; a ZON→ZON
  filter round trip does not preserve enum-ness. Documented in `--help` and the
  README.
- **Numbers route through gojq's int/float model.** Integers that exceed `int`
  are preserved via `*big.Int`; ordinary floats use `float64`.
- **TOML top level must be a table.** Emitting a bare scalar or array as TOML is
  an error (exit 4).

## Dependencies (CLI module only)

- `github.com/itchyny/gojq` — jq engine.
- `gopkg.in/yaml.v3` — YAML output.
- `github.com/pelletier/go-toml/v2` — TOML output.

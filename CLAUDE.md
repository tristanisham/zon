# CLAUDE.md — zon

Working notes for the `github.com/tristanisham/zon` library and `cmd/zon` CLI.

## Working agreements (from the user)

- **Challenge all assumptions.** Do not trust that the current implementation matches
  the real ZON spec — verify against Zig's `std.zon` grammar and prove behavior with tests.
- **TDD to validate the library.** Write a failing test that pins the intended behavior,
  watch it fail, then make it pass. Especially for the "assumptions to validate" below.
- **Keep this checklist current.** Update statuses and the dated notes log as work lands.

## Checklist

### Library (implemented)
- [x] lexer / parser / AST
- [x] decode (reflection) + struct tags
- [x] encode (pretty + compact) + Marshaler/Unmarshaler + EnumLiteral
- [x] streaming Encoder/Decoder
- [x] initial unit + round-trip tests (82.3% coverage at last run)

### TDD correctness validation (challenge assumptions) — DONE
- [x] Verify the real ZON grammar against Zig `std.zon` (web/spec check)
- [x] One failing-first test per validated/refuted assumption (see `spec_test.go`)
- [x] Fix library where behavior diverges from the spec
- Result: only one real bug — leading-zero decimals were accepted as octal.
  Fixed in `lexer.go` (rejected at lex time). inf/nan, char literals, `//`-only
  comments, hex/oct/bin leading-zero digits, enum round-trip all confirmed correct.

### Benchmarks for the full ZON spec — DONE (10-way partition)
- [x] Isolation strategy: git init + per-agent worktrees (decided with user)
- [x] 10 `bench_*_test.go` files, one per partition, unique symbol prefix each
- [x] Integrate + run `go test -bench=. -benchmem` clean (all targets pass, vet/gofmt clean)
- Note: written inline (not via parallel agents); `b.Loop()` + `b.ReportAllocs()`.

### Fuzzing — DONE
- [x] `fuzz_test.go`: FuzzUnmarshalNoPanic, FuzzLexNoPanic, FuzzParseNoPanic,
  FuzzMarshalUnmarshalRoundTrip (~45 shared seeds). No panics in ~1.7M/1.5M execs.
- [x] **Non-UTF-8 encoder lossiness bug — FIXED.** Was: lexer copies raw bytes into
  string/`@"..."`/multiline literals, but `writeZonString` ranged over runes and
  emitted U+FFFD for invalid bytes, so `\\<0xFF>` round-tripped to `"�"`. Root cause
  was deeper than the encoder: `\xNN` in a string decoded via `WriteRune` to a
  *codepoint* (`"\xff"`→U+00FF), so no escape could reproduce a raw byte. Fix (both
  sides, matching Zig byte-escape semantics):
  - `lexer.go` `scanStringBody`: `\xNN` in a string now writes a raw byte
    (char literals keep codepoint semantics via `scanEscape`).
  - `encode.go` `writeZonString`: iterates byte-wise, emits `\xNN` for any byte not
    part of a valid UTF-8 sequence (and for control bytes < 0x20).
  - Pinned by `TestNonUTF8StringRoundTrips` + `TestStringHexEscapeIsRawByte`; round-trip
    fuzzer no longer skips non-UTF-8 inputs. Regression seeds remain under
    `testdata/fuzz/FuzzMarshalUnmarshalRoundTrip/`.
  - ⚠️ Behavior change: `"\xNN"` in a string now decodes to a single raw byte instead
    of codepoint U+00NN. Spec-correct (Zig), but a breaking change for any caller that
    relied on the old `\x`→codepoint decoding.

### Conformance against Zig's own ZON front end — DONE (2026-10-23)
- [x] Reference oracle `testdata/oracle/oracle.zig` (Zig 0.17.0): `std.zig.Ast` (.zon) +
  `std.zig.ZonGen`, prints a canonical value or the first error per document.
- [x] `TestConformance`: 655-case corpus (`conformance_cases_test.go`), Zig verdicts in
  `testdata/conformance.golden` (runs without Zig; regenerate with `-zon.oracle -zon.update`).
- [x] `TestDifferentialFuzz` / `TestDifferentialEncoder` (need `-zon.oracle`): ~2.6M generated
  docs + 50k marshaled random values, 0 mismatches after the fixes below.
- Bugs found and fixed (all were accepted/emitted as valid but Zig rejects, or vice versa):
  doc comments (`///`, `//!`) accepted; control chars (incl. TAB, DEL) accepted in comments,
  strings, char and multiline literals; lone CR in comments/multiline; multiline `\r\n` kept
  the CR; multiline strings split on blank lines/comments (Zig joins them); `-0` integers;
  number literal validation (repeated/trailing/after-prefix `_`, `0X`/`0O`/`0B`, empty prefix,
  bad digits for base, leading zeros in floats); hex floats without `p` rejected; out-of-range
  floats rejected (Zig rounds to ±inf); `\u{}` above U+10FFFF or surrogates accepted (became
  U+FFFD); keywords usable as bare names; empty / NUL `@""` names; duplicate struct fields;
  `@"true"` etc. read as literals; `-'a'` accepted; `. {}` (trivia between `.` and `{`)
  rejected; leading UTF-8 BOM rejected. Encoder: keyword names unquoted (`.fn`), empty/NUL
  names and duplicate tag names emitted, raw DEL emitted in strings.
- Decode: float literals that are exactly integral now decode into int types (std.zon rule).
- Known upstream Zig 0.17.0 bugs (library is correct, harness reports them separately):
  `std.fmt.parseFloat(f128, "9_703e283")` = 9.7e286 (drops a digit; f64 parse is right);
  `parseCharLiteral` panics on a char literal with a truncated UTF-8 sequence (`'\xc3'`).
- Remaining deliberate differences from typed `std.zon.parse` (Go API choices, json-like):
  unknown fields ignored, `null` into non-pointer sets zero, arrays truncate extra items,
  ints wider than 64 bits decode only into floats/`any`.

### CLI `cmd/zon` (jq-for-zon) — DESIGN PARKED, awaiting approval
- [ ] Approve design (gojq-backed, ZON/JSON in; ZON/JSON/YAML/TOML/raw out)
- [ ] Implement after library is validated

## Assumptions — VALIDATED against the ZON spec (Ziggit spec thread + ANTLR grammar)

1. ✅ **`inf` / `nan` are valid ZON floats.** WRONG to suspect — they are the one thing
   ZON adds on top of Zig's literal subset. Support kept; covered by `TestSpecInfNanRoundTrip`.
2. ❌→FIXED **Leading-zero decimal.** Was accepted as octal (`0123`→83). Zig/ZON forbids it.
   Now rejected in the lexer. Covered by `TestLeadingZeroIntegerRejected`. Leading-zero
   digits inside hex/oct/bin remain valid (`TestSpecLeadingZeroDigitsInOtherBases`).
3. ✅ **`//` line comments only.** No block comments. Refined against Zig: `///` and `//!`
   are doc-comment tokens and are rejected in ZON (`////` is a plain comment); comment text
   may not contain control characters other than a CR directly before LF (TAB included).
4. ❌→FIXED **Multiline string** joins `\\` lines with `\n`, no trailing newline — but a
   blank line does NOT end it: each `\\` line is a token, so lines separated by any
   whitespace or comments form one literal (verified against Zig 0.17.0). A `\r\n` ending
   is not content.
5. ✅ **Char literal `'a'`** is a valid value, decodes to its integer codepoint
   (`TestSpecCharLiteralValue`).
6. ✅ **`-` is only a numeric sign** (ZON has no operators).
7. ✅ **Empty `.{}`** decodes into struct/map/slice.
8. ✅ **Top-level value may be any ZON value**, not only a struct.
9. ✅ **Hex floats, underscores, `0o`/`0b`** in-spec.
10. ✅ **Encoder emits floats with `.0`** so they re-parse as floats.

## Benchmark partition (10 agents, one file each, prefix `Benchmark<Area>_…`)

1. `bench_scalars_test.go` — bool, null
2. `bench_integers_test.go` — decimal/hex/oct/bin/underscores (lex+parse+decode+encode)
3. `bench_floats_test.go` — decimal/scientific/hex floats (+ special values if valid)
4. `bench_strings_test.go` — quoted strings, escapes, unicode
5. `bench_multiline_chars_test.go` — multiline strings + char literals
6. `bench_enums_test.go` — enum literals + EnumLiteral round-trip
7. `bench_structs_test.go` — flat/wide structs, tags, omitempty
8. `bench_tuples_test.go` — tuples/arrays, nested arrays, slices/arrays decode
9. `bench_nested_test.go` — deep nesting + realistic build.zig.zon (large doc) round-trip
10. `bench_reflect_test.go` — maps, pointers, decode-into-`any`, custom Marshaler/Unmarshaler

Each agent: package `zon` (white-box, may hit internal lex/parse), unique symbol prefix,
`-benchmem`, no shared package-level helpers (avoid duplicate-symbol build breaks).

## Notes log

- 2026-10-23: README now documents the accepted grammar, the differences from Zig (Zig bugs,
  numeric precision, json-like decode vs `std.zon.parse`) and encoder output. Every claim was
  checked against `conformance.golden` or a probe run; keep it in sync when behavior changes.

- 2026-10-23: Built a Zig 0.17.0 oracle and differential harness. Earlier "validated"
  assumptions came from the spec thread/ANTLR grammar and missed ~20 divergences (see the
  conformance section). Lesson: validate against the reference implementation, not prose.
- 2026-05-22: Library implemented and green. Started validation+benchmark planning.
  Flagged that benchmarks ≠ validation, and that 10 parallel agents need isolation
  because the package compiles as a whole. CLI design parked pending approval.

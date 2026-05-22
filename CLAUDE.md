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

### Benchmarks for the full ZON spec — IN PROGRESS (10-way partition)
- [x] Isolation strategy: git init + per-agent worktrees (decided with user)
- [ ] Dispatch agents per partition (see below), unique file + symbol prefix each
- [ ] Integrate + run `go test -bench=. -benchmem` clean

### CLI `cmd/zon` (jq-for-zon) — DESIGN PARKED, awaiting approval
- [ ] Approve design (gojq-backed, ZON/JSON in; ZON/JSON/YAML/TOML/raw out)
- [ ] Implement after library is validated

## Assumptions — VALIDATED against the ZON spec (Ziggit spec thread + ANTLR grammar)

1. ✅ **`inf` / `nan` are valid ZON floats.** WRONG to suspect — they are the one thing
   ZON adds on top of Zig's literal subset. Support kept; covered by `TestSpecInfNanRoundTrip`.
2. ❌→FIXED **Leading-zero decimal.** Was accepted as octal (`0123`→83). Zig/ZON forbids it.
   Now rejected in the lexer. Covered by `TestLeadingZeroIntegerRejected`. Leading-zero
   digits inside hex/oct/bin remain valid (`TestSpecLeadingZeroDigitsInOtherBases`).
3. ✅ **`//` line comments only.** Confirmed (ANTLR grammar: `LineComment '//' ~[\r\n]*`);
   Zig has no block comments.
4. ✅ **Multiline string** joins `\\` lines with `\n`, no trailing newline, blank line ends it.
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

- 2026-05-22: Library implemented and green. Started validation+benchmark planning.
  Flagged that benchmarks ≠ validation, and that 10 parallel agents need isolation
  because the package compiles as a whole. CLI design parked pending approval.

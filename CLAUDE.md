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

### TDD correctness validation (challenge assumptions) — NOT STARTED
- [ ] Verify the real ZON grammar against Zig `std.zon` (web/spec check)
- [ ] One failing-first test per validated/refuted assumption below
- [ ] Fix library where behavior diverges from the spec

### Benchmarks for the full ZON spec — NOT STARTED (10-way partition)
- [ ] Decide isolation strategy (git worktrees vs shared-dir unique files)
- [ ] Dispatch agents per partition (see below), unique file + symbol prefix each
- [ ] Integrate + run `go test -bench=. -benchmem` clean

### CLI `cmd/zon` (jq-for-zon) — DESIGN PARKED, awaiting approval
- [ ] Approve design (gojq-backed, ZON/JSON in; ZON/JSON/YAML/TOML/raw out)
- [ ] Implement after library is validated

## Assumptions to validate (challenge these with tests + spec)

These are baked into the current lexer/parser/encoder and may be WRONG:

1. **`inf` / `nan` are valid ZON floats.** Suspect — Zig `std.zon` may not accept bare
   `inf`/`nan`. If not, remove from parser/encoder.
2. **Leading-zero decimal → octal.** `parseInt` uses `strconv.ParseInt(raw, 0, 64)`, so
   `0123` parses as octal. ZON likely forbids leading-zero ints entirely. Decide + test.
3. **`//` line comments only.** Confirm ZON has no block/doc comments to skip.
4. **Multiline string continuation** ends at a blank line / non-`\\` line. Verify joining
   rule and trailing-newline semantics against Zig.
5. **Char literal `'a'` decodes to its integer codepoint.** Confirm ZON even allows char
   literals as values, and the intended Go mapping.
6. **`-` is only a numeric sign** (handled via `tokenMinus`). Confirm no other unary use.
7. **Empty `.{}`** is treated as an empty tuple that also decodes into struct/map. Confirm.
8. **Top-level value may be any ZON value**, not only a struct.
9. **Hex floats `0x1.8p4`, underscores, `0o`/`0b`** are all in-spec.
10. **Encoder emits floats with `.0`** to avoid round-tripping as ints — confirm this is
    valid ZON float syntax (vs requiring an explicit type).

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

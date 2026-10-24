# ZON conformance

Validated on 2026-10-24 against Zig 0.17.0. The reference is Zig's
[ZON front end](https://github.com/ziglang/zig/blob/0.17.0/lib/std/zig/ZonGen.zig),
[tokenizer](https://github.com/ziglang/zig/blob/0.17.0/lib/std/zig/tokenizer.zig),
[number literals](https://github.com/ziglang/zig/blob/0.17.0/lib/std/zig/number_literal.zig),
[string literals](https://github.com/ziglang/zig/blob/0.17.0/lib/std/zig/string_literal.zig),
and [runtime parser](https://github.com/ziglang/zig/blob/0.17.0/lib/std/zon/parse.zig).

`reference_test.go` preserves 655 cases from the earlier Delta work in
`testdata/zig-conformance.tsv`. The default Go suite compares both acceptance and
canonical values against these recorded Zig results. With `ZON_TEST_ZIG` set,
the suite builds `testdata/oracle/oracle.zig` and revalidates 654 recorded results
against the installed reference compiler. One malformed character literal
(`'` followed by a truncated UTF-8 byte and `'`) panics in Zig 0.17.0; its
reference result is explicitly skipped, while Go rejects it normally.

`conformance_test.go` adds focused regression cases, checks the exported decode
and encode behavior, and submits fresh encoder output to Zig. Those cases also
seed the existing Go fuzzers.

## Reproduce

```bash
go test ./...
ZON_TEST_ZIG=/absolute/path/to/zig go test . \
  -run 'TestConformance|TestReferenceCorpus|TestZigConformance|TestEncoderRejectsDuplicateTags' \
  -count=1 -v
go test -race ./...
go vet ./...
go test . -run '^$' -fuzz '^FuzzUnmarshalNoPanic$' -fuzztime=15s -parallel=4
go test . -run '^$' -fuzz '^FuzzMarshalUnmarshalRoundTrip$' -fuzztime=15s -parallel=4
```

The optional reference tests target Zig 0.17.0's APIs. They do not download or
switch the user's Zig installation. Normal library and CLI builds require Go
only.

## Corrections

- Reject duplicate fields, including names duplicated through escapes.
- Distinguish bare keywords, quoted identifiers, and scalar literals; quote
  reserved keywords when encoding names. Reject empty and NUL-containing names,
  and duplicate struct tags that would emit duplicate fields.
- Validate all numeric syntax before decoding, including ignored fields.
  Reject uppercase base prefixes, misplaced separators, leading decimal zeros,
  negative integer zero, and negated character literals.
- Support whitespace/comments between `.` and `{`, an initial UTF-8 BOM, hex
  floats without an explicit exponent, and `1.e5`/`0x1.p0` spellings.
- Reject out-of-range Unicode escapes, surrogate string escapes, raw control
  bytes, and documentation comments. Preserve byte escapes and non-UTF-8 strings.
- Join multiline string tokens across whitespace and ordinary comments;
  normalize CRLF line endings. Escape DEL when encoding strings.
- Preserve integers beyond 64 bits using `*big.Int` during generic decoding and
  support encoding `big.Int` and `*big.Int`. Convert integer literals in any base
  to Go floats. Narrow overflowing float literals to infinity, matching Zig.

## Go API boundaries

Syntax acceptance is separate from conversion to a Go destination. A valid ZON
value can fail to fit the requested Go type. Integer destinations accept integer
and character literals; float destinations accept integer and float literals.
Unlike Zig's typed runtime decoder, the Go API does not coerce integral float
literals into integer destinations. This follows the library's encoding/json
style of typed decoding.

Generic floats use IEEE float64. Zig's front end parses via f128; double
rounding and known Zig float-parser differences can produce different low bits.
For these recorded cases the suite explicitly accepts Zig's direct f64 result
as well as its f128-to-f64 result. This is not a claim of f128 precision.

An empty `.{}` decodes generically as `[]any{}` because it does not identify an
object or an array. The package accepts it into explicitly typed structs, maps,
slices, and arrays. Unknown Go struct fields are ignored, but their ZON syntax
is still checked. Comments and source formatting are not preserved. Custom
`MarshalZON` implementations remain responsible for returning valid ZON.

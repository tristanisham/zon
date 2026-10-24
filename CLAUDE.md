# Working notes — zon

`github.com/tristanisham/zon` provides a Go library and a jq-style CLI in
`cmd/zon`. The core package uses only the standard library. The CLI uses ZVM's
`github.com/urfave/cli/v3` version; use the same ZVM libraries for shared needs.
The accepted CLI interface is documented in README.md.

## Working agreements

- Verify syntax against Zig's actual front end; do not infer it from Go syntax
  or assume earlier compliance claims are complete.
- Pin library corrections with failing-first regression tests.
- Keep the conformance report, CLI help, README, and manpage current when
  behavior changes. Preserve other local work.

## Current validation

- [x] Lexer, parser, reflection codecs, tags, and streaming API.
- [x] Unit tests, round trips, benchmarks, and fuzz targets.
- [x] Revalidated recorded conformance results against Zig 0.17.0.
- [x] Added regression tests for additional syntax and encoder defects.
- [x] Implemented the approved CLI: jq filters; ZON/JSON input;
  ZON/JSON/YAML/TOML and raw string output.
- [x] Removed obsolete `.delta` workspace metadata.

Use `go test ./...`, `go test -race ./...`, and `go vet ./...`.
See CONFORMANCE.md for the optional live Zig oracle and precise compatibility
boundaries. Build with `go build -o build/zon ./cmd/zon`.

## Notes

- 2026-05-22: Initial implementation and preliminary validation; CLI design parked.
- 2026-10-24: User authorized fresh validation and CLI implementation, including
  revising previous decisions. Reused the earlier Delta conformance corpus,
  independently refreshed its reference results, and corrected further defects.
  The earlier claim that only leading-zero decimal integers were wrong was
  incomplete; CONFORMANCE.md records the current results.

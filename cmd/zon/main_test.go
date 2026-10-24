package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
	"go.yaml.in/yaml/v3"
)

func invoke(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	err := newCommand(strings.NewReader(input), &out, &stderr).Run(context.Background(), append([]string{"zon"}, args...))
	if stderr.Len() != 0 {
		t.Logf("stderr: %s", &stderr)
	}
	return out.String(), err
}

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		args        []string
		want, err   string
	}{
		{name: "default identity", input: `.{.b=2,.a=1}`, want: ".{\n    .a = 1,\n    .b = 2,\n}\n"},
		{name: "enum to string", input: `.{.name=.demo}`, args: []string{"-c", ".name"}, want: "\"demo\"\n"},
		{name: "raw string", input: `.{.version="1.2.3"}`, args: []string{"-r", ".version"}, want: "1.2.3\n"},
		{name: "raw fallback", input: `.{.a=1}`, args: []string{"-rc", "."}, want: ".{.a=1}\n"},
		{name: "json output", input: `.{.b=2,.a=1}`, args: []string{"--to", "json", "."}, want: "{\n  \"a\": 1,\n  \"b\": 2\n}\n"},
		{name: "json auto", input: `{"a":[1,2]}`, args: []string{"-c", ".a | map(.+1)"}, want: ".{2,3}\n"},
		{name: "json stream", input: "1\n2\n3", args: []string{"--in", "json", "--to", "json", ".+1"}, want: "2\n3\n4\n"},
		{name: "scalar json fallback", input: `"\u0041"`, args: []string{"-r"}, want: "A\n"},
		{name: "comment detection", input: "// config\n.{.a=2}", args: []string{".a"}, want: "2\n"},
		{name: "zon forced", input: `{"a":1}`, args: []string{"--in", "zon"}, err: "unexpected character"},
		{name: "json forced", input: `.{.a=1}`, args: []string{"--in", "json"}, err: "JSON input"},
		{name: "null input", input: "invalid", args: []string{"-nc", "{answer: 6*7}"}, want: ".{.answer=42}\n"},
		{name: "sort keys", input: `.{.z=1,.a=2}`, args: []string{"-Sc", "--to", "json"}, want: "{\"a\":2,\"z\":1}\n"},
		{name: "dependency keys", input: `.{.dependencies=.{.b=.{},.a=.{}}}`, args: []string{"-c", ".dependencies | keys"}, want: ".{\"a\",\"b\"}\n"},
		{name: "multiple results", input: `.{1,2,3}`, args: []string{".[] | select(.>1)"}, want: "2\n3\n"},
		{name: "uint64 arithmetic", input: `18446744073709551615`, args: []string{".+1"}, want: "18446744073709551616\n"},
		{name: "big integer ZON", input: `123456789012345678901234567890`, args: []string{".+1"}, want: "123456789012345678901234567891\n"},
		{name: "big integer JSON", input: `123456789012345678901234567890`, args: []string{"--in", "json", "--to", "json", ".+1"}, want: "123456789012345678901234567891\n"},
		{name: "computed big integer", args: []string{"-n", "2 | pow(.;100)"}, want: "1.2676506002282294e+30\n"},
		{name: "yaml output", input: `.{.a=1,.b=.{2,true}}`, args: []string{"--to", "yaml"}, want: "a: 1\nb:\n    - 2\n    - true\n"},
		{name: "yaml compact", input: `.{.a=.{1,2}}`, args: []string{"-c", "--to", "yaml"}, want: "{a: [1, 2]}\n"},
		{name: "yaml documents", input: `.{1,2}`, args: []string{"--to", "yaml", ".[]"}, want: "1\n---\n2\n"},
		{name: "yaml big integer", input: `18446744073709551616`, args: []string{"--to", "yaml"}, want: "!!int 18446744073709551616\n"},
		{name: "toml output", input: `.{.name="demo",.version=1}`, args: []string{"--to", "toml"}, want: "name = 'demo'\nversion = 1\n"},
		{name: "toml inline", input: `.{.a=.{.b=1}}`, args: []string{"-c", "--to", "toml"}, want: "a = {b = 1}\n"},
		{name: "toml scalar", input: `1`, args: []string{"--to", "toml"}, err: "top-level object"},
		{name: "toml null", input: `.{.a=null}`, args: []string{"--to", "toml"}, err: "cannot represent null"},
		{name: "toml integer overflow", input: `.{.a=18446744073709551616}`, args: []string{"--to", "toml"}, err: "signed 64-bit"},
		{name: "toml multiple results", input: `.{.a=1}`, args: []string{"--to", "toml", "., ."}, err: "one result"},
		{name: "invalid input format", args: []string{"--in", "yaml"}, err: "invalid input format"},
		{name: "invalid output format", args: []string{"--to", "csv"}, err: "invalid output format"},
		{name: "invalid filter", args: []string{"-n", ".["}, err: "filter:"},
		{name: "undefined function", args: []string{"-n", "unknown_function"}, err: "filter:"},
		{name: "runtime error", input: `1`, args: []string{".foo"}, err: "filter:"},
		{name: "invalid zon", input: `.{.a=1,.a=2}`, err: "duplicate struct field"},
		{name: "invalid json", input: `{"a":`, err: "JSON input"},
		{name: "nonfinite json", input: `inf`, args: []string{"--to", "json"}, err: "unsupported value"},
		{name: "empty explicit zon", args: []string{"--in", "zon"}, err: "unexpected token"},
		{name: "empty json stream", args: []string{"--in", "json"}, want: ""},
		{name: "empty filter result", input: `1`, args: []string{"empty"}, want: ""},
		{name: "halt", input: `1 2`, args: []string{"--in", "json", "halt"}, want: ""},
		{name: "invalid output key", input: `{"":1}`, args: []string{"--in", "json"}, err: "identifier must be nonempty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := invoke(t, tc.input, tc.args...)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("error=%v, want containing %q; out=%q", err, tc.err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("output=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestFiles(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.zon"), filepath.Join(dir, "b.json")
	for path, data := range map[string]string{a: `.{.x=1}`, b: `{"x":3}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := invoke(t, `.{.x=2}`, ".x", a, "-", b)
	if err != nil || got != "1\n2\n3\n" {
		t.Fatalf("output=%q, error=%v", got, err)
	}
	_, err = invoke(t, "", ".", filepath.Join(dir, "missing"))
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing file: %v", err)
	}
	got, err = invoke(t, "bad", "-n", "1", filepath.Join(dir, "missing"))
	if err != nil || got != "1\n" {
		t.Fatalf("null-input read file: %q %v", got, err)
	}
}

func TestHelpAndVersion(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "--version"} {
		out, err := invoke(t, "", arg)
		if err != nil || !strings.Contains(out, "zon") {
			t.Fatalf("%s: %q %v", arg, out, err)
		}
	}
	_, err := invoke(t, "", "--unknown")
	if err == nil {
		t.Fatal("unknown flag accepted")
	}
}

func TestHaltExitCode(t *testing.T) {
	_, err := invoke(t, "", "-n", "halt_error(7)")
	var exit cli.ExitCoder
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("halt_error(7): %v", err)
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := newCommand(strings.NewReader(""), io.Discard, io.Discard).Run(ctx, []string{"zon", "-n", "def f: f; f"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context: %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestIOErrors(t *testing.T) {
	err := newCommand(failingReader{}, io.Discard, io.Discard).Run(context.Background(), []string{"zon"})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read: %v", err)
	}
	err = newCommand(strings.NewReader("1"), failingWriter{}, io.Discard).Run(context.Background(), []string{"zon"})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write: %v", err)
	}
}

func TestYAMLStringKeys(t *testing.T) {
	out, err := invoke(t, `.{.@"true"="false",.@"null"="yes"}`, "--to", "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := yaml.Unmarshal([]byte(out), &value); err != nil {
		t.Fatal(err)
	}
	if value["true"] != "false" || value["null"] != "yes" {
		t.Fatalf("changed string values: %#v", value)
	}
}

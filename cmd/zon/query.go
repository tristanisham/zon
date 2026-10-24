package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/tristanisham/zon"
	"github.com/urfave/cli/v3"
)

var errHalt = errors.New("halt")

func run(ctx context.Context, cmd *cli.Command) error {
	input, output := cmd.String("in"), cmd.String("to")
	if input != "auto" && input != "zon" && input != "json" {
		return fmt.Errorf("invalid input format %q (want auto, zon, json)", input)
	}
	if output != "zon" && output != "json" && output != "yaml" && output != "toml" {
		return fmt.Errorf("invalid output format %q (want zon, json, yaml, toml)", output)
	}
	filter, files := ".", cmd.Args().Slice()
	if len(files) > 0 {
		filter, files = files[0], files[1:]
	}
	query, err := gojq.Parse(filter)
	if err != nil {
		return fmt.Errorf("filter: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("filter: %w", err)
	}
	emitter := &resultWriter{writer: cmd.Writer, format: output, compact: cmd.Bool("compact"), raw: cmd.Bool("raw-output")}
	evaluate := func(value any) error { return evaluate(ctx, code, emitter, value) }
	if cmd.Bool("null-input") {
		return finish(evaluate(nil))
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		var data []byte
		if path == "-" {
			data, err = io.ReadAll(cmd.Reader)
		} else {
			data, err = os.ReadFile(path)
		}
		if err == nil {
			err = readInput(data, input, evaluate)
		}
		if errors.Is(err, errHalt) {
			return nil
		}
		if err != nil {
			if path == "-" {
				path = "<stdin>"
			}
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func finish(err error) error {
	if errors.Is(err, errHalt) {
		return nil
	}
	return err
}

func evaluate(ctx context.Context, code *gojq.Code, out *resultWriter, input any) error {
	input, err := queryValue(input)
	if err != nil {
		return err
	}
	iter := code.RunWithContext(ctx, input)
	for {
		value, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, ok := value.(error); ok {
			var halt *gojq.HaltError
			if errors.As(err, &halt) {
				if halt.Value() == nil && halt.ExitCode() == 0 {
					return errHalt
				}
				return cli.Exit(halt.Error(), halt.ExitCode())
			}
			return fmt.Errorf("filter: %w", err)
		}
		// gojq may retain json.Number literals in its output.
		value, err = queryValue(value)
		if err != nil {
			return err
		}
		if err := out.write(value); err != nil {
			return err
		}
	}
}

func readInput(data []byte, format string, emit func(any) error) error {
	trimmed := bytes.TrimSpace(data)
	if format == "auto" {
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			format = "json"
		} else {
			var value any
			if err := zon.Unmarshal(data, &value); err == nil {
				return emit(value)
			}
			// A leading dot is unambiguously ZON. For scalars try JSON next.
			if len(trimmed) > 0 && (trimmed[0] == '.' || bytes.HasPrefix(trimmed, []byte("//"))) {
				format = "zon"
			} else {
				format = "json"
			}
		}
	}
	if format == "zon" {
		var value any
		if err := zon.Unmarshal(data, &value); err != nil {
			return err
		}
		return emit(value)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		var value any
		err := dec.Decode(&value)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("JSON input: %w", err)
		}
		if err := emit(value); err != nil {
			return err
		}
	}
}

// gojq accepts int, float64 and *big.Int, but not the int64/uint64 or
// EnumLiteral values produced by the library. Preserve integer precision.
func queryValue(value any) (any, error) {
	switch v := value.(type) {
	case zon.EnumLiteral:
		return string(v), nil
	case int64:
		if int64(int(v)) == v {
			return int(v), nil
		}
		return big.NewInt(v), nil
	case uint64:
		if v <= uint64(^uint(0)>>1) {
			return int(v), nil
		}
		return new(big.Int).SetUint64(v), nil
	case json.Number:
		if !strings.ContainsAny(string(v), ".eE") {
			if n, ok := new(big.Int).SetString(string(v), 10); ok {
				if n.IsInt64() && int64(int(n.Int64())) == n.Int64() {
					return int(n.Int64()), nil
				}
				return n, nil
			}
		}
		f, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return nil, fmt.Errorf("number %s: %w", v, err)
		}
		return f, nil
	case []any:
		for i, item := range v {
			x, err := queryValue(item)
			if err != nil {
				return nil, err
			}
			v[i] = x
		}
		return v, nil
	case map[string]any:
		for key, item := range v {
			x, err := queryValue(item)
			if err != nil {
				return nil, err
			}
			v[key] = x
		}
		return v, nil
	default:
		return value, nil
	}
}

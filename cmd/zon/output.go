package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math/big"
	"slices"

	"github.com/pelletier/go-toml/v2"
	"github.com/tristanisham/zon"
	"go.yaml.in/yaml/v3"
)

type resultWriter struct {
	writer       io.Writer
	format       string
	compact, raw bool
	documents    int
}

func (w *resultWriter) write(value any) error {
	if str, ok := value.(string); ok && w.raw {
		_, err := fmt.Fprintln(w.writer, str)
		return err
	}
	var data []byte
	var err error
	switch w.format {
	case "zon":
		if w.compact {
			data, err = zon.MarshalIndent(value, "", "")
		} else {
			data, err = zon.Marshal(value)
		}
	case "json":
		if w.compact {
			data, err = json.Marshal(value)
		} else {
			data, err = json.MarshalIndent(value, "", "  ")
		}
	case "yaml":
		var node *yaml.Node
		node, err = yamlValue(value, w.compact)
		if err == nil {
			data, err = yaml.Marshal(node)
		}
		if w.documents > 0 {
			data = append([]byte("---\n"), data...)
		}
	case "toml":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("TOML output requires a top-level object")
		}
		if w.documents > 0 {
			return fmt.Errorf("TOML output supports one result per invocation")
		}
		var converted any
		converted, err = tomlValue(value)
		if err == nil {
			var buf bytes.Buffer
			err = toml.NewEncoder(&buf).SetTablesInline(w.compact).Encode(converted)
			data = buf.Bytes()
		}
	}
	if err != nil {
		return fmt.Errorf("%s output: %w", w.format, err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	n, err := w.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		w.documents++
	}
	return err
}

// YAML's reflection encoder sees big.Int as an empty struct. Explicit nodes
// retain arbitrary-precision integers and apply flow style recursively.
func yamlValue(value any, compact bool) (*yaml.Node, error) {
	if n, ok := value.(*big.Int); ok {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: n.String()}, nil
	}
	node := new(yaml.Node)
	switch v := value.(type) {
	case []any:
		node.Kind, node.Tag = yaml.SequenceNode, "!!seq"
		for _, item := range v {
			child, err := yamlValue(item, compact)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
	case map[string]any:
		// Emit keys in deterministic order and handle big.Int recursively.
		keys := slices.Sorted(maps.Keys(v))
		node.Kind, node.Tag = yaml.MappingNode, "!!map"
		for _, key := range keys {
			child, err := yamlValue(v[key], compact)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
		}
	default:
		if err := node.Encode(value); err != nil {
			return nil, err
		}
	}
	if compact {
		node.Style = yaml.FlowStyle
	}
	return node, nil
}

// TOML integers are signed 64-bit, and TOML has no null value.
func tomlValue(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, fmt.Errorf("TOML cannot represent null")
	case *big.Int:
		if !v.IsInt64() {
			return nil, fmt.Errorf("integer %s exceeds TOML's signed 64-bit range", v)
		}
		return v.Int64(), nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			x, err := tomlValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			x, err := tomlValue(item)
			if err != nil {
				return nil, err
			}
			out[key] = x
		}
		return out, nil
	default:
		return value, nil
	}
}

package zon

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncoderDecoderRoundTrip(t *testing.T) {
	type point struct {
		X int `zon:"x"`
		Y int `zon:"y"`
	}
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(point{1, 2}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("Encode output should end with newline: %q", buf.String())
	}

	var got point
	if err := NewDecoder(&buf).Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != (point{1, 2}) {
		t.Errorf("got %+v, want {1 2}", got)
	}
}

func TestEncoderSetIndent(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	enc.SetIndent("", "")
	if err := enc.Encode([]int{1, 2}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if buf.String() != ".{1,2}\n" {
		t.Errorf("got %q, want compact output", buf.String())
	}
}

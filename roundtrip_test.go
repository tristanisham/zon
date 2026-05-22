package zon

import (
	"os"
	"reflect"
	"testing"
)

// Manifest mirrors the shape of a build.zig.zon file.
type Manifest struct {
	Name    string                `zon:"name"`
	Version string                `zon:"version"`
	MinZig  string                `zon:"minimum_zig_version"`
	Deps    map[string]Dependency `zon:"dependencies"`
	Paths   []string              `zon:"paths"`
}

type Dependency struct {
	URL  string `zon:"url"`
	Hash string `zon:"hash"`
	Lazy bool   `zon:"lazy,omitempty"`
}

func TestRoundTripBuildZigZon(t *testing.T) {
	data, err := os.ReadFile("testdata/build.zig.zon")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	var first Manifest
	if err := Unmarshal(data, &first); err != nil {
		t.Fatalf("first Unmarshal: %v", err)
	}
	if first.Name != "example" || first.Version != "0.3.1" {
		t.Fatalf("unexpected manifest: %+v", first)
	}
	if !first.Deps["known_folders"].Lazy {
		t.Errorf("known_folders.lazy = false, want true")
	}
	if len(first.Paths) != 4 {
		t.Errorf("paths = %v", first.Paths)
	}

	// Marshal back out, then decode again: the two decoded values must match.
	out, err := Marshal(first)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var second Manifest
	if err := Unmarshal(out, &second); err != nil {
		t.Fatalf("second Unmarshal of:\n%s\nerror: %v", out, err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("round trip mismatch:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

// Decoding into any and re-encoding should also be stable.
func TestRoundTripGeneric(t *testing.T) {
	data, err := os.ReadFile("testdata/build.zig.zon")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	var v any
	if err := Unmarshal(data, &v); err != nil {
		t.Fatalf("Unmarshal into any: %v", err)
	}
	out, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var again any
	if err := Unmarshal(out, &again); err != nil {
		t.Fatalf("re-Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(v, again) {
		t.Errorf("generic round trip mismatch")
	}
}

package zon_test

import (
	"fmt"

	"github.com/tristanisham/zon"
)

func Example() {
	type Manifest struct {
		Name    string   `zon:"name"`
		Version string   `zon:"version"`
		Paths   []string `zon:"paths"`
	}

	// Unmarshal a ZON document into a Go struct.
	src := `.{
        .name = "demo",
        .version = "0.1.0",
        .paths = .{ "", "src" },
    }`
	var m Manifest
	if err := zon.Unmarshal([]byte(src), &m); err != nil {
		panic(err)
	}
	fmt.Println(m.Name, m.Version, m.Paths)

	// Marshal a Go value back into ZON.
	out, err := zon.Marshal(m)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))

	// Output:
	// demo 0.1.0 [ src]
	// .{
	//     .name = "demo",
	//     .version = "0.1.0",
	//     .paths = .{
	//         "",
	//         "src",
	//     },
	// }
}

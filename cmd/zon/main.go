package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/urfave/cli/v3"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := newCommand(os.Stdin, os.Stdout, os.Stderr).Run(ctx, os.Args); err != nil {
		if err.Error() != "" {
			fmt.Fprintln(os.Stderr, "zon:", err)
		}
		var exit cli.ExitCoder
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}

func newCommand(in io.Reader, out, stderr io.Writer) *cli.Command {
	v := version
	if info, ok := debug.ReadBuildInfo(); ok && v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	return &cli.Command{
		Name: "zon", Usage: "Query and convert ZON data using jq filters", ArgsUsage: "[FILTER] [files...]", Version: v,
		Description: "FILTER defaults to . (identity). Read stdin when no files are given; use - for stdin among files.\n   Examples: zon --to json . build.zig.zon; zon -r '.version' build.zig.zon",
		Reader:      in, Writer: out, ErrWriter: stderr, HideHelpCommand: true, UseShortOptionHandling: true,
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		OnUsageError:   func(_ context.Context, _ *cli.Command, err error, _ bool) error { return err },
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "in", Value: "auto", Usage: "input format: auto, zon, json"},
			&cli.StringFlag{Name: "to", Value: "zon", Usage: "output format: zon, json, yaml, toml"},
			&cli.BoolFlag{Name: "compact", Aliases: []string{"c"}, Usage: "compact output (YAML flow style; TOML inline tables)"},
			&cli.BoolFlag{Name: "raw-output", Aliases: []string{"r"}, Usage: "print string results without quotes"},
			&cli.BoolFlag{Name: "null-input", Aliases: []string{"n"}, Usage: "run once with null without reading input"},
			&cli.BoolFlag{Name: "sort-keys", Aliases: []string{"S"}, Usage: "sort object keys (output is always sorted)"},
		},
		Action: run,
	}
}

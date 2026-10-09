// Command maskriver is the fail-closed MaskRiver CLI scaffold.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/runner"
)

const version = "0.0.0-dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return usageError(stderr)
		}
		usage(stdout)
		return 0
	case "version", "--version":
		if len(args) != 1 {
			return usageError(stderr)
		}
		fmt.Fprintln(stdout, "MaskRiver", version)
		return 0
	case "scan", "mask", "validate":
	default:
		return usageError(stderr)
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	options := config.Options{}
	if command == "mask" {
		flags.BoolVar(&options.Apply, "apply", false, "request writes (not implemented; default is dry run)")
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: maskriver %s [flags]\nScaffold only; database operations are not implemented.\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return usageError(stderr)
	}
	if command == "mask" {
		fmt.Fprintf(stdout, "dry-run=%t; no database opened; no writes performed\n", options.DryRun())
	}
	fmt.Fprintf(stderr, "%s: %v\n", command, runner.ErrNotImplemented)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "MaskRiver — data masking engine (scaffold)")
	fmt.Fprintln(w, "Usage: maskriver <scan|mask|validate|version> [flags]")
	fmt.Fprintln(w, "Database commands are not implemented. mask defaults to dry run; --apply is also refused.")
}

func usageError(w io.Writer) int {
	fmt.Fprintln(w, "invalid arguments; use maskriver --help")
	return 2
}

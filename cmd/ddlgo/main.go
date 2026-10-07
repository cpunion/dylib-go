// ddlgo inspects binary files or calls entries with caller-supplied C signatures.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	dylib "github.com/cpunion/dylib-go"
)

func run(args []string, output io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ddlgo inspect FILE | ddlgo call [OPTIONS] 'func NAME(TYPES)RESULT' VALUES... FILE... | ddlgo call [OPTIONS] 'NAME(VALUE:TYPE, ...)RESULT' FILE... | ddlgo call [OPTIONS] SYMBOL A B FILE...")
	}
	switch args[0] {
	case "inspect":
		if len(args) != 2 {
			return fmt.Errorf("inspect requires one file")
		}
		i, e := dylib.Inspect(args[1])
		if e != nil {
			return e
		}
		enc := json.NewEncoder(output)
		enc.SetIndent("", "  ")
		return enc.Encode(i)
	case "call":
		fs := flag.NewFlagSet("call", flag.ContinueOnError)
		fs.SetOutput(output)
		process := fs.Bool("process", false, "resolve exported host symbols")
		keep := fs.Bool("keep-libraries", false, "retain runtime-bearing OS libraries until process exit")
		if e := fs.Parse(args[1:]); e != nil {
			return e
		}
		input, err := parseCallInput(fs.Args())
		if err != nil {
			return err
		}
		return executeCall(input, dylib.Options{ProcessSymbols: *process, KeepLibraries: *keep}, output)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}

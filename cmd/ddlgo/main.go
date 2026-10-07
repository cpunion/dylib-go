// ddlgo inspects binary files or calls a known int32_t(int32_t,int32_t) entry.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	dylib "github.com/cpunion/llgo-dylib"
)

func run(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ddlgo inspect FILE | ddlgo call [-process] SYMBOL A B FILE...")
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(i)
	case "call":
		fs := flag.NewFlagSet("call", flag.ContinueOnError)
		process := fs.Bool("process", false, "resolve exported host symbols")
		keep := fs.Bool("keep-libraries", false, "retain runtime-bearing OS libraries until process exit")
		if e := fs.Parse(args[1:]); e != nil {
			return e
		}
		v := fs.Args()
		if len(v) < 4 {
			return fmt.Errorf("call requires SYMBOL A B FILE...")
		}
		a, e := strconv.ParseInt(v[1], 10, 32)
		if e != nil {
			return e
		}
		b, e := strconv.ParseInt(v[2], 10, 32)
		if e != nil {
			return e
		}
		s := dylib.New(dylib.Options{ProcessSymbols: *process, KeepLibraries: *keep})
		defer s.Close()
		for _, p := range v[3:] {
			if e = s.Load(p); e != nil {
				return e
			}
		}
		r, e := s.CallInt32(v[0], int32(a), int32(b))
		if e != nil {
			return e
		}
		fmt.Println(r)
		return s.Close()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/abi/signature"
	examplecall "github.com/cpunion/dylib-go/examples/call"
)

type callInput struct {
	signature.Invocation
	files  []string
	legacy bool
}

func parseCallInput(args []string) (callInput, error) {
	if len(args) == 0 {
		return callInput{}, fmt.Errorf("call requires a declaration or invocation followed by FILE...")
	}
	text := strings.TrimSpace(args[0])
	var input callInput
	var err error
	fields := strings.Fields(text)
	switch {
	case len(fields) > 0 && fields[0] == "func":
		input.Declaration, err = signature.Parse(text)
		if err != nil {
			return callInput{}, err
		}
		count := len(input.Signature.Args)
		if len(args) < count+2 {
			return callInput{}, fmt.Errorf("signature requires %d arguments followed by at least one file", count)
		}
		for i := range input.Signature.Args {
			v, err := signature.ParseTypedValue(input.Signature.ArgumentType(i), args[i+1])
			if err != nil {
				return callInput{}, fmt.Errorf("argument %d: %w", i+1, err)
			}
			input.Args = append(input.Args, v)
		}
		input.files = args[count+1:]
	case strings.Contains(text, "("):
		input.Invocation, err = signature.ParseCall(text)
		if err != nil {
			return callInput{}, err
		}
		input.files = args[1:]
	default:
		if len(args) < 4 {
			return callInput{}, fmt.Errorf("legacy call requires SYMBOL A B FILE...")
		}
		input.Declaration = signature.Declaration{Name: text, Signature: abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}}
		input.legacy = true
		for i, text := range args[1:3] {
			v, err := strconv.ParseInt(text, 10, 32)
			if err != nil {
				return callInput{}, fmt.Errorf("argument %d: %w", i+1, err)
			}
			input.Args = append(input.Args, abi.Int32(int32(v)))
		}
		input.files = args[3:]
	}
	if input.Name == "" || len(input.files) == 0 {
		return callInput{}, fmt.Errorf("call requires a symbol and at least one input file")
	}
	return input, nil
}

func executeCall(input callInput, options dylib.Options, output io.Writer) error {
	if !input.legacy && !abi.Available() {
		return fmt.Errorf("dynamic signatures require a CLI built with -tags libffi: %w", abi.ErrUnavailable)
	}
	s := dylib.New(options)
	defer s.Close()
	for _, p := range input.files {
		if err := s.Load(p); err != nil {
			return err
		}
	}
	var result abi.Value
	if abi.Available() {
		f, err := s.Bind(input.Name, input.Signature)
		if err != nil {
			return err
		}
		result, err = f.Call(input.Args...)
		if err != nil {
			return err
		}
	} else {
		// Keep the original demonstration available without libffi. All
		// caller-supplied signatures use the dynamic backend above.
		symbol, err := s.Resolve(input.Name)
		if err != nil {
			return err
		}
		err = symbol.WithAddress(func(address uintptr) error {
			v, err := examplecall.Int32(address, int32(input.Args[0].Bits), int32(input.Args[1].Bits))
			result = abi.Int32(v)
			return err
		})
		if err != nil {
			return err
		}
	}
	if err := s.Close(); err != nil {
		return err
	}
	text, err := signature.FormatValue(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, text)
	return err
}

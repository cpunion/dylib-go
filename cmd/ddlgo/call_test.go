package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestCallInputs(t *testing.T) {
	for _, args := range [][]string{
		{"func add(int32,int32)int32", "20", "22", "add.o", "other.a"},
		{"add(20:int32 22:int32)int32", "add.o", "other.a"},
		{"add", "20", "022", "add.o", "other.a"},
	} {
		input, err := parseCallInput(args)
		if err != nil || input.Name != "add" || !reflect.DeepEqual(input.Args, []abi.Value{abi.Int32(20), abi.Int32(22)}) || !reflect.DeepEqual(input.files, []string{"add.o", "other.a"}) {
			t.Fatalf("%v: %+v, %v", args, input, err)
		}
	}
	input, err := parseCallInput([]string{"func no_result()", "input.so"})
	if err != nil || len(input.Args) != 0 || input.Signature.Result != abi.Void {
		t.Fatalf("void: %+v, %v", input, err)
	}
}

func TestInputErrorsBeforeLoading(t *testing.T) {
	for _, args := range [][]string{
		{}, {"inspect"}, {"inspect", "a.o", "b.o"}, {"unknown", "x"},
		{"call", "add", "20", "22"}, {"call", "add(20:int32 22:int32)int32"},
		{"call", "func add(int32,int32)int32", "20", "input.o"},
		{"call", "func add(string)int32", "20", "input.o"},
		{"call", "func add(uint32)uint32", "-1", "input.o"},
		{"call", "add", "2147483648", "22", "input.o"},
		{"call", "-abi=unknown", "func f()", "input.o"},
		{"call", "-variadic-from=0", "func f(int32)", "1", "input.o"},
		{"call", "-variadic-from=2", "func f(int32)", "1", "input.o"},
		{"call", "-variadic-from=-2", "func f()", "input.o"},
		{"call", "-abi=stdcall", "-variadic-from=1", "func f(int32)", "1", "input.o"},
	} {
		var output bytes.Buffer
		err := run(args, &output)
		if err == nil || strings.Contains(err.Error(), "no such file") {
			t.Fatalf("%v: expected input error before loading, got %v", args, err)
		}
	}
}

func TestMissingDynamicBackend(t *testing.T) {
	if abi.Available() {
		t.Skip("this check exercises builds without the optional backend")
	}
	var output bytes.Buffer
	err := run([]string{"call", "func add(int32,int32)int32", "20", "22", "missing.o"}, &output)
	if !errors.Is(err, abi.ErrUnavailable) || !strings.Contains(err.Error(), "-tags libffi") {
		t.Fatalf("missing backend: %v", err)
	}
}

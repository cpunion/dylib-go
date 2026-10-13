package clang

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var errDefaultPacking = errors.New("clang: compiler default packing requires driver inspection")

type recordPackingPolicy struct {
	defaultCap *uint64
	pragmas    map[string]uint64
}

// Global packing lives in Clang's language options, not record AST attributes.
// Ask the driver for the effective frontend arguments rather than reinterpreting
// user flags, response files or compiler configuration defaults.
func compilerDefaultPacking(ctx context.Context, compiler, path string, flags []string) (uint64, error) {
	args := append(append([]string(nil), flags...), "-###", "-x", "c", "-std=c17", "-include", path, "-fsyntax-only", "-")
	data, err := runCompilerStream(ctx, compiler, args, "", maxDiagnostics, true)
	if err != nil {
		return 0, err
	}
	return decodeDefaultPacking(string(data))
}

func decodeDefaultPacking(data string) (uint64, error) {
	var packing uint64
	commands := 0
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "\"") {
			continue
		}
		args, err := driverArguments(line)
		if err != nil {
			return 0, fmt.Errorf("clang: invalid compiler driver arguments")
		}
		if len(args) < 2 || args[1] != "-cc1" {
			continue
		}
		commands++
		for _, arg := range args[2:] {
			if strings.HasPrefix(arg, "-fpack-struct=") {
				packing, err = strconv.ParseUint(strings.TrimPrefix(arg, "-fpack-struct="), 10, 64)
				if err != nil || !validPacking(packing) {
					return 0, fmt.Errorf("clang: unsupported compiler default packing")
				}
			}
		}
	}
	if commands != 1 {
		return 0, fmt.Errorf("clang: expected one compiler frontend command")
	}
	return packing, nil
}

// Clang -### quotes each argument using C string escaping, including Windows
// paths. Decode complete arguments so text inside -D values is never a flag.
func driverArguments(line string) ([]string, error) {
	var args []string
	for line != "" {
		if line[0] != '"' {
			return nil, fmt.Errorf("unquoted driver argument")
		}
		end := 1
		for ; end < len(line); end++ {
			if line[end] == '\\' {
				end++
				continue
			}
			if line[end] == '"' {
				break
			}
		}
		if end >= len(line) {
			return nil, fmt.Errorf("unterminated driver argument")
		}
		arg, err := strconv.Unquote(line[:end+1])
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		line = line[end+1:]
		if line != "" && line[0] != ' ' && line[0] != '\t' {
			return nil, fmt.Errorf("missing driver argument separator")
		}
		line = strings.TrimSpace(line)
	}
	return args, nil
}

func (t Target) msLayout() bool {
	return t.OS == "windows" && strings.Contains(t.Triple, "msvc")
}

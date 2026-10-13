package clang

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var errRecordPacking = errors.New("clang: record packing requires compiler probes")

func validPacking(value uint64) bool {
	return value <= 16 && (value == 0 || value&(value-1) == 0)
}

func validatePacking(r Record) error {
	if !validPacking(r.MaxFieldAlignment) {
		return fmt.Errorf("clang: invalid packing for %s", r.Name)
	}
	return nil
}

// Clang can report an explicit expression alignment that pragma packing does
// not preserve (notably a GNU packed record). Bound it by the record's actual
// base alignment and member offset before using it for native storage.
func memberStorageAlignment(expression, base, offset uint64) (uint64, error) {
	if expression == 0 || base == 0 || !validAlignment(expression) || !validAlignment(base) {
		return 0, fmt.Errorf("invalid compiler member alignment")
	}
	for expression > base || offset%expression != 0 {
		expression /= 2
	}
	return expression, nil
}

func validateMemberAlignments(r Record, complete bool) error {
	packed := r.MaxFieldAlignment != 0
	if len(r.MemberAlignments) == 0 && (!complete || !packed) {
		return nil
	}
	if !packed || len(r.MemberAlignments) != len(r.Description.Fields) {
		return fmt.Errorf("clang: missing or invalid member alignments in %s", r.Name)
	}
	for i, alignment := range r.MemberAlignments {
		if alignment == 0 && r.bitfieldAt(i) != nil {
			continue
		}
		if alignment == 0 || !validAlignment(alignment) {
			return fmt.Errorf("clang: invalid member alignment in %s", r.Name)
		}
		if complete && (len(r.Layout.Offsets) != len(r.MemberAlignments) || alignment > r.Layout.Alignment || r.Layout.Offsets[i]%alignment != 0) {
			return fmt.Errorf("clang: compiler member alignment mismatch in %s", r.Name)
		}
	}
	return nil
}

// JSON identifies pragma attributes but does not serialize their numeric cap.
// Synthetic typedefs link accessible spellings to record IDs in one text dump.
// Never compare pointer-like AST IDs between different compiler invocations.
func recordPackingProbes(data []byte) (string, map[string]string, error) {
	var root astNode
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil, err
	}
	r := newTypeResolver(root, nil, nil)
	var names []string
	for id, name := range r.spellings {
		for _, child := range r.nodes[id].Inner {
			if child.Kind == "MaxFieldAlignmentAttr" {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	keys := make(map[string]string)
	var source strings.Builder
	for i, name := range names {
		key := fmt.Sprintf("dylib_record_packing_%d", i)
		keys[key] = name
		fmt.Fprintf(&source, "typedef %s %s;\n", name, key)
		if source.Len() > maxHeaderSize || len(keys) > maxDescriptionNodes {
			return "", nil, fmt.Errorf("clang: packing probes exceed source limit")
		}
	}
	if len(keys) == 0 {
		return "", nil, fmt.Errorf("clang: missing accessible packing declarations")
	}
	return source.String(), keys, nil
}

func decodePackingProbes(data []byte, keys map[string]string) (map[string]uint64, error) {
	type entry struct {
		kind, id, probe string
		depth           int
	}
	var stack []entry
	caps := make(map[string]uint64)
	records := make(map[string]string)
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		text := strings.TrimLeft(line, " |`-")
		fields := strings.Fields(text)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "0x") {
			continue
		}
		depth := (len(line) - len(text)) / 2
		for len(stack) != 0 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}
		node := entry{kind: fields[0], id: fields[1], depth: depth}
		if node.kind == "TypedefDecl" {
			for _, word := range fields[2:] {
				if _, ok := keys[word]; ok {
					if seen[word] {
						return nil, fmt.Errorf("clang: duplicate packing probe %s", word)
					}
					seen[word], node.probe = true, word
				}
			}
		}
		if node.kind == "MaxFieldAlignmentAttr" && len(stack) != 0 && stack[len(stack)-1].kind == "RecordDecl" {
			bits, err := strconv.ParseUint(fields[len(fields)-1], 10, 64)
			if err != nil || bits < 8 || bits%8 != 0 || !validPacking(bits/8) {
				return nil, fmt.Errorf("clang: invalid compiler packing cap")
			}
			id := stack[len(stack)-1].id
			if _, duplicate := caps[id]; duplicate {
				return nil, fmt.Errorf("clang: duplicate compiler packing cap")
			}
			caps[id] = bits / 8
		}
		if node.kind == "Record" {
			for i := len(stack) - 1; i >= 0; i-- {
				if probe := stack[i].probe; probe != "" {
					if _, duplicate := records[probe]; duplicate {
						return nil, fmt.Errorf("clang: ambiguous packing probe %s", probe)
					}
					records[probe] = node.id
					break
				}
			}
		}
		stack = append(stack, node)
	}
	result := make(map[string]uint64)
	for probe, name := range keys {
		value := caps[records[probe]]
		if !seen[probe] || value == 0 {
			return nil, fmt.Errorf("clang: missing compiler packing probe %s", probe)
		}
		result[name] = value
	}
	return result, nil
}

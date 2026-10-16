package cxx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBaseDiscoveryMetadataValidation(t *testing.T) {
	_, namespace, probes, err := baseProbes([]string{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	type alias struct {
		Kind, Name string
		Type       map[string]string
	}
	valid := []alias{}
	for _, probe := range probes {
		value := "true"
		if probe.base >= 0 {
			value = "false"
		}
		valid = append(valid, alias{"TypeAliasDecl", probe.name, map[string]string{"desugaredQualType": namespace + "::Answer<" + value + ">"}})
	}
	encode := func(kind, name string, aliases []alias) []byte {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(map[string]any{"kind": kind, "name": name, "inner": aliases}); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	data := encode("NamespaceDecl", namespace, valid)
	if got, err := decodeBases(data, namespace, []string{"A", "B"}, probes); err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	for name, input := range map[string][]byte{
		"invalid_json": []byte("invalid"), "missing": encode("NamespaceDecl", namespace, valid[:2]),
		"root_kind": encode("RecordDecl", namespace, valid), "namespace": encode("NamespaceDecl", "wrong", valid),
		"trailing":  append(append([]byte(nil), data...), []byte(" {}")...),
		"duplicate": encode("NamespaceDecl", namespace, append(append([]alias(nil), valid...), valid[0])),
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := decodeBases(input, namespace, []string{"A", "B"}, probes); err == nil || got != nil {
				t.Fatal("malformed inheritance metadata accepted", got, err)
			}
		})
	}
	for _, malformed := range []string{"Answer<true>", namespace + "::Answer<1>", namespace + "::Answer<false>"} {
		patched := strings.Replace(string(data), namespace+"::Answer<true>", malformed, 1)
		if got, err := decodeBases([]byte(patched), namespace, []string{"A", "B"}, probes); err == nil || got != nil {
			t.Fatal("invalid class validity/probe result accepted", got, err)
		}
	}
}

func TestBaseDiscoverySourceBudget(t *testing.T) {
	names := make([]string, maxDiscoveryTypes)
	for i := range names {
		names[i] = strings.Repeat("A", maxDiscoveryName-2) + fmt.Sprintf("%02d", i)
	}
	if source, namespace, probes, err := baseProbes(names); err == nil || source != "" || namespace != "" || probes != nil {
		t.Fatal("oversize query source accepted", err)
	}
}

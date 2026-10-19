package module

import "strings"

const maxSemanticVersionBytes = 256

type semanticVersion struct {
	core [3]string
	pre  []string
}

// Parse the SemVer 2.0.0 grammar, independently of native ABI contracts.
// Keep numbers as canonical digit strings: the specification has no integer
// width limit, and comparison must not overflow a machine integer.
func parseSemanticVersion(text string) (semanticVersion, bool) {
	var v semanticVersion
	if len(text) == 0 || len(text) > maxSemanticVersionBytes {
		return v, false
	}
	core, build, hasBuild := strings.Cut(text, "+")
	if hasBuild && !semanticIdentifiers(build, false) {
		return v, false
	}
	core, pre, hasPre := strings.Cut(core, "-")
	if hasPre {
		if !semanticIdentifiers(pre, true) {
			return v, false
		}
		v.pre = strings.Split(pre, ".")
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, part := range parts {
		if !semanticNumber(part) || len(part) > 1 && part[0] == '0' {
			return v, false
		}
		v.core[i] = part
	}
	return v, true
}

func semanticNumber(text string) bool {
	if text == "" {
		return false
	}
	for i := range text {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func semanticIdentifiers(text string, prerelease bool) bool {
	for _, part := range strings.Split(text, ".") {
		if part == "" || prerelease && semanticNumber(part) && len(part) > 1 && part[0] == '0' {
			return false
		}
		for i := range part {
			c := part[i]
			if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '-') {
				return false
			}
		}
	}
	return true
}

func semanticNumberOrder(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

func compareSemanticVersions(a, b semanticVersion) int {
	for i := range a.core {
		if order := semanticNumberOrder(a.core[i], b.core[i]); order != 0 {
			return order
		}
	}
	if len(a.pre) == 0 && len(b.pre) != 0 {
		return 1
	}
	if len(b.pre) == 0 && len(a.pre) != 0 {
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, y := a.pre[i], b.pre[i]
		if x == y {
			continue
		}
		xn, yn := semanticNumber(x), semanticNumber(y)
		if xn && yn {
			return semanticNumberOrder(x, y)
		}
		if xn {
			return -1
		}
		if yn {
			return 1
		}
		return strings.Compare(x, y)
	}
	if len(a.pre) < len(b.pre) {
		return -1
	}
	if len(a.pre) > len(b.pre) {
		return 1
	}
	return 0
}

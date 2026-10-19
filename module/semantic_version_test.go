package module

import (
	"strings"
	"testing"
)

func TestSemanticVersionGrammar(t *testing.T) {
	valid := []string{"0.0.0", "1.2.3", "1.2.3-0", "1.2.3-alpha.1", "1.2.3-x.7.z.92", "1.2.3----", "1.2.3-01a", "1.2.3+001", "1.2.3-rc.1+build.001", strings.Repeat("9", 80) + ".0.0", "1.2.3+" + strings.Repeat("x", 250)}
	invalid := []string{"", "v1.2.3", "1", "1.2", "1.2.3.4", "01.2.3", "1.02.3", "1.2.03", "+1.2.3", "-1.2.3", "1.2.3-", "1.2.3+", "1.2.3-01", "1.2.3-alpha.00", "1.2.3-alpha..one", "1.2.3+one..two", "1.2.3+one+two", "1.2.3-alpha_beta", "1.2.3+α", " 1.2.3", "1.2.3\n", "1.2.3\x00", "1.2.3+" + strings.Repeat("x", 251)}
	for _, value := range valid {
		if _, ok := parseSemanticVersion(value); !ok {
			t.Errorf("valid SemVer rejected: %q", value)
		}
	}
	for _, value := range invalid {
		if _, ok := parseSemanticVersion(value); ok {
			t.Errorf("invalid SemVer accepted: %q", value)
		}
	}
}

func TestSemanticVersionPrecedence(t *testing.T) {
	// SemVer's published prerelease sequence, followed by numeric boundaries
	// which cannot be compared lexically or through fixed-width integers.
	ordered := []string{"1.0.0-9", "1.0.0-18446744073709551615", "1.0.0-18446744073709551616", "1.0.0-ALPHA", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.9.0", "1.10.0", "2.0.0", "18446744073709551615.0.0", "18446744073709551616.0.0"}
	for i, a := range ordered {
		x, valid := parseSemanticVersion(a)
		if !valid {
			t.Fatal(a)
		}
		for j, b := range ordered {
			y, valid := parseSemanticVersion(b)
			if !valid {
				t.Fatal(b)
			}
			order := compareSemanticVersions(x, y)
			if i < j && order >= 0 || i == j && order != 0 || i > j && order <= 0 {
				t.Errorf("wrong precedence: %s vs %s: %d", a, b, order)
			}
		}
	}
	for _, pair := range [][2]string{{"1.2.3+one", "1.2.3+two"}, {"1.2.3-alpha+001", "1.2.3-alpha"}} {
		a, _ := parseSemanticVersion(pair[0])
		b, _ := parseSemanticVersion(pair[1])
		if compareSemanticVersions(a, b) != 0 {
			t.Fatal("build metadata changed precedence", pair)
		}
	}
}

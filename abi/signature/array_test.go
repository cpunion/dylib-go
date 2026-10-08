package signature

import (
	"testing"
)

func TestArrayLiterals(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"f({items:{20,22}}:struct{items [2]int32})", "{items:{20,22}}"},
		{"f(&{20,22}:*[2]int32)", "&{20,22}"},
		{"f({2:22,0:20}:*[3]int32)", "&{20,0,22}"},
		{"f({1:20,22}:*[4]int32)", "&{0,20,22,0}"},
		{"f({0x1:20,2_0}:*[0x3]int32)", "&{0,20,20}"},
		{"f({{20,22},{}}:*[2][2]int32)", "&{{20,22},{0,0}}"},
		{"f({{a:20},{b:22}}:*[2]struct{a,b int32})", "&{{a:20,b:0},{a:0,b:22}}"},
		{"f({p:{&20,&22}}:struct{p [2]*int32})", "{p:{&20,&22}}"},
		{"f({p:{&{20,22},nil}}:struct{p [2]*[2]int32})", "{p:{&{20,22},0x0}}"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			call, err := ParseCall(tc.text)
			if err != nil {
				t.Fatal(err)
			}
			got, err := FormatValue(call.Args[0])
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
			v, err := ParseTypedValue(call.Signature.ArgumentType(0), got)
			if err != nil {
				t.Fatal(err)
			}
			roundtrip, err := FormatValue(v)
			if err != nil || roundtrip != got {
				t.Fatalf("roundtrip=%q, %v", roundtrip, err)
			}
		})
	}
}

func TestRejectInvalidArrays(t *testing.T) {
	for _, text := range []string{
		"func f([2]int32)", "func f()[2]int32",
		"func f(*[]int32)", "func f(*[...]int32)",
		"func f(*[0]int32)", "func f(*[-1]int32)",
		"func f(*[1+1]int32)", "func f(*[99999999999999]int32)",
		"func f(*[65537]int8)", "func f(*[65536][65536]int8)",
		"func f(struct{data []int32})",
	} {
		if _, err := Parse(text); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
	for _, literal := range []string{
		"{1,2,3}", "{2:1}", "{-1:1}", "{n:1}", "{1.0:1}",
		"{0:1,0:2}", "{1,0:2}", "{1:1,2}", "{true}", "{2147483648}",
		"{1+2}", "[2]int32{20,22}",
	} {
		if _, err := ParseCall("f(" + literal + ":*[2]int32)"); err == nil {
			t.Fatalf("accepted %s", literal)
		}
	}
}

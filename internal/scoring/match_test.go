package scoring

import (
	"slices"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"백엔드 개발자 모집", []string{"백엔드", "개발자", "모집"}},
		{"React, Node.js", []string{"react", "node", "js"}},
		{"개발자를", []string{"개발자를"}}, // particle attached — a single token
		{"  여러   공백  ", []string{"여러", "공백"}},
		{"Spring Boot 3", []string{"spring", "boot", "3"}},
		{"", nil},
		{"!!!", nil},
	}
	for _, tc := range cases {
		if got := tokenize(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestTextContains(t *testing.T) {
	cases := []struct {
		text, phrase string
		want         bool
	}{
		{"백엔드 개발자 모집", "개발자", true},
		{"백엔드 개발자 모집", "개발", false},                  // token-exact: 개발 != 개발자
		{"백엔드 개발자를 뽑아요", "개발자", false},               // particle attached: 개발자를 != 개발자
		{"복지: 완전 재택 근무 가능", "완전 재택", true},           // contiguous multi-token phrase
		{"재택 완전 근무", "완전 재택", false},                 // tokens present but not contiguous/in order
		{"React Developer", "react developer", true}, // case-insensitive
		{"백엔드 개발자 모집", "", false},                    // empty phrase matches nothing
	}
	for _, tc := range cases {
		if got := textContains(tc.text, tc.phrase); got != tc.want {
			t.Errorf("textContains(%q, %q) = %v, want %v", tc.text, tc.phrase, got, tc.want)
		}
	}
}

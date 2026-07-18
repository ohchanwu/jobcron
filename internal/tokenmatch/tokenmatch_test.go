package tokenmatch

import (
	"slices"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"punctuation and digits", "React, 백엔드 개발자 3", []string{"react", "백엔드", "개발자", "3"}},
		{"attached particle", "개발자를", []string{"개발자를"}},
		{"NFC", "Cafe\u0301", []string{"café"}},
		{"only separators", "!!!", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Tokenize(tt.text); !slices.Equal(got, tt.want) {
				t.Fatalf("Tokenize(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name         string
		text, phrase string
		want         bool
	}{
		{"contiguous ordered phrase", "백엔드 신입 개발자", "신입 개발자", true},
		{"token exact", "개발자를 찾습니다", "개발", false},
		{"attached particle", "개발자를 찾습니다", "개발자", false},
		{"phrase order", "백엔드 신입 개발자", "개발자 신입", false},
		{"ordered but noncontiguous", "신입 경력 개발자", "신입 개발자", false},
		{"newlines", "서버\n\n개발자", "서버 개발자", true},
		{"ASCII case folding", "React Native 개발", "react native", true},
		{"digit token", "경력 3 년", "3 년", true},
		{"empty phrase", "text", "   ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.text, tt.phrase); got != tt.want {
				t.Fatalf("Contains(%q, %q) = %v, want %v", tt.text, tt.phrase, got, tt.want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		name         string
		text, phrase string
		start, end   int
		ok           bool
	}{
		{"case folded", "Lead Research projects", "research", 5, 13, true},
		{"punctuation separated", "데이터 연구-개발을 수행", "연구 개발을", len("데이터 "), len("데이터 연구-개발을"), true},
		{"NFD text", "Cafe\u0301 분석", "Café", 0, len("Cafe\u0301"), true},
		{"no match", "개발자를 찾습니다", "개발", 0, 0, false},
		{"empty phrase", "개발자를 찾습니다", "!!!", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := Find(tt.text, tt.phrase)
			if start != tt.start || end != tt.end || ok != tt.ok {
				t.Fatalf("Find(%q, %q) = (%d, %d, %v), want (%d, %d, %v)",
					tt.text, tt.phrase, start, end, ok, tt.start, tt.end, tt.ok)
			}
			if ok && tt.text[start:end] == "" {
				t.Fatal("matched span is empty")
			}
		})
	}
}

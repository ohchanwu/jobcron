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

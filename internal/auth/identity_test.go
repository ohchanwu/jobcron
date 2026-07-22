package auth

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Student@Example.COM "); got != "student@example.com" {
		t.Fatalf("NormalizeEmail = %q, want student@example.com", got)
	}
}

func TestValidateEmail(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "bare normalized address", value: "student@example.com"},
		{name: "display name", value: "Student <student@example.com>", wantErr: true},
		{name: "invalid address", value: "student@", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateEmail(%q) error = %v, wantErr %v", tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "fourteen Unicode characters", value: strings.Repeat("가", 14), wantErr: true},
		{name: "fifteen Unicode characters", value: strings.Repeat("가", 15)},
		{name: "maximum byte length", value: strings.Repeat("a", 1024)},
		{name: "over maximum byte length", value: strings.Repeat("a", 1025), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidatePassword(%d bytes) error = %v, wantErr %v", len(tc.value), err, tc.wantErr)
			}
		})
	}
}

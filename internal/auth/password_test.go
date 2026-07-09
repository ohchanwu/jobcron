package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordVerifiesOriginalPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword = false, want true")
	}
}

func TestHashPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword = true, want false")
	}
}

func TestHashPasswordEncodesParameters(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	for _, want := range []string{
		"$argon2id$",
		"v=19",
		"m=65536,t=3,p=2",
	} {
		if !strings.Contains(hash, want) {
			t.Fatalf("hash %q missing %q", hash, want)
		}
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	ok, err := VerifyPassword("$argon2id$not-enough-fields", "password")
	if err == nil {
		t.Fatal("VerifyPassword error = nil, want malformed hash error")
	}
	if ok {
		t.Fatal("VerifyPassword = true, want false")
	}
}

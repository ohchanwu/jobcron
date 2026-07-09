package auth

import (
	"encoding/base64"
	"testing"
)

func TestGenerateSessionTokenUsesAtLeast32RandomBytes(t *testing.T) {
	token, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not raw URL base64: %v", err)
	}
	if len(raw) < 32 {
		t.Fatalf("raw token length = %d, want at least 32", len(raw))
	}

	other, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("second GenerateSessionToken: %v", err)
	}
	if other == token {
		t.Fatal("two generated tokens were identical")
	}
}

func TestHashSessionTokenDoesNotExposeToken(t *testing.T) {
	token := "session-token-value"
	hash := HashSessionToken(token)
	if hash == "" {
		t.Fatal("hash is empty")
	}
	if hash == token {
		t.Fatal("hash equals raw token")
	}
	if got := HashSessionToken(token); got != hash {
		t.Fatalf("hash is not stable: got %q want %q", got, hash)
	}
}

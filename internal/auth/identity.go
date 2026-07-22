package auth

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	MinPasswordCharacters = 15
	MaxPasswordBytes      = 1024
)

// NormalizeEmail returns the canonical stored account address.
func NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// ValidateEmail accepts a bare email address, not a display-name form.
func ValidateEmail(value string) error {
	value = NormalizeEmail(value)
	address, err := mail.ParseAddress(value)
	if err != nil || address.Name != "" || address.Address != value {
		return errors.New("auth: invalid email address")
	}
	return nil
}

// ValidatePassword enforces the account password request bounds.
func ValidatePassword(value string) error {
	if len(value) > MaxPasswordBytes {
		return fmt.Errorf("auth: password exceeds %d-byte limit", MaxPasswordBytes)
	}
	if utf8.RuneCountInString(value) < MinPasswordCharacters {
		return fmt.Errorf("auth: password must be at least %d characters", MinPasswordCharacters)
	}
	return nil
}

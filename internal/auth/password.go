package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

// HashPassword hashes password using Argon2id and returns a self-describing
// encoded hash string.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("auth: password is required")
	}
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonIterations,
		argonParallelism,
		encodeBase64(salt),
		encodeBase64(key),
	), nil
}

// VerifyPassword verifies password against an encoded Argon2id hash produced
// by HashPassword.
func VerifyPassword(encodedHash, password string) (bool, error) {
	params, salt, key, err := parseEncodedHash(encodedHash)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}

type argonParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parseEncodedHash(encodedHash string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, errors.New("auth: malformed argon2id hash")
	}
	if parts[2] != "v=19" {
		return argonParams{}, nil, nil, errors.New("auth: unsupported argon2id version")
	}
	params, err := parseParams(parts[3])
	if err != nil {
		return argonParams{}, nil, nil, err
	}
	salt, err := decodeBase64(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("auth: decode password salt: %w", err)
	}
	key, err := decodeBase64(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, fmt.Errorf("auth: decode password hash: %w", err)
	}
	if len(salt) == 0 || len(key) == 0 {
		return argonParams{}, nil, nil, errors.New("auth: malformed argon2id hash")
	}
	return params, salt, key, nil
}

func parseParams(encoded string) (argonParams, error) {
	var params argonParams
	seen := map[string]bool{}
	for _, part := range strings.Split(encoded, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return argonParams{}, errors.New("auth: malformed argon2id parameters")
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return argonParams{}, fmt.Errorf("auth: malformed argon2id parameter %s: %w", key, err)
		}
		seen[key] = true
		switch key {
		case "m":
			params.memory = uint32(n)
		case "t":
			params.iterations = uint32(n)
		case "p":
			if n > 255 {
				return argonParams{}, errors.New("auth: argon2id parallelism is too large")
			}
			params.parallelism = uint8(n)
		default:
			return argonParams{}, fmt.Errorf("auth: unknown argon2id parameter %q", key)
		}
	}
	if !seen["m"] || !seen["t"] || !seen["p"] || params.memory == 0 || params.iterations == 0 || params.parallelism == 0 {
		return argonParams{}, errors.New("auth: malformed argon2id parameters")
	}
	return params, nil
}

func encodeBase64(src []byte) string {
	return base64.RawStdEncoding.EncodeToString(src)
}

func decodeBase64(src string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(src)
}

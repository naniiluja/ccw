// Package auth gates the ccw server: a password for a person and a bearer
// token for a machine. It uses only the standard library.
package auth

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MinPasswordLength is the shortest password, in characters, that the gate
// accepts. ccw holds the credentials of many providers, so a password that is
// easy to type must still be long.
const MinPasswordLength = 12

const (
	// pbkdf2Iterations is the PBKDF2-HMAC-SHA256 work factor recommended by the
	// OWASP Password Storage Cheat Sheet (600,000 iterations), see
	// https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html.
	pbkdf2Iterations = 600000
	saltBytes        = 16
	keyBytes         = 32
	hashScheme       = "pbkdf2-sha256"
)

// b64 encodes the salt and key of a stored hash. It has no "$", the field
// separator of the hash.
var b64 = base64.RawStdEncoding

// ValidatePassword reports whether a password is long enough to protect the
// stored credentials. The error never repeats the password.
func ValidatePassword(password string) error {
	if n := utf8.RuneCountInString(password); n < MinPasswordLength {
		return fmt.Errorf("the password has %d characters, want at least %d", n, MinPasswordLength)
	}
	return nil
}

// HashPassword returns a salted PBKDF2 hash of password in the form
// "pbkdf2-sha256$<iterations>$<salt>$<key>". Only this value is stored.
func HashPassword(password string) (string, error) {
	return hashPassword(password, pbkdf2Iterations)
}

func hashPassword(password string, iterations int) (string, error) {
	salt := randomBytes(saltBytes)
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyBytes)
	if err != nil {
		return "", fmt.Errorf("derive the password key: %w", err)
	}
	return hashScheme + "$" + strconv.Itoa(iterations) + "$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key), nil
}

// parsedHash is a stored hash split into its fields.
type parsedHash struct {
	iterations int
	salt, key  []byte
}

// parseHash reads a stored hash. The iteration count travels with the hash, so
// raising pbkdf2Iterations later keeps the old hashes readable.
func parseHash(encoded string) (parsedHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != hashScheme {
		return parsedHash{}, errors.New("not a " + hashScheme + " hash")
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return parsedHash{}, errors.New("the iteration count is not a positive number")
	}
	salt, err := b64.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return parsedHash{}, errors.New("the salt does not decode")
	}
	key, err := b64.DecodeString(parts[3])
	if err != nil || len(key) == 0 {
		return parsedHash{}, errors.New("the key does not decode")
	}
	return parsedHash{iterations: iterations, salt: salt, key: key}, nil
}

// verifyPassword reports whether password matches a stored hash. The keys are
// compared in constant time. A malformed hash matches nothing.
func verifyPassword(encoded, password string) bool {
	h, err := parseHash(encoded)
	if err != nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, h.salt, h.iterations, len(h.key))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(key, h.key) == 1
}

// GeneratePassword returns a random password of 24 URL-safe characters (144
// bits), for an install that has none.
func GeneratePassword() string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(18))
}

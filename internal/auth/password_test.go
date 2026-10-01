package auth

import (
	"strconv"
	"strings"
	"testing"
)

// testIterations keeps the matrix fast under -race. The iteration count lives
// in the stored hash, so a cheap hash exercises the same verify path.
const testIterations = 1000

func cheapConfig(t *testing.T, password string) *Config {
	t.Helper()
	hash, err := hashPassword(password, testIterations)
	if err != nil {
		t.Fatal(err)
	}
	return NewConfig(hash, "machine-token-abc", []byte("k"), 3600)
}

// The CheckPassword contract matrix. Equivalence classes: empty, wrong, correct,
// very long (1 KB); boundary: a 12-character password against its own
// 11-character prefix and a one-character extension.
func TestCheckPasswordMatrix(t *testing.T) {
	const twelve = "abcdefghijkl"
	long := strings.Repeat("x", 1024)
	for _, tc := range []struct {
		name   string
		stored string
		input  string
		want   bool
	}{
		{"empty input", twelve, "", false},
		{"wrong input", twelve, "abcdefghijkX", false},
		{"correct input", twelve, twelve, true},
		{"correct input with a trailing space", twelve, twelve + " ", false},
		{"different case", twelve, strings.ToUpper(twelve), false},
		{"11-character prefix of a 12-character password", twelve, twelve[:11], false},
		{"13-character extension of a 12-character password", twelve, twelve + "m", false},
		{"very long wrong input", twelve, long, false},
		{"very long correct password", long, long, true},
		{"very long password, input one byte short", long, long[:1023], false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cheapConfig(t, tc.stored).CheckPassword(tc.input); got != tc.want {
				t.Errorf("CheckPassword(%d bytes) = %v, want %v", len(tc.input), got, tc.want)
			}
		})
	}
}

// A malformed stored hash must refuse every input and never panic.
func TestCheckPasswordRefusesAMalformedHash(t *testing.T) {
	good, err := hashPassword("abcdefghijkl", testIterations)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(good, "$")
	for _, bad := range []string{
		"",
		"abcdefghijkl",
		"pbkdf2-sha256$1000$onlythree",
		"scrypt$" + strings.Join(parts[1:], "$"),
		parts[0] + "$0$" + parts[2] + "$" + parts[3],
		parts[0] + "$-5$" + parts[2] + "$" + parts[3],
		parts[0] + "$many$" + parts[2] + "$" + parts[3],
		parts[0] + "$" + parts[1] + "$!!!$" + parts[3],
		parts[0] + "$" + parts[1] + "$" + parts[2] + "$!!!",
		parts[0] + "$" + parts[1] + "$" + parts[2] + "$",
	} {
		c := NewConfig(bad, "", []byte("k"), 3600)
		if c.CheckPassword("abcdefghijkl") {
			t.Errorf("hash %q accepted a password", bad)
		}
		if _, err := FromHash(bad); err == nil {
			t.Errorf("FromHash(%q) = nil error, want a refusal at startup", bad)
		}
	}
	if _, err := FromHash(good); err != nil {
		t.Errorf("FromHash(good) = %v, want nil", err)
	}
}

// The minimum length is counted in characters, not bytes, at the 11/12 boundary.
func TestValidatePasswordBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		pw   string
		ok   bool
	}{
		{"empty", "", false},
		{"11 characters", strings.Repeat("a", 11), false},
		{"12 characters", strings.Repeat("a", 12), true},
		{"11 two-byte characters", strings.Repeat("é", 11), false},
		{"12 two-byte characters", strings.Repeat("é", 12), true},
		{"1 KB", strings.Repeat("a", 1024), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.pw)
			if (err == nil) != tc.ok {
				t.Fatalf("ValidatePassword(%q) = %v, want ok=%v", tc.pw, err, tc.ok)
			}
			if err != nil && tc.pw != "" && strings.Contains(err.Error(), tc.pw) {
				t.Errorf("the error %q repeats the password", err)
			}
		})
	}
}

// The production hash uses the OWASP iteration count, a fresh salt each time,
// and never holds the plaintext.
func TestHashPasswordStoresOnlyASaltedHash(t *testing.T) {
	const pw = "correct-horse-battery"
	a, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of one password are equal: the salt is not random")
	}
	if strings.Contains(a, pw) {
		t.Error("the hash holds the plaintext")
	}
	parts := strings.Split(a, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != strconv.Itoa(pbkdf2Iterations) {
		t.Fatalf("hash %q is not pbkdf2-sha256$%d$salt$key", a, pbkdf2Iterations)
	}
	if pbkdf2Iterations != 600000 {
		t.Errorf("pbkdf2Iterations = %d, want the OWASP 600000", pbkdf2Iterations)
	}
	if !NewConfig(a, "", nil, 3600).CheckPassword(pw) {
		t.Error("the production hash refused its own password")
	}
}

func TestGeneratePasswordIsLongAndUnique(t *testing.T) {
	a, b := GeneratePassword(), GeneratePassword()
	if err := ValidatePassword(a); err != nil {
		t.Errorf("a generated password is refused: %v", err)
	}
	if a == b {
		t.Error("two generated passwords are equal")
	}
}

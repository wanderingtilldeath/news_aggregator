package api

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Passwords are stored as "pbkdf2-sha256$iterations$salt$key": PBKDF2-HMAC-SHA256 from the standard library, with a
// random salt per password. The iteration count is in the string, so it can be raised later without breaking old
// hashes. 600,000 is the figure OWASP gives for this function.
const (
	keyLength      = 32
	maxPasswordLen = 200
	minPasswordLen = 10
)

// pbkdf2Iterations is a variable only so the tests can lower it; nothing else changes it.
var pbkdf2Iterations = 600_000

// hashSlots lets only two passwords be hashed at once, so a flood of logins queues instead of using all the CPU of a
// small server (a hash takes about a second there) and starving every other request.
var hashSlots = make(chan struct{}, 2)

func derive(password string, salt []byte, iterations int) ([]byte, error) {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()

	return pbkdf2.Key(sha256.New, password, salt, iterations, keyLength)
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := derive(password, salt, pbkdf2Iterations)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding

	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// checkPassword reports whether the password is the one the stored hash was made from. A malformed hash never matches.
func checkPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 || iterations > 10_000_000 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := derive(password, salt, iterations)

	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

var (
	dummyOnce sync.Once
	dummy     string
)

// spendTheTime does the work of checking a password against a hash that matches nothing, so a login for a username
// that does not exist takes as long as one that does and does not give the username away.
func spendTheTime(password string) {
	dummyOnce.Do(func() { dummy, _ = hashPassword("no such account") })
	checkPassword(password, dummy)
}

// recoveryAlphabet has 32 symbols, with nothing easily mistaken for another (no 0, 1, I or O).
const recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newRecoveryCode makes a 100-bit code written as four groups of five ("ABCDE-FGHJK-LMNPQ-RSTUV"). It is shown to the
// reader once; only its hash is kept.
func newRecoveryCode() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, v := range raw {
		if i > 0 && i%5 == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(recoveryAlphabet[v&31])
	}

	return b.String(), nil
}

// normalRecoveryCode makes what a reader typed comparable: upper case, without spaces or dashes.
func normalRecoveryCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

// hashRecoveryCode is the SHA-256 of a normalised code; the code has 100 bits, so a plain hash is enough.
func hashRecoveryCode(code string) string {
	return hashToken(normalRecoveryCode(code))
}

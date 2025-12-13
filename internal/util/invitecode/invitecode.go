package invitecode

import (
	"crypto/rand"
	"encoding/base32"
)

// Generate creates a random 9-character invite code using base32 uppercase letters/digits.
func Generate() (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:])
	if len(code) > 9 {
		code = code[:9]
	}
	return code, nil
}

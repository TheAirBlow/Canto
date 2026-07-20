package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// sessionTokenBytes is the raw entropy size of a session token before base64 encoding.
const sessionTokenBytes = 32

// GenerateSessionToken returns a fresh random session token for a login cookie.
func GenerateSessionToken() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

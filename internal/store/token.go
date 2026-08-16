package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrConflict = errors.New("conflict")
	ErrNotFound = errors.New("not found")
)

func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func NewToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "lgr_" + hex.EncodeToString(b[:]), nil
}

func TokenPreview(plain string) string {
	if len(plain) < 12 {
		return "lgr_…"
	}
	return fmt.Sprintf("%s…%s", plain[:8], plain[len(plain)-4:])
}

func NewClaimID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "clm_" + hex.EncodeToString(b[:]), nil
}

func ClaimIDOK(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	got := HashToken(plain)
	return subtle.ConstantTimeCompare([]byte(hash), []byte(got)) == 1
}

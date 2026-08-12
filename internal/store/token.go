package store

import (
	"crypto/rand"
	"crypto/sha256"
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

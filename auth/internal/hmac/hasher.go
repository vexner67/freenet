package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type Hasher struct {
	secret []byte
}

func NewHasher(secret []byte) *Hasher {
	return &Hasher{
		secret: secret,
	}
}

func (h *Hasher) Hash(value string) string {
	mac := hmac.New(sha256.New, h.secret)
	mac.Write([]byte(value))

	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Hasher) Compare(value string, hash string) bool {
	expected := h.Hash(value)

	return hmac.Equal([]byte(expected), []byte(hash))
}

// Package seal encrypts small secrets kept in the database (AES-256-GCM),
// so that a copy of the database alone does not give them away.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"github.com/vexner67/freenet/pkg/errs"
)

var ErrSealed = errors.New("sealed data is damaged or sealed with another key")

// Box seals and opens with one key.
type Box struct {
	aead cipher.AEAD
}

// New derives the key of purpose from secret, so one secret of the
// service serves several keys that never meet.
func New(secret, purpose string) (*Box, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("seal:" + purpose))

	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, errs.Wrap("aes.NewCipher", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errs.Wrap("cipher.NewGCM", err)
	}

	return &Box{aead: aead}, nil
}

// Seal is a random nonce and the plain text encrypted after it; label
// binds it to its owner, so a sealed value moved to another row does not
// open.
func (b *Box) Seal(plain []byte, label string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, errs.Wrap("rand.Read", err)
	}

	return b.aead.Seal(nonce, nonce, plain, []byte(label)), nil
}

// Open is what Seal sealed with the same label; ErrSealed.
func (b *Box) Open(sealed []byte, label string) ([]byte, error) {
	size := b.aead.NonceSize()
	if len(sealed) < size {
		return nil, ErrSealed
	}

	plain, err := b.aead.Open(nil, sealed[:size], sealed[size:], []byte(label))
	if err != nil {
		return nil, ErrSealed
	}

	return plain, nil
}

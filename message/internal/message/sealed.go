package message

import (
	"bytes"
	"errors"
)

// Messages of dialogs are end-to-end encrypted: the browsers of the two
// people seal what a message says, and the server keeps and passes on the
// sealed bytes it cannot read. What the server still has to know — who
// writes to whom and when, the photos, the shared post, who wrote the
// messages carried on — stays outside; the words of those carried on are
// sealed with the rest.

// KeySize is the size of a public X25519 key.
const KeySize = 32

// sealedVersion is the first byte of a sealed message: how it is sealed,
// X25519 between the two keys that follow, then AES-GCM.
const sealedVersion = 1

const (
	ivSize  = 12
	tagSize = 16
	// sealedHead is the version and the keys of the sender and of the
	// recipient, which tell the browser which of its keys opens it.
	sealedHead = 1 + 2*KeySize
	minSealed  = sealedHead + ivSize + tagSize
	// maxSealed bounds a message as the gateway takes requests of 1 MB:
	// 60 000 characters and the words of what it carries on, in base64.
	maxSealed = 640 << 10
)

var (
	ErrKeyInvalid = errors.New("invalid encryption key")
	// ErrNoKey: the user has not set up the encryption of messages yet.
	ErrNoKey         = errors.New("the user has no encryption key")
	ErrSealedInvalid = errors.New("invalid encrypted message")
	ErrSealedTooLong = errors.New("encrypted message is too long")
	// ErrPlainText: a message of a dialog says something the server could
	// read.
	ErrPlainText = errors.New("messages are end-to-end encrypted")
	// ErrKeyChanged: a message is sealed with keys that are not those of
	// its two people now; the browser takes the new ones and seals again.
	ErrKeyChanged = errors.New("the encryption keys changed")
	// ErrForwardIntoChat: words sealed for two would be put in plain text
	// for everyone in a chat.
	ErrForwardIntoChat = errors.New("encrypted messages cannot be forwarded into a chat")
	// ErrSealedInChat: a chat of several people is not encrypted.
	ErrSealedInChat = errors.New("messages of chats are not encrypted")
)

// Key is the public key a user's browsers seal and open messages with:
// 32 bytes of X25519. The private one never leaves them.
type Key string

func NewKey(b []byte) (Key, error) {
	if len(b) != KeySize || bytes.Equal(b, make([]byte, KeySize)) {
		return "", ErrKeyInvalid
	}

	return Key(b), nil
}

func (k Key) Bytes() []byte {
	return []byte(k)
}

// Sealed is what a message of a dialog says, sealed by the sender's
// browser: the version, the keys of the sender and of the recipient, the
// IV and AES-GCM's ciphertext with its tag. "" is none: a message of
// photos alone says nothing to seal.
type Sealed string

func NewSealed(b []byte) (Sealed, error) {
	if len(b) == 0 {
		return "", nil
	}

	if len(b) > maxSealed {
		return "", ErrSealedTooLong
	}

	if len(b) < minSealed || b[0] != sealedVersion {
		return "", ErrSealedInvalid
	}

	s := Sealed(b)
	if _, err := NewKey(s.SenderKey().Bytes()); err != nil {
		return "", ErrSealedInvalid
	}

	if _, err := NewKey(s.RecipientKey().Bytes()); err != nil {
		return "", ErrSealedInvalid
	}

	return s, nil
}

func (s Sealed) Bytes() []byte {
	if s == "" {
		return nil
	}

	return []byte(s)
}

// SenderKey and RecipientKey are the keys s is sealed between; "" for
// none.
func (s Sealed) SenderKey() Key {
	if s == "" {
		return ""
	}

	return Key(s[1 : 1+KeySize])
}

func (s Sealed) RecipientKey() Key {
	if s == "" {
		return ""
	}

	return Key(s[1+KeySize : sealedHead])
}

// CheckKeys tells whether s is sealed between the keys the sender and the
// recipient have now: a browser that sealed with an old one would leave
// the other side a message it cannot open. "" for a key is a user without
// one.
func (s Sealed) CheckKeys(sender, recipient Key) error {
	if s == "" {
		return nil
	}

	if sender == "" || recipient == "" || s.SenderKey() != sender || s.RecipientKey() != recipient {
		return ErrKeyChanged
	}

	return nil
}

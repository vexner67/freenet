package twofactor

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	// RecoveryCodes are given at once, as openvk's ten.
	RecoveryCodes = 10
	// recoveryLen letters of recoveryAlphabet: 50 bits, shown as two
	// groups of five.
	recoveryLen = 10
	// recoveryAlphabet has no 0, 1, l and o, which people confuse.
	recoveryAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"
)

// NewRecoveryCodes are RecoveryCodes fresh codes, each good once, as
// "abcde-fghij".
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, RecoveryCodes)
	for i := range codes {
		raw := make([]byte, recoveryLen)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("rand.Read: %w", err)
		}

		// 256 is a multiple of the 32 letters: every one is as likely.
		for j, b := range raw {
			raw[j] = recoveryAlphabet[int(b)%len(recoveryAlphabet)]
		}

		codes[i] = string(raw[:recoveryLen/2]) + "-" + string(raw[recoveryLen/2:])
	}

	return codes, nil
}

// IsRecoveryCode tells a recovery code, as Normalize leaves it, from a
// code of the app.
func IsRecoveryCode(code string) bool {
	if len(code) != recoveryLen {
		return false
	}

	for _, r := range code {
		if !strings.ContainsRune(recoveryAlphabet, r) {
			return false
		}
	}

	return true
}

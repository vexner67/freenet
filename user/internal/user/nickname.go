package user

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

const maxNicknameLen = 32

var (
	ErrNicknameEmpty   = errors.New("nickname is empty")
	ErrNicknameTooLong = errors.New("nickname is too long")
	ErrNicknameInvalid = errors.New("nickname contains invalid characters")

	nicknamePattern = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9_]*[A-Za-z0-9])?$`)
)

type Nickname string

func NewNickname(s string) (Nickname, error) {
	if s == "" {
		return "", ErrNicknameEmpty
	}

	if utf8.RuneCountInString(s) > maxNicknameLen {
		return "", ErrNicknameTooLong
	}

	if !nicknamePattern.MatchString(s) {
		return "", ErrNicknameInvalid
	}

	return Nickname(s), nil
}

func (n Nickname) String() string {
	return string(n)
}

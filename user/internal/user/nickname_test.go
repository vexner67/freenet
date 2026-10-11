package user_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewNickname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		err   error
	}{
		{"ann", nil},
		{"Ann_Lee2", nil},
		{"editor", nil},
		{"", user.ErrNicknameEmpty},
		{strings.Repeat("a", 33), user.ErrNicknameTooLong},
		{"2ann", user.ErrNicknameInvalid},
		{"ann_", user.ErrNicknameInvalid},
		{"анна", user.ErrNicknameInvalid},
		{"edit", user.ErrNicknameReserved},
		{"API", user.ErrNicknameReserved},
		{"Static", user.ErrNicknameReserved},
		{"feed", user.ErrNicknameReserved},
		{"Post", user.ErrNicknameReserved},
		{"notifications", user.ErrNicknameReserved},
		{"notes", user.ErrNicknameReserved},
		{"fave", user.ErrNicknameReserved},
		{"note", user.ErrNicknameReserved},
		{"topic", user.ErrNicknameReserved},
		{"wall", user.ErrNicknameReserved},
		{"Albums", user.ErrNicknameReserved},
		{"album", user.ErrNicknameReserved},
		{"photo", user.ErrNicknameReserved},
		{"IM", user.ErrNicknameReserved},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			if _, err := user.NewNickname(tt.input); !errors.Is(err, tt.err) {
				t.Fatalf("NewNickname(%q) = %v, want %v", tt.input, err, tt.err)
			}
		})
	}
}

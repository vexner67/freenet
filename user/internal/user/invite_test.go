package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_CheckInvitedAt(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	registered := user.RegisteredAt(u.ID())

	tests := []struct {
		name string
		now  time.Time
		want error
	}{
		{"just signed up", registered.Add(time.Minute), nil},
		{"on the last day", registered.Add(user.InviteFor), nil},
		{"too late", registered.Add(user.InviteFor + time.Second), user.ErrInviteExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := u.CheckInvitedAt(tt.now); !errors.Is(err, tt.want) {
				t.Fatalf("CheckInvitedAt = %v, want %v", err, tt.want)
			}
		})
	}

}

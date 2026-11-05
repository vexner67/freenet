package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewFriendSection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    user.FriendSection
		ownOnly bool
		err     error
	}{
		{name: "empty is all", in: "", want: user.FriendsAll},
		{name: "online", in: "online", want: user.FriendsOnline},
		{name: "common", in: "common", want: user.FriendsCommon},
		{name: "requests", in: "requests", want: user.FriendRequests, ownOnly: true},
		{name: "followers", in: "followers", want: user.FriendFollowers, ownOnly: true},
		{name: "outgoing", in: "outgoing", want: user.FriendsOutgoing, ownOnly: true},
		{name: "unknown", in: "following", err: user.ErrInvalidFriendSection},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewFriendSection(tt.in)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}

			if got != tt.want || got.OwnOnly() != tt.ownOnly {
				t.Fatalf("got %q (own only %v), want %q (%v)", got, got.OwnOnly(), tt.want, tt.ownOnly)
			}
		})
	}
}

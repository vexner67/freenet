package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_CanBeMentionedBy(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	author := uuid.New()

	if !u.CanBeMentionedBy(author, user.Ties{}) {
		t.Fatal("anyone may name ann")
	}
	if u.CanBeMentionedBy(u.ID(), user.Ties{}) {
		t.Fatal("nobody hears of naming themselves")
	}
	if u.CanBeMentionedBy(author, user.Ties{Blocked: true, Followed: true}) {
		t.Fatal("a block shuts mentions")
	}

	if err = u.Deactivate("", time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if u.CanBeMentionedBy(author, user.Ties{}) {
		t.Fatal("a deleted page hears nothing")
	}
}

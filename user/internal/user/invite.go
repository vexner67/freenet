package user

import (
	"errors"
	"time"
)

// InviteFor is how long after signing up a page may name who invited it
// ("Пригласить" of openvk): the link is followed to sign up, so an old
// page saying it was invited would only make up a notification.
const InviteFor = 7 * 24 * time.Hour

var (
	// ErrInviteExpired: the page is older than InviteFor.
	ErrInviteExpired = errors.New("only new pages accept invitations")
	// ErrAlreadyInvited: the page has named who invited it before.
	ErrAlreadyInvited = errors.New("an invitation is accepted already")
)

// CheckInvitedAt tells whether u may still name who invited it at now.
func (u *User) CheckInvitedAt(now time.Time) error {
	if now.Sub(RegisteredAt(u.id)) > InviteFor {
		return ErrInviteExpired
	}

	return nil
}

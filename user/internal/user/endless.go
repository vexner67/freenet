package user

import (
	"slices"
	"strings"
)

// EndlessVotes are what the administrators of the site have: a balance
// nothing on the site comes near, kept so (see Admins.Emails), so they can
// pay for anything. The site shows a balance from EndlessFloor up as "∞",
// and the treasury does not count one in the votes on the accounts.
const (
	EndlessVotes int64 = 1_000_000_000_000_000
	EndlessFloor int64 = 100_000_000_000_000
)

// IsEndless tells whether votes are the endless balance of an
// administrator.
func IsEndless(votes int64) bool {
	return votes >= EndlessFloor
}

// Emails are the emails of the administrators, in lower case, sorted.
func (a Admins) Emails() []string {
	emails := make([]string, 0, len(a))
	for e := range a {
		emails = append(emails, strings.ToLower(e.String()))
	}
	slices.Sort(emails)

	return emails
}

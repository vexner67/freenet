package user

import "github.com/google/uuid"

// MaxMentions is how many people one text names for them to hear of it;
// the rest are links and no more.
const MaxMentions = 10

// MaxKnownNicknames is how many nicknames are looked up at once to tell
// which of them are someone's.
const MaxKnownNicknames = 100

// KnownNickname is a nickname that is someone's, as its owner writes it,
// and whether that page wears the checkmark: a mention shows it too.
type KnownNickname struct {
	Nickname Nickname
	Verified bool
}

// CanBeMentionedBy tells whether u hears of being named by authorID in a
// post or a comment ("Вас упомянул пользователь" of openvk): not of their
// own words, not across a block either way, and not on a deleted page.
func (u *User) CanBeMentionedBy(authorID uuid.UUID, t Ties) bool {
	return authorID != u.id && !t.Blocked && !u.IsDeactivated()
}

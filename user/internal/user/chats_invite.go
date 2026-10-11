package user

import "github.com/google/uuid"

// DefaultChatsInvite lets the people a user follows add them to group
// chats ("Кто может добавлять меня в чаты" of openvk), so that a stranger
// does not pull a new page into one.
const DefaultChatsInvite = MessagesWriteFollowing

// MaxInvited is how many people are looked at when a chat is made or
// grows: a chat holds 50 with the one who invites.
const MaxInvited = 49

// CanBeInvitedBy tells whether inviterID may add u to a group chat: not
// themselves, not across a block either way and not a deleted page;
// otherwise u's setting decides.
func (u *User) CanBeInvitedBy(inviterID uuid.UUID, t Ties) bool {
	if inviterID == u.id || t.Blocked || u.IsDeactivated() {
		return false
	}

	switch u.privacy.ChatsInvite {
	case MessagesWriteEveryone:
		return true
	case MessagesWriteFollowing:
		return t.Followed
	default:
		return false
	}
}

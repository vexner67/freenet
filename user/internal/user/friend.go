package user

import "errors"

// FriendSection is one list of openvk's "Друзья". Friends follow each
// other: a follow not followed back is a friend request ("Добавить в
// друзья"), and following back accepts it. The lists are the friends,
// those online and those the viewer is friends with too; and, of one's
// own page only, the requests (followers not followed back nor left among
// the followers), the followers (everyone not followed back) and the
// outgoing requests, whom one follows without being followed back.
type FriendSection string

const (
	FriendsAll      FriendSection = "all"
	FriendsOnline   FriendSection = "online"
	FriendsCommon   FriendSection = "common"
	FriendRequests  FriendSection = "requests"
	FriendFollowers FriendSection = "followers"
	FriendsOutgoing FriendSection = "outgoing"
)

var (
	ErrInvalidFriendSection = errors.New("invalid friends section")
	// ErrNotYourRequests: the requests, the followers and the outgoing
	// requests of a page are its owner's only.
	ErrNotYourRequests = errors.New("only the owner sees the friend requests")
	// ErrNoFriendRequest: there is no request of that user to decline.
	ErrNoFriendRequest = errors.New("no friend request of this user")
)

// NewFriendSection reads a section; empty is all the friends.
func NewFriendSection(s string) (FriendSection, error) {
	switch FriendSection(s) {
	case "":
		return FriendsAll, nil
	case FriendsAll, FriendsOnline, FriendsCommon, FriendRequests, FriendFollowers, FriendsOutgoing:
		return FriendSection(s), nil
	default:
		return "", ErrInvalidFriendSection
	}
}

// OwnOnly tells whether only the owner of the page may read the section.
func (s FriendSection) OwnOnly() bool {
	return s == FriendRequests || s == FriendFollowers || s == FriendsOutgoing
}

func (s FriendSection) String() string {
	return string(s)
}

package room

type Access string

const (
	AccessEveryone Access = "everyone"
	AccessFriends  Access = "friends"
	AccessLink     Access = "link"
	AccessClub     Access = "club"
)

func NewAccess(s string) (Access, error) {
	switch Access(s) {
	case AccessEveryone, AccessFriends, AccessLink, AccessClub:
		return Access(s), nil
	}
	return "", ErrInvalidAccess
}
func (a Access) String() string { return string(a) }

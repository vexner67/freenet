package user

import "errors"

var (
	ErrNotFound      = errors.New("user not found")
	ErrAlreadyExists = errors.New("user already exists")
	ErrNicknameTaken = errors.New("nickname is already taken")
	ErrFollowSelf    = errors.New("you cannot follow yourself")
	// ErrFollowBlocked: either of the two has blacklisted the other.
	ErrFollowBlocked = errors.New("you cannot follow this user")
	ErrBlockSelf     = errors.New("you cannot blacklist yourself")
	ErrPartnerIsSelf = errors.New("you cannot name yourself in your relationship")
	// ErrPartnerNotFound: the nickname of the relationship names nobody.
	ErrPartnerNotFound = errors.New("partner not found")
)

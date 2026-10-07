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
)

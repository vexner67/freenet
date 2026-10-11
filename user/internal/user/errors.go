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
	// ErrIgnoreSelf: one's own posts are always in one's news.
	ErrIgnoreSelf = errors.New("you cannot ignore yourself")
	// ErrTooManyIgnored: MaxIgnoredSources are ignored already.
	ErrTooManyIgnored = errors.New("too many ignored sources")
	ErrPartnerIsSelf  = errors.New("you cannot name yourself in your relationship")
	// ErrPartnerNotFound: the nickname of the relationship names nobody.
	ErrPartnerNotFound = errors.New("partner not found")
)

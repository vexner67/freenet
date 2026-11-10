package room

import "errors"

var (
	ErrUnavailable     = errors.New("room service dependency unavailable")
	ErrInvalidID       = errors.New("invalid room identifier")
	ErrInvalidTitle    = errors.New("invalid room title")
	ErrInvalidKind     = errors.New("invalid room kind")
	ErrInvalidAccess   = errors.New("invalid room access")
	ErrInvalidControl  = errors.New("invalid room control")
	ErrInvalidPlayback = errors.New("invalid room playback")
	ErrInvalidChat     = errors.New("invalid room chat")
	ErrNotFound        = errors.New("room not found")
	ErrFull            = errors.New("room is full")
	ErrClosed          = errors.New("room is closed")
	ErrForbidden       = errors.New("room access denied")
	ErrKicked          = errors.New("removed from room")
	ErrTooManyRooms    = errors.New("too many rooms")
	ErrStalePlayback   = errors.New("room playback has changed")
	ErrNotMember       = errors.New("not in room")
)

// Package failure classifies errors the gateway returns to clients.
package failure

import "errors"

type Kind int

const (
	Internal Kind = iota
	InvalidArgument
	Unauthenticated
	PermissionDenied
	NotFound
	Conflict
	TooManyRequests
	Unavailable
	MethodNotAllowed
)

// Error is a failure whose Message is safe to show to the client.
type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func New(kind Kind, message string) error {
	return &Error{Kind: kind, Message: message}
}

// From finds the failure in err's chain; anything else is Internal with
// a generic message, so unexpected details never reach the client.
func From(err error) *Error {
	var f *Error
	if errors.As(err, &f) {
		return f
	}

	return &Error{Kind: Internal, Message: "internal error"}
}

// Package backend calls the internal gRPC services on behalf of the API.
package backend

import (
	"github.com/vexner67/freenet/gateway/internal/failure"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fromStatus turns a service status into a failure. Messages of known codes
// were chosen by the services for clients and pass through; transport-level
// codes get fixed messages, since grpc-go fills them with internal addresses.
func fromStatus(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch st.Code() {
	case codes.InvalidArgument:
		return failure.New(failure.InvalidArgument, st.Message())
	case codes.Unauthenticated:
		return failure.New(failure.Unauthenticated, st.Message())
	case codes.PermissionDenied:
		return failure.New(failure.PermissionDenied, "permission denied")
	case codes.NotFound:
		return failure.New(failure.NotFound, st.Message())
	case codes.AlreadyExists, codes.FailedPrecondition:
		return failure.New(failure.Conflict, st.Message())
	case codes.ResourceExhausted:
		return failure.New(failure.TooManyRequests, st.Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return failure.New(failure.Unavailable, "service temporarily unavailable")
	default:
		// Internal and the rest: logged by the caller, generic for the client.
		return err
	}
}

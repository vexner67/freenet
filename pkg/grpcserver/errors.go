package grpcserver

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Translator maps an error a handler returned to the status the client
// should see. It reports false for errors it does not know.
type Translator func(err error) (*status.Status, bool)

// ErrorInterceptor converts handler errors into statuses. Handlers return
// plain errors; clients see only messages chosen by translate, everything
// unknown becomes Internal and is logged with its full chain.
func ErrorInterceptor(logger *slog.Logger, translate Translator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		return nil, toStatusError(ctx, logger, translate, info.FullMethod, err)
	}
}

func toStatusError(
	ctx context.Context,
	logger *slog.Logger,
	translate Translator,
	method string,
	err error,
) error {
	// Statuses created by handlers or interceptors are already meant for
	// the client. Deliberately no errors.As: a status wrapped by errs.Wrap
	// comes from a downstream call and must not leak.
	if _, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
		return err
	}

	if st, ok := translate(err); ok {
		if st.Code() == codes.Unavailable {
			logger.WarnContext(ctx, "dependency unavailable", "method", method, "error", err)
		}

		return st.Err()
	}

	logger.ErrorContext(ctx, "request failed", "method", method, "error", err)

	return status.Error(codes.Internal, "internal error")
}

package reqid

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryServerInterceptor takes the caller's ID from the metadata, or makes
// one for a call that came without: every log line of the call has one.
func UnaryServerInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	return handler(fromMetadata(ctx), req)
}

// StreamServerInterceptor is UnaryServerInterceptor for streams.
func StreamServerInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return handler(srv, &stream{ServerStream: ss, ctx: fromMetadata(ss.Context())})
}

// DialOptions pass the ID of the context on to the services called.
func DialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(unaryClient),
		grpc.WithChainStreamInterceptor(streamClient),
	}
}

func unaryClient(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(toMetadata(ctx), method, req, reply, cc, opts...)
}

func streamClient(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return streamer(toMetadata(ctx), desc, cc, method, opts...)
}

func fromMetadata(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)

	if ids := md.Get(metadataKey); len(ids) == 1 && Valid(ids[0]) {
		return ContextWithID(ctx, ids[0])
	}

	return ContextWithID(ctx, New())
}

func toMetadata(ctx context.Context) context.Context {
	// The server would make a new ID anyway; an invalid one may not even
	// be sendable as metadata.
	id := FromContext(ctx)
	if !Valid(id) {
		return ctx
	}

	if md, _ := metadata.FromOutgoingContext(ctx); len(md.Get(metadataKey)) > 0 {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, metadataKey, id)
}

type stream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stream) Context() context.Context {
	return s.ctx
}

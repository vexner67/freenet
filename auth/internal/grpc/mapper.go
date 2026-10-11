package grpc

import (
	"github.com/google/uuid"
	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoTokenPair(tokenPair service.TokenPair) *authv1.TokenPair {
	return &authv1.TokenPair{
		AccessToken:  tokenPair.Access,
		RefreshToken: tokenPair.Refresh,
	}
}

// toProtoSessions marks the session the caller asks from. The refresh
// token hash never leaves auth.
func toProtoSessions(sessions []*session.Session, current uuid.UUID) []*authv1.Session {
	out := make([]*authv1.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, &authv1.Session{
			Id:         s.ID().String(),
			CreatedAt:  timestamppb.New(s.CreatedAt()),
			LastUsedAt: timestamppb.New(s.UsedAt()),
			Ip:         s.Device().IP(),
			UserAgent:  s.Device().Agent(),
			Current:    s.ID() == current,
		})
	}

	return out
}

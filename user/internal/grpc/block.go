package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/emptypb"
)

type BlockService interface {
	Block(ctx context.Context, blockerID, blockedID uuid.UUID) error
	Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error
	List(ctx context.Context, blockerID, cursor uuid.UUID, size int) (service.BlockPage, error)
	BlockedIDs(ctx context.Context, blockerID uuid.UUID) ([]uuid.UUID, error)
	Relation(ctx context.Context, viewerID, userID uuid.UUID) (service.Relation, error)
}

type PermissionService interface {
	CanPostOnWall(ctx context.Context, authorID, ownerID uuid.UUID) (bool, error)
	CanMessage(ctx context.Context, senderID, recipientID uuid.UUID, wroteFirst bool) (bool, error)
	Access(ctx context.Context, viewerID, ownerID uuid.UUID) (user.Access, error)
	ProfileFor(ctx context.Context, viewerID uuid.UUID, nickname user.Nickname) (user.PublicProfile, error)
	Mentioned(ctx context.Context, authorID uuid.UUID, nicknames []user.Nickname) ([]uuid.UUID, error)
}

// callerAnd is the caller from the token and the user the request names.
func callerAnd(ctx context.Context, userID string) (uuid.UUID, uuid.UUID, error) {
	callerID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	otherID, err := parseID(userID, "user id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return callerID, otherID, nil
}

func (h *UserHandler) CanPostOnWall(
	ctx context.Context,
	req *userv1.CanPostOnWallRequest,
) (*userv1.CanPostOnWallResponse, error) {
	authorID, ownerID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	allowed, err := h.permissionService.CanPostOnWall(ctx, authorID, ownerID)
	if err != nil {
		return nil, err
	}

	return &userv1.CanPostOnWallResponse{Allowed: allowed}, nil
}

func (h *UserHandler) GetAccess(ctx context.Context, req *userv1.GetAccessRequest) (*userv1.GetAccessResponse, error) {
	ownerID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	access, err := h.permissionService.Access(ctx, viewerID(ctx), ownerID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetAccessResponse{
		Page: access.Page, Info: access.Info, Photos: access.Photos, Friends: access.Friends, Clubs: access.Clubs,
	}, nil
}

// viewerID is the caller of a public method: known only if a token was
// sent, uuid.Nil otherwise.
func viewerID(ctx context.Context) uuid.UUID {
	if identity, ok := jwtauth.IdentityFromContext(ctx); ok {
		return identity.UserID
	}

	return uuid.Nil
}

func (h *UserHandler) CanMessage(ctx context.Context, req *userv1.CanMessageRequest) (*userv1.CanMessageResponse, error) {
	senderID, recipientID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	allowed, err := h.permissionService.CanMessage(ctx, senderID, recipientID, req.GetWroteFirst())
	if err != nil {
		return nil, err
	}

	return &userv1.CanMessageResponse{Allowed: allowed}, nil
}

func (h *UserHandler) ResolveMentions(ctx context.Context, req *userv1.ResolveMentionsRequest) (*userv1.ResolveMentionsResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	// A text may say "@anything": what is not a nickname names nobody.
	var nicknames []user.Nickname
	for _, s := range req.GetNicknames() {
		if nickname, err := user.NewNickname(s); err == nil {
			nicknames = append(nicknames, nickname)
		}
	}

	mentioned, err := h.permissionService.Mentioned(ctx, authorID, nicknames)
	if err != nil {
		return nil, err
	}

	resp := &userv1.ResolveMentionsResponse{UserIds: make([]string, 0, len(mentioned))}
	for _, id := range mentioned {
		resp.UserIds = append(resp.UserIds, id.String())
	}

	return resp, nil
}

// FindNicknames tells which of the nicknames a text says are someone's.
func (h *UserHandler) FindNicknames(ctx context.Context, req *userv1.FindNicknamesRequest) (*userv1.FindNicknamesResponse, error) {
	var nicknames []user.Nickname
	for _, s := range req.GetNicknames() {
		if nickname, err := user.NewNickname(s); err == nil {
			nicknames = append(nicknames, nickname)
		}
	}

	known, err := h.userService.KnownNicknames(ctx, nicknames)
	if err != nil {
		return nil, err
	}

	resp := &userv1.FindNicknamesResponse{Nicknames: make([]string, 0, len(known))}
	for _, nickname := range known {
		resp.Nicknames = append(resp.Nicknames, nickname.String())
	}

	return resp, nil
}

func (h *UserHandler) Block(ctx context.Context, req *userv1.BlockRequest) (*emptypb.Empty, error) {
	blockerID, blockedID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err = h.blockService.Block(ctx, blockerID, blockedID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) Unblock(ctx context.Context, req *userv1.UnblockRequest) (*emptypb.Empty, error) {
	blockerID, blockedID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	if err = h.blockService.Unblock(ctx, blockerID, blockedID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) ListBlockedIDs(
	ctx context.Context,
	_ *userv1.ListBlockedIDsRequest,
) (*userv1.ListBlockedIDsResponse, error) {
	blockerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ids, err := h.blockService.BlockedIDs(ctx, blockerID)
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListBlockedIDsResponse{UserIds: make([]string, len(ids))}
	for i, id := range ids {
		resp.UserIds[i] = id.String()
	}

	return resp, nil
}

func (h *UserHandler) ListBlocked(ctx context.Context, req *userv1.ListBlockedRequest) (*userv1.ListBlockedResponse, error) {
	blockerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var cursor uuid.UUID
	if req.GetCursor() != "" {
		if cursor, err = parseID(req.GetCursor(), "cursor"); err != nil {
			return nil, err
		}
	}

	page, err := h.blockService.List(ctx, blockerID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListBlockedResponse{
		Users: make([]*userv1.PublicProfile, 0, len(page.Users)),
		Total: int32(page.Total), //nolint:gosec // counts of people fit in int32
	}

	for _, p := range page.Users {
		resp.Users = append(resp.Users, toProtoPublicProfile(p))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *UserHandler) GetRelation(ctx context.Context, req *userv1.GetRelationRequest) (*userv1.GetRelationResponse, error) {
	viewerID, userID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	rel, err := h.blockService.Relation(ctx, viewerID, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetRelationResponse{BlockedByMe: rel.BlockedByViewer, BlockedMe: rel.BlockedViewer}, nil
}

package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserService interface {
	GetOrCreateByEmail(ctx context.Context, email user.Email) (uuid.UUID, error)
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, profile user.Profile, partner user.Nickname) (*user.User, error)
	UpdateContacts(ctx context.Context, id uuid.UUID, contacts user.Contacts) (*user.User, error)
	UpdateInterests(ctx context.Context, id uuid.UUID, interests user.Interests) (*user.User, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status user.Status) (*user.User, error)
	UpdateNickname(ctx context.Context, id uuid.UUID, nickname user.Nickname) (*user.User, error)
	UpdateInterface(ctx context.Context, id uuid.UUID, ui user.Interface) (*user.User, error)
	UpdateExtraFields(ctx context.Context, id uuid.UUID, fields []user.ExtraField) (*user.User, error)
	UpdatePrivacy(ctx context.Context, id uuid.UUID, privacy user.Privacy) (*user.User, error)
	GetPublicProfiles(ctx context.Context, ids []uuid.UUID) ([]user.PublicProfile, error)
	SetAvatar(ctx context.Context, id, avatarID uuid.UUID) (*user.User, error)
	Search(ctx context.Context, q user.SearchQuery, cursor uuid.UUID, size int) (service.SearchPage, error)
	KnownNicknames(ctx context.Context, nicknames []user.Nickname) ([]user.KnownNickname, error)
}

type UserHandler struct {
	userv1.UnimplementedUserServiceServer
	userService       UserService
	accountService    AccountService
	followService     FollowService
	blockService      BlockService
	ignoreService     IgnoreService
	giftService       GiftService
	stickerService    StickerService
	walletService     WalletService
	verification      VerificationService
	bans              BanService
	permissionService PermissionService
	presenceService   PresenceService
	backdropService   BackdropService
	logger            *slog.Logger
}

// Services are the use cases behind the handler.
type Services struct {
	Users    UserService
	Accounts AccountService
	Follows  FollowService
	Blocks   BlockService
	Ignores  IgnoreService
	Gifts    GiftService
	Stickers StickerService
	Wallets  WalletService
	// Verification gives and takes the checkmark.
	Verification VerificationService
	// Bans block pages and let them be.
	Bans        BanService
	Permissions PermissionService
	Presence    PresenceService
	Backdrops   BackdropService
}

func NewUserHandler(s Services, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		userService:       s.Users,
		accountService:    s.Accounts,
		followService:     s.Follows,
		blockService:      s.Blocks,
		ignoreService:     s.Ignores,
		giftService:       s.Gifts,
		stickerService:    s.Stickers,
		walletService:     s.Wallets,
		verification:      s.Verification,
		bans:              s.Bans,
		permissionService: s.Permissions,
		presenceService:   s.Presence,
		backdropService:   s.Backdrops,
		logger:            logger,
	}
}

func (h *UserHandler) GetOrCreateByEmail(
	ctx context.Context,
	req *userv1.GetOrCreateByEmailRequest,
) (*userv1.GetOrCreateByEmailResponse, error) {
	email, err := user.NewEmail(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	id, err := h.userService.GetOrCreateByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// For the access tokens auth issues: a deleted or a blocked page gets
	// restricted ones.
	state, err := h.accountService.State(ctx, id)
	if err != nil {
		return nil, err
	}

	return &userv1.GetOrCreateByEmailResponse{UserId: id.String(), Deactivated: state.Deactivated, Banned: state.Banned}, nil
}

func (h *UserHandler) GetMe(ctx context.Context, _ *userv1.GetMeRequest) (*userv1.GetMeResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	u, err := h.userService.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	me := toProtoUser(u)
	me.Admin = h.walletService.IsAdmin(u)

	return &userv1.GetMeResponse{User: me}, nil
}

func (h *UserHandler) UpdateProfile(
	ctx context.Context,
	req *userv1.UpdateProfileRequest,
) (*userv1.UpdateProfileResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := fromProtoProfile(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var partner user.Nickname
	if req.GetRelationshipPartner() != "" {
		// A nickname of a wrong form names nobody either.
		if partner, err = user.NewNickname(req.GetRelationshipPartner()); err != nil {
			return nil, user.ErrPartnerNotFound
		}
	}

	u, err := h.userService.UpdateProfile(ctx, userID, profile, partner)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateProfileResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) UpdateContacts(ctx context.Context, req *userv1.UpdateContactsRequest) (*userv1.UpdateContactsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	contacts, err := fromProtoContacts(req.GetContacts())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdateContacts(ctx, userID, contacts)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateContactsResponse{User: toProtoUser(u)}, nil
}

// UpdateInterface is "Интерфейс" of the settings.
func (h *UserHandler) UpdateInterface(ctx context.Context, req *userv1.UpdateInterfaceRequest) (*userv1.UpdateInterfaceResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ui, err := user.NewInterface(fromProtoInterface(req.GetInterface()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdateInterface(ctx, userID, ui)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateInterfaceResponse{User: toProtoUser(u)}, nil
}

// UpdateNickname is the address of the page alone: the settings change it
// without sending the rest of the profile.
func (h *UserHandler) UpdateNickname(ctx context.Context, req *userv1.UpdateNicknameRequest) (*userv1.UpdateNicknameResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	nickname, err := user.NewNickname(req.GetNickname())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdateNickname(ctx, userID, nickname)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateNicknameResponse{User: toProtoUser(u)}, nil
}

// UpdateStatus is the status alone, as openvk edits it on the page.
func (h *UserHandler) UpdateStatus(ctx context.Context, req *userv1.UpdateStatusRequest) (*userv1.UpdateStatusResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := user.NewStatus(req.GetStatus())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdateStatus(ctx, userID, s)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateStatusResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) UpdateInterests(ctx context.Context, req *userv1.UpdateInterestsRequest) (*userv1.UpdateInterestsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	interests, err := fromProtoInterests(req.GetInterests())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdateInterests(ctx, userID, interests)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateInterestsResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) UpdateExtraFields(ctx context.Context, req *userv1.UpdateExtraFieldsRequest) (*userv1.UpdateExtraFieldsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	fields, err := fromProtoExtraFields(req.GetFields())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdateExtraFields(ctx, userID, fields)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateExtraFieldsResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) UpdatePrivacy(
	ctx context.Context,
	req *userv1.UpdatePrivacyRequest,
) (*userv1.UpdatePrivacyResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	privacy, err := fromProtoPrivacy(req.GetPrivacy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	u, err := h.userService.UpdatePrivacy(ctx, userID, privacy)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdatePrivacyResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) GetProfile(
	ctx context.Context,
	req *userv1.GetProfileRequest,
) (*userv1.GetProfileResponse, error) {
	nickname, err := user.NewNickname(req.GetNickname())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	profile, err := h.permissionService.ProfileFor(ctx, viewerID(ctx), nickname)
	if err != nil {
		return nil, err
	}

	return &userv1.GetProfileResponse{Profile: toProtoPublicProfile(profile)}, nil
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}

func (h *UserHandler) SetAvatar(ctx context.Context, req *userv1.SetAvatarRequest) (*userv1.SetAvatarResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var avatarID uuid.UUID
	if req.GetAvatarId() != "" {
		if avatarID, err = parseID(req.GetAvatarId(), "avatar id"); err != nil {
			return nil, err
		}
	}

	u, err := h.userService.SetAvatar(ctx, userID, avatarID)
	if err != nil {
		return nil, err
	}

	return &userv1.SetAvatarResponse{User: toProtoUser(u)}, nil
}

func (h *UserHandler) SearchUsers(ctx context.Context, req *userv1.SearchUsersRequest) (*userv1.SearchUsersResponse, error) {
	q, err := user.NewSearchQuery(req.GetQuery(), user.SearchFilters{
		Hometown: req.GetHometown(), City: req.GetCity(), Film: req.GetFilm(), Show: req.GetShow(), Book: req.GetBook(),
		Verified: req.GetVerified(), Banned: req.GetBanned(),
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var cursor uuid.UUID
	if req.GetCursor() != "" {
		if cursor, err = parseID(req.GetCursor(), "cursor"); err != nil {
			return nil, err
		}
	}

	page, err := h.userService.Search(ctx, q, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &userv1.SearchUsersResponse{Users: make([]*userv1.PublicProfile, 0, len(page.Users))}
	for _, p := range page.Users {
		resp.Users = append(resp.Users, toProtoPublicProfile(p))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

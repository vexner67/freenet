package grpc

import (
	"context"
	"log/slog"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserService interface {
	Create(ctx context.Context, input service.CreateUserInput) (*user.User, error)
}

type UserHandler struct {
	userv1.UnimplementedUserServiceServer
	userService UserService
	logger      *slog.Logger
}

func NewUserHandler(userService UserService, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

func (h *UserHandler) Create(ctx context.Context, req *userv1.CreateRequest) (*userv1.User, error) {
	name, err := user.NewName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	surname, err := user.NewSurname(req.Surname)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	nickname, err := user.NewNickname(req.Nickname)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	st, err := user.NewStatus(req.Status)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	gender, err := user.NewGender(
		fromProtoGender(req.Gender),
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	relationship, err := user.NewRelationship(
		fromProtoRelationship(req.Relationship),
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	worldview, err := user.NewWorldview(
		fromProtoWorldview(req.Worldview),
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	politicalViews, err := user.NewPoliticalViews(
		fromProtoPoliticalViews(req.PoliticalViews),
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	birthday, err := user.NewBirthday(
		fromProtoBirthday(req.Birthday),
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	email, err := user.NewEmail(req.Email)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	input := service.CreateUserInput{
		Name:           name,
		Surname:        surname,
		Nickname:       nickname,
		Status:         st,
		Gender:         gender,
		Relationship:   relationship,
		Worldview:      worldview,
		PoliticalViews: politicalViews,
		Birthday:       birthday,
		Email:          email,
	}

	u, err := h.userService.Create(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return toProtoUser(u), nil
}

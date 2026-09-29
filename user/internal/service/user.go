package service

import (
	"context"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

type UserRepository interface {
	Save(ctx context.Context, u *user.User) error
}

type UserService struct {
	userRepo UserRepository
}

func NewUserService(userRepo UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
	}
}

func (s *UserService) Create(ctx context.Context, input CreateUserInput) (*user.User, error) {
	u, err := user.New(
		input.Name,
		input.Surname,
		input.Nickname,
		input.Status,
		input.Gender,
		input.Relationship,
		input.Worldview,
		input.PoliticalViews,
		input.Birthday,
		input.Email,
	)
	if err != nil {
		return nil, errs.Wrap("user.New", err)
	}

	if err = s.userRepo.Save(ctx, u); err != nil {
		return nil, errs.Wrap("s.userRepo.Create", err)
	}

	return u, nil
}

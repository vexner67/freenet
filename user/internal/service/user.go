package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

type UserRepository interface {
	// Save inserts a new user. Returns user.ErrAlreadyExists if the email
	// is taken and user.ErrNicknameTaken if the nickname is.
	Save(ctx context.Context, u *user.User) error
	// Update stores the profile and privacy of an existing user. Returns
	// user.ErrNotFound and user.ErrNicknameTaken.
	Update(ctx context.Context, u *user.User) error
	// GetByID, GetByNickname and GetIDByEmail return user.ErrNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
	GetByNickname(ctx context.Context, nickname user.Nickname) (*user.User, error)
	// GetByIDs skips IDs without a user and keeps no particular order.
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*user.User, error)
	GetIDByEmail(ctx context.Context, email user.Email) (uuid.UUID, error)
	// Search finds users with a filled-in profile matching q, but deleted
	// pages, newest first, with IDs below before (uuid.Nil: from the newest).
	Search(ctx context.Context, q user.SearchQuery, before uuid.UUID, limit int) ([]*user.User, error)
}

// SearchPage is one screen of search results; NextCursor is uuid.Nil on the last.
type SearchPage struct {
	Users      []user.PublicProfile
	NextCursor uuid.UUID
}

type UserService struct {
	userRepo UserRepository
}

func NewUserService(userRepo UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
	}
}

// GetOrCreateByEmail returns the ID of the user with the given email,
// registering a new user with an empty profile if there is none.
func (s *UserService) GetOrCreateByEmail(ctx context.Context, email user.Email) (uuid.UUID, error) {
	id, err := s.userRepo.GetIDByEmail(ctx, email)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, user.ErrNotFound) {
		return uuid.Nil, errs.Wrap("s.userRepo.GetIDByEmail", err)
	}

	u, err := user.New(email)
	if err != nil {
		return uuid.Nil, errs.Wrap("user.New", err)
	}

	err = s.userRepo.Save(ctx, u)
	if err == nil {
		return u.ID(), nil
	}
	if !errors.Is(err, user.ErrAlreadyExists) {
		return uuid.Nil, errs.Wrap("s.userRepo.Save", err)
	}

	// A concurrent request registered the same email between our lookup
	// and insert; the unique constraint decided, so read the winner.
	id, err = s.userRepo.GetIDByEmail(ctx, email)
	if err != nil {
		return uuid.Nil, errs.Wrap("s.userRepo.GetIDByEmail", err)
	}

	return id, nil
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	return u, nil
}

// UpdateProfile replaces "Основное"; partner is the nickname of whom the
// relationship names, "" for nobody (user.ErrPartnerNotFound for no one).
func (s *UserService) UpdateProfile(ctx context.Context, id uuid.UUID, profile user.Profile, partner user.Nickname) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	profile.Partner = uuid.Nil
	if partner != "" && profile.Relationship.TakesPartner() {
		p, err := s.userRepo.GetByNickname(ctx, partner)
		if errors.Is(err, user.ErrNotFound) {
			return nil, user.ErrPartnerNotFound
		}
		if err != nil {
			return nil, errs.Wrap("s.userRepo.GetByNickname", err)
		}
		profile.Partner = p.ID()
	}

	if err = u.UpdateProfile(profile); err != nil {
		return nil, errs.Wrap("u.UpdateProfile", err)
	}

	if err = s.userRepo.Update(ctx, u); err != nil {
		return nil, errs.Wrap("s.userRepo.Update", err)
	}

	return u, nil
}

// UpdateContacts replaces "Контактная информация".
func (s *UserService) UpdateContacts(ctx context.Context, id uuid.UUID, contacts user.Contacts) (*user.User, error) {
	return s.update(ctx, id, func(u *user.User) error {
		u.UpdateContacts(contacts)
		return nil
	})
}

// UpdateStatus replaces the status alone.
func (s *UserService) UpdateStatus(ctx context.Context, id uuid.UUID, status user.Status) (*user.User, error) {
	return s.update(ctx, id, func(u *user.User) error {
		u.UpdateStatus(status)
		return nil
	})
}

// UpdateInterests replaces "Личная информация".
func (s *UserService) UpdateInterests(ctx context.Context, id uuid.UUID, interests user.Interests) (*user.User, error) {
	return s.update(ctx, id, func(u *user.User) error {
		u.UpdateInterests(interests)
		return nil
	})
}

// UpdateExtraFields replaces "Дополнительно".
func (s *UserService) UpdateExtraFields(ctx context.Context, id uuid.UUID, fields []user.ExtraField) (*user.User, error) {
	return s.update(ctx, id, func(u *user.User) error {
		return errs.Wrap("u.UpdateExtraFields", u.UpdateExtraFields(fields))
	})
}

// update reads a user, changes it with change and stores it.
func (s *UserService) update(ctx context.Context, id uuid.UUID, change func(u *user.User) error) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = change(u); err != nil {
		return nil, err
	}

	if err = s.userRepo.Update(ctx, u); err != nil {
		return nil, errs.Wrap("s.userRepo.Update", err)
	}

	return u, nil
}

func (s *UserService) UpdatePrivacy(ctx context.Context, id uuid.UUID, privacy user.Privacy) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = u.UpdatePrivacy(privacy); err != nil {
		return nil, errs.Wrap("u.UpdatePrivacy", err)
	}

	if err = s.userRepo.Update(ctx, u); err != nil {
		return nil, errs.Wrap("s.userRepo.Update", err)
	}

	return u, nil
}

// GetPublicProfiles is what anyone may see about several users at once,
// e.g. the authors of a news feed. Unknown IDs are skipped.
func (s *UserService) GetPublicProfiles(ctx context.Context, ids []uuid.UUID) ([]user.PublicProfile, error) {
	users, err := s.userRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByIDs", err)
	}

	profiles := make([]user.PublicProfile, 0, len(users))
	for _, u := range users {
		profiles = append(profiles, u.PublicProfile())
	}

	return profiles, nil
}

func (s *UserService) SetAvatar(ctx context.Context, id, avatarID uuid.UUID) (*user.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	u.SetAvatar(avatarID)

	if err = s.userRepo.Update(ctx, u); err != nil {
		return nil, errs.Wrap("s.userRepo.Update", err)
	}

	return u, nil
}

func (s *UserService) Search(ctx context.Context, q user.SearchQuery, cursor uuid.UUID, size int) (SearchPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra user tells whether there is a next page.
	found, err := s.userRepo.Search(ctx, q, cursor, size+1)
	if err != nil {
		return SearchPage{}, errs.Wrap("s.userRepo.Search", err)
	}

	var page SearchPage

	if len(found) > size {
		found = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	page.Users = make([]user.PublicProfile, 0, len(found))
	for _, u := range found {
		page.Users = append(page.Users, u.PublicProfile())
	}

	return page, nil
}

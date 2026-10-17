package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/pkg/errs"
)

// RequestRepository keeps who asked to join which closed club ("Заявки").
type RequestRepository interface {
	// Add is idempotent; club.ErrNotFound when the club is gone.
	Add(ctx context.Context, clubID, userID uuid.UUID) error
	// Delete tells whether there was a request to delete.
	Delete(ctx context.Context, clubID, userID uuid.UUID) (bool, error)
	Has(ctx context.Context, clubID, userID uuid.UUID) (bool, error)
	// List returns up to limit requests made before the request before
	// (uuid.Nil: from the newest), the newest first.
	List(ctx context.Context, clubID, before uuid.UUID, limit int) ([]Request, error)
	Count(ctx context.Context, clubID uuid.UUID) (int, error)
	// DeleteByClub forgets the requests of a club that opened.
	DeleteByClub(ctx context.Context, clubID uuid.UUID) error
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// Request is one user asking to join a club; ID is the cursor of lists.
type Request struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// RequestPage is one screen of the requests of a club; NextCursor is
// uuid.Nil on the last, Total counts them all.
type RequestPage struct {
	Requests   []Request
	NextCursor uuid.UUID
	Total      int
}

// Requests pages those who ask to join the club, the newest first, for
// those who run it ("Заявки").
func (s *ClubService) Requests(ctx context.Context, viewerID, clubID, cursor uuid.UUID, size int) (RequestPage, error) {
	if err := s.checkDecide(ctx, viewerID, clubID); err != nil {
		return RequestPage{}, err
	}

	size = pageSize(size)

	// One extra tells whether there is a next page.
	requests, err := s.requests.List(ctx, clubID, cursor, size+1)
	if err != nil {
		return RequestPage{}, errs.Wrap("s.requests.List", err)
	}

	page := RequestPage{Requests: requests}
	if len(requests) > size {
		page.Requests = requests[:size]
		page.NextCursor = requests[size-1].ID
	}

	if page.Total, err = s.requests.Count(ctx, clubID); err != nil {
		return RequestPage{}, errs.Wrap("s.requests.Count", err)
	}

	return page, nil
}

// Accept lets userID, who asked, into the club ("Принять заявку"): its
// admins' call. club.ErrNoRequest when there is no such request, as when
// another admin has decided already.
func (s *ClubService) Accept(ctx context.Context, requesterID, clubID, userID uuid.UUID) error {
	if err := s.checkDecide(ctx, requesterID, clubID); err != nil {
		return err
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		asked, err := s.requests.Delete(ctx, clubID, userID)
		if err != nil {
			return errs.Wrap("s.requests.Delete", err)
		}

		if !asked {
			return club.ErrNoRequest
		}

		if err = s.members.Join(ctx, clubID, userID); err != nil {
			return errs.Wrap("s.members.Join", err)
		}

		return errs.Wrap("s.readers.Joined", s.readers.Joined(ctx, clubID, userID))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Reject turns the request of userID down ("Отклонить заявку"): the
// admins' call; club.ErrNoRequest when there is none.
func (s *ClubService) Reject(ctx context.Context, requesterID, clubID, userID uuid.UUID) error {
	if err := s.checkDecide(ctx, requesterID, clubID); err != nil {
		return err
	}

	asked, err := s.requests.Delete(ctx, clubID, userID)
	if err != nil {
		return errs.Wrap("s.requests.Delete", err)
	}

	if !asked {
		return club.ErrNoRequest
	}

	return nil
}

// checkDecide tells whether userID decides on the requests of the club.
func (s *ClubService) checkDecide(ctx context.Context, userID, clubID uuid.UUID) error {
	if _, err := s.clubs.Get(ctx, clubID); err != nil {
		return errs.Wrap("s.clubs.Get", err)
	}

	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	return errs.Wrap("club.CheckDecide", club.CheckDecide(role))
}

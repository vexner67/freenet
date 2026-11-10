package room

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Room struct {
	id, ownerID, clubID uuid.UUID
	title               Title
	kind                Kind
	access              Access
	control             Control
	permanent           bool
	createdAt, closedAt time.Time
}

type Input struct {
	OwnerID, ClubID uuid.UUID
	Title           Title
	Kind            Kind
	Access          Access
	Control         Control
	Permanent       bool
}

func New(in Input, now time.Time) (*Room, error) {
	if err := validate(&in); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("uuid.NewV7: %w", err)
	}
	return Restore(id, in, now, time.Time{}), nil
}

// Restore reconstructs a row whose constraints have already been checked.
func Restore(id uuid.UUID, in Input, createdAt, closedAt time.Time) *Room {
	return &Room{id: id, ownerID: in.OwnerID, clubID: in.ClubID, title: in.Title, kind: in.Kind, access: in.Access, control: in.Control, permanent: in.Permanent, createdAt: createdAt.UTC(), closedAt: closedAt.UTC()}
}

func validate(in *Input) error {
	if in.OwnerID == uuid.Nil {
		return ErrForbidden
	}
	title, err := NewTitle(in.Title.String())
	if err != nil {
		return err
	}
	in.Title = title
	if _, err := NewKind(in.Kind.String()); err != nil {
		return err
	}
	if _, err := NewAccess(in.Access.String()); err != nil {
		return err
	}
	if _, err := NewControl(in.Control.String()); err != nil {
		return err
	}
	if in.Access == AccessClub && in.ClubID == uuid.Nil {
		return ErrInvalidAccess
	}
	return nil
}

func (r *Room) ID() uuid.UUID        { return r.id }
func (r *Room) OwnerID() uuid.UUID   { return r.ownerID }
func (r *Room) ClubID() uuid.UUID    { return r.clubID }
func (r *Room) Title() Title         { return r.title }
func (r *Room) Kind() Kind           { return r.kind }
func (r *Room) Access() Access       { return r.access }
func (r *Room) Control() Control     { return r.control }
func (r *Room) Permanent() bool      { return r.permanent }
func (r *Room) CreatedAt() time.Time { return r.createdAt }
func (r *Room) ClosedAt() time.Time  { return r.closedAt }
func (r *Room) Closed() bool         { return !r.closedAt.IsZero() }

// Viewer contains relationships resolved by their owning services.
type Viewer struct {
	ID                                          uuid.UUID
	Friend, ClubMember, Blocked, Banned, Kicked bool
}

func (r *Room) CanEnter(v Viewer) error {
	if r.Closed() {
		return ErrClosed
	}
	if v.ID == uuid.Nil || v.Banned || v.Blocked {
		return ErrForbidden
	}
	if v.Kicked {
		return ErrKicked
	}
	if v.ID == r.ownerID {
		return nil
	}
	switch r.access {
	case AccessEveryone, AccessLink:
		return nil
	case AccessFriends:
		if v.Friend {
			return nil
		}
	case AccessClub:
		if v.ClubMember {
			return nil
		}
	}
	return ErrForbidden
}

// CanView hides inaccessible rooms; link-only rooms never appear in lists.
func (r *Room) CanView(v Viewer, listing bool) bool {
	if r.Closed() || v.Blocked || v.Banned || v.Kicked || (listing && r.access == AccessLink) {
		return false
	}
	if v.ID == uuid.Nil {
		return r.access == AccessEveryone
	}
	return r.CanEnter(v) == nil
}

func (r *Room) CanControl(id uuid.UUID, member bool) bool {
	return !r.Closed() && member && (id == r.ownerID || r.control == ControlEveryone)
}

func (r *Room) Update(actor uuid.UUID, in Input) error {
	if actor != r.ownerID {
		return ErrForbidden
	}
	if r.Closed() {
		return ErrClosed
	}
	in.OwnerID, in.ClubID, in.Permanent = r.ownerID, r.clubID, r.permanent
	if err := validate(&in); err != nil {
		return err
	}
	r.title, r.kind, r.access, r.control = in.Title, in.Kind, in.Access, in.Control
	return nil
}

func (r *Room) Close(actor uuid.UUID, now time.Time) error {
	if actor != r.ownerID {
		return ErrForbidden
	}
	if !r.Closed() {
		r.closedAt = now.UTC()
	}
	return nil
}

func (r *Room) Kick(actor, target uuid.UUID, now time.Time) (time.Time, error) {
	if r.Closed() {
		return time.Time{}, ErrClosed
	}
	if actor != r.ownerID || target == r.ownerID || target == uuid.Nil {
		return time.Time{}, ErrForbidden
	}
	if r.permanent {
		return now.Add(24 * time.Hour).UTC(), nil
	}
	return time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), nil
}

func (r *Room) FollowPlayback(actor uuid.UUID, member bool, kind PlaybackKind) error {
	if !r.CanControl(actor, member) {
		return ErrForbidden
	}
	switch kind {
	case PlaybackYouTube:
		r.kind = KindWatch
	case PlaybackAudio:
		r.kind = KindListen
	}
	return nil
}

// AcceptsPlayback keeps the screen consistent when its owner changes kind.
func (r *Room) AcceptsPlayback(kind PlaybackKind) bool {
	return kind == PlaybackNone || (r.kind == KindWatch && kind == PlaybackYouTube) || (r.kind == KindListen && kind == PlaybackAudio)
}

package grpc

import (
	"time"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	walls = map[club.Wall]clubv1.Wall{
		club.WallOpen:    clubv1.Wall_WALL_OPEN,
		club.WallClosed:  clubv1.Wall_WALL_CLOSED,
		club.WallSuggest: clubv1.Wall_WALL_SUGGEST,
	}
	adminsDisplays = map[club.AdminsDisplay]clubv1.AdminsDisplay{
		club.AdminsDisplayCreator: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR,
		club.AdminsDisplayAll:     clubv1.AdminsDisplay_ADMINS_DISPLAY_ALL,
		club.AdminsDisplayNone:    clubv1.AdminsDisplay_ADMINS_DISPLAY_NONE,
	}
	kinds = map[club.Kind]clubv1.Kind{
		club.KindGroup: clubv1.Kind_KIND_GROUP,
		club.KindEvent: clubv1.Kind_KIND_EVENT,
	}
	picks = map[clubv1.Going]service.MemberPick{
		clubv1.Going_GOING_UNSPECIFIED: service.PickAll,
		clubv1.Going_GOING_SURE:        service.PickSure,
		clubv1.Going_GOING_MAYBE:       service.PickMaybe,
	}
	roles = map[club.Role]clubv1.Role{
		club.RoleNone:   clubv1.Role_ROLE_NONE,
		club.RoleMember: clubv1.Role_ROLE_MEMBER,
		club.RoleAdmin:  clubv1.Role_ROLE_ADMIN,
		club.RoleOwner:  clubv1.Role_ROLE_OWNER,
	}
)

func toProtoClub(c *club.Club) *clubv1.Club {
	return &clubv1.Club{
		Id:            c.ID().String(),
		Name:          c.Name().String(),
		Description:   c.Description().String(),
		Address:       c.Address().String(),
		Wall:          walls[c.Wall()],
		MemberCount:   int32(c.Members()), //nolint:gosec // counters come from int4 columns
		CreatedAt:     timestamppb.New(c.CreatedAt()),
		AvatarId:      optionalID(c.AvatarID()),
		Website:       c.Website().String(),
		AdminsDisplay: adminsDisplays[c.AdminsDisplay()],

		OpenTopics:      c.OpenTopics(),
		TopicsAboveWall: c.TopicsAboveWall(),
		BackdropLeft:    optionalID(c.Backdrop().Left),
		BackdropRight:   optionalID(c.Backdrop().Right),

		Kind:       kinds[c.Kind()],
		StartsAt:   optionalTime(c.Schedule().Start),
		EndsAt:     optionalTime(c.Schedule().End),
		MaybeCount: int32(c.Maybes()), //nolint:gosec // counters come from int4 columns
	}
}

// optionalTime is unset for the zero time.
func optionalTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

// fromProtoTime is the zero time for an unset one.
func fromProtoTime(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.AsTime()
}

// fromProtoSchedule reads the schedule of an event.
func fromProtoSchedule(start, end *timestamppb.Timestamp) (club.Schedule, error) {
	return club.NewSchedule(fromProtoTime(start), fromProtoTime(end))
}

// fromProtoKind reads which clubs to list: KIND_UNSPECIFIED is "" for
// both groups and events.
func fromProtoKind(k clubv1.Kind) club.Kind {
	for kind, pk := range kinds {
		if pk == k {
			return kind
		}
	}

	return ""
}

func toProtoClubs(clubs []*club.Club) []*clubv1.Club {
	out := make([]*clubv1.Club, 0, len(clubs))
	for _, c := range clubs {
		out = append(out, toProtoClub(c))
	}

	return out
}

func toProtoView(v service.View) *clubv1.ClubView {
	return &clubv1.ClubView{
		Club:    toProtoClub(v.Club),
		Role:    roles[v.Role],
		OwnerId: optionalID(v.OwnerID),

		OwnerTitle: v.OwnerTitle.String(),
		Maybe:      v.Maybe,

		CanStartTopic: v.Club.CanStartTopic(v.Role),
	}
}

// fromProtoWall reads a wall mode; WALL_UNSPECIFIED is no mode at all.
func fromProtoWall(w clubv1.Wall) (club.Wall, error) {
	for mode, pw := range walls {
		if pw == w {
			return mode, nil
		}
	}

	return "", club.ErrInvalidWall
}

// fromProtoAdminsDisplay reads what a page shows of its admins;
// ADMINS_DISPLAY_UNSPECIFIED is nothing chosen.
func fromProtoAdminsDisplay(d clubv1.AdminsDisplay) (club.AdminsDisplay, error) {
	for display, pd := range adminsDisplays {
		if pd == d {
			return display, nil
		}
	}

	return "", club.ErrInvalidAdminsDisplay
}

// fromProtoRole reads a role the owner may give.
func fromProtoRole(r clubv1.Role) (club.Role, error) {
	for role, pr := range roles {
		if pr == r {
			return club.NewRole(role.String())
		}
	}

	return "", club.ErrInvalidRole
}

// fromProtoChanges reads the settings form of a club.
func fromProtoChanges(req *clubv1.UpdateClubRequest) (club.Changes, error) {
	name, err := club.NewName(req.GetName())
	if err != nil {
		return club.Changes{}, err
	}

	description, err := club.NewDescription(req.GetDescription())
	if err != nil {
		return club.Changes{}, err
	}

	address, err := club.NewAddress(req.GetAddress())
	if err != nil {
		return club.Changes{}, err
	}

	website, err := club.NewWebsite(req.GetWebsite())
	if err != nil {
		return club.Changes{}, err
	}

	wall, err := fromProtoWall(req.GetWall())
	if err != nil {
		return club.Changes{}, err
	}

	adminsDisplay, err := fromProtoAdminsDisplay(req.GetAdminsDisplay())
	if err != nil {
		return club.Changes{}, err
	}

	// A group sends no schedule; whether an event has one is its own rule.
	var schedule club.Schedule
	if req.GetStartsAt() != nil {
		if schedule, err = fromProtoSchedule(req.GetStartsAt(), req.GetEndsAt()); err != nil {
			return club.Changes{}, err
		}
	}

	return club.Changes{
		Name: name, Description: description, Address: address, Website: website,
		Wall: wall, AdminsDisplay: adminsDisplay,
		OpenTopics: req.GetOpenTopics(), TopicsAboveWall: req.GetTopicsAboveWall(),
		Schedule: schedule,
	}, nil
}

// fromProtoRef reads a club's ID or address: an address never parses as a
// UUID. One that is not even a valid address names no club.
func fromProtoRef(s string) (service.Ref, error) {
	if id, err := uuid.Parse(s); err == nil {
		return service.Ref{ID: id}, nil
	}

	address, err := club.NewAddress(s)
	if err != nil || address == "" {
		return service.Ref{}, club.ErrNotFound
	}

	return service.Ref{Address: address}, nil
}

// optionalID is "" for uuid.Nil.
func optionalID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}

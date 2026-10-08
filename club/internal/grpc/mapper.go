package grpc

import (
	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	walls = map[club.Wall]clubv1.Wall{
		club.WallOpen:   clubv1.Wall_WALL_OPEN,
		club.WallClosed: clubv1.Wall_WALL_CLOSED,
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
		Id:          c.ID().String(),
		Name:        c.Name().String(),
		Description: c.Description().String(),
		Address:     c.Address().String(),
		Wall:        walls[c.Wall()],
		MemberCount: int32(c.Members()), //nolint:gosec // counters come from int4 columns
		CreatedAt:   timestamppb.New(c.CreatedAt()),
	}
}

func toProtoView(v service.View) *clubv1.ClubView {
	return &clubv1.ClubView{
		Club:    toProtoClub(v.Club),
		Role:    roles[v.Role],
		OwnerId: optionalID(v.OwnerID),
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

	wall, err := fromProtoWall(req.GetWall())
	if err != nil {
		return club.Changes{}, err
	}

	return club.Changes{Name: name, Description: description, Address: address, Wall: wall}, nil
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

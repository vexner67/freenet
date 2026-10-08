// Package club is the domain of communities ("группы" of openvk): what
// they are called and say, who is in them and what each member may do.
package club

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound         = errors.New("club not found")
	ErrCannotEdit       = errors.New("only the admins can edit the club")
	ErrOwnerCannotLeave = errors.New("the owner cannot leave the club")
	ErrCannotAppoint    = errors.New("only the owner appoints the admins")
	ErrNotMember        = errors.New("the user is not a member of the club")
)

// Club is a community: a page with a wall that people join. Whoever
// creates it is its owner. A club does not change once made: Edited
// returns a new one.
type Club struct {
	id          uuid.UUID
	name        Name
	description Description
	address     Address
	wall        Wall
	createdAt   time.Time
	// members counts everyone in the club, the owner and admins too.
	members int
}

// New creates a club, open to posts from everyone as in openvk, with no
// address and no members yet: storing it makes its creator the owner.
func New(name Name, description Description, now time.Time) (*Club, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Club{
		id:          id,
		name:        name,
		description: description,
		wall:        WallOpen,
		// PostgreSQL keeps microseconds; trim now so a club reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Stored is a club as storage keeps it.
type Stored struct {
	ID          uuid.UUID
	Name        Name
	Description Description
	Address     Address
	Wall        Wall
	CreatedAt   time.Time
	Members     int
}

// Restore rebuilds a club from storage without validation or new IDs.
func Restore(s Stored) *Club {
	return &Club{
		id:          s.ID,
		name:        s.Name,
		description: s.Description,
		address:     s.Address,
		wall:        s.Wall,
		createdAt:   s.CreatedAt,
		members:     s.Members,
	}
}

// Changes are what the admins may change in a club: everything at once,
// as the settings form sends it.
type Changes struct {
	Name        Name
	Description Description
	Address     Address
	Wall        Wall
}

// Edited returns c with changes made by someone who is role in it: only
// the admins edit a club.
func (c *Club) Edited(role Role, changes Changes) (*Club, error) {
	if !role.Manages() {
		return nil, ErrCannotEdit
	}

	edited := *c
	edited.name = changes.Name
	edited.description = changes.Description
	edited.address = changes.Address
	edited.wall = changes.Wall

	return &edited, nil
}

// CanPostOnWall tells whether someone who is role in the club may write
// on its wall, on their own behalf or, fromClub, on the club's. Only the
// admins speak for the club and write on a closed wall; an open one
// takes posts from anyone, members or not, as in openvk.
func (c *Club) CanPostOnWall(role Role, fromClub bool) bool {
	if fromClub || c.wall == WallClosed {
		return role.Manages()
	}

	return true
}

// CheckLeave: anyone may leave a club but its owner, who would leave it
// with nobody to appoint admins.
func CheckLeave(role Role) error {
	if role == RoleOwner {
		return ErrOwnerCannotLeave
	}

	return nil
}

// CheckAppoint tells whether requester may make someone who is target in
// the club a member or an admin: only the owner appoints, and only members
// and admins other than the owner.
func CheckAppoint(requester, target Role) error {
	if requester != RoleOwner {
		return ErrCannotAppoint
	}

	switch target {
	case RoleMember, RoleAdmin:
		return nil
	case RoleOwner:
		return ErrCannotAppoint
	default:
		return ErrNotMember
	}
}

func (c *Club) ID() uuid.UUID {
	return c.id
}

func (c *Club) Name() Name {
	return c.name
}

func (c *Club) Description() Description {
	return c.description
}

func (c *Club) Address() Address {
	return c.address
}

func (c *Club) Wall() Wall {
	return c.wall
}

func (c *Club) CreatedAt() time.Time {
	return c.createdAt
}

func (c *Club) Members() int {
	return c.members
}

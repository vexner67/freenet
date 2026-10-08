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
	ErrCannotPin        = errors.New("only the admins can add the club to their menu")
	ErrTooManyPinned    = errors.New("at most 10 clubs in the menu")
	ErrCannotTransfer   = errors.New("only the owner hands the club over")
	ErrNotAdmin         = errors.New("the new owner must be an admin of the club")
)

// MaxPinned is how many of the clubs they run a user keeps in the left
// menu, as openvk allows.
const MaxPinned = 10

// Club is a community: a page with a wall that people join. Whoever
// creates it is its owner. A club does not change once made: Edited
// returns a new one.
type Club struct {
	id          uuid.UUID
	name        Name
	description Description
	address     Address
	website     Website
	wall        Wall
	// adminsDisplay is what the page shows of those who run the club.
	adminsDisplay AdminsDisplay
	createdAt     time.Time
	// members counts everyone in the club, the owner and admins too.
	members int
	// avatarID is the club's photo in the media service; uuid.Nil for none.
	avatarID uuid.UUID
}

// New creates a club, open to posts from everyone and showing its creator
// as in openvk, with no address and no members yet: storing it makes its
// creator the owner.
func New(name Name, description Description, now time.Time) (*Club, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Club{
		id:            id,
		name:          name,
		description:   description,
		wall:          WallOpen,
		adminsDisplay: AdminsDisplayCreator,
		// PostgreSQL keeps microseconds; trim now so a club reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Stored is a club as storage keeps it.
type Stored struct {
	ID            uuid.UUID
	Name          Name
	Description   Description
	Address       Address
	Website       Website
	Wall          Wall
	AdminsDisplay AdminsDisplay
	CreatedAt     time.Time
	Members       int
	AvatarID      uuid.UUID
}

// Restore rebuilds a club from storage without validation or new IDs.
func Restore(s Stored) *Club {
	return &Club{
		id:            s.ID,
		name:          s.Name,
		description:   s.Description,
		address:       s.Address,
		website:       s.Website,
		wall:          s.Wall,
		adminsDisplay: s.AdminsDisplay,
		createdAt:     s.CreatedAt,
		members:       s.Members,
		avatarID:      s.AvatarID,
	}
}

// Changes are what the admins may change in a club: everything at once,
// as the settings form sends it.
type Changes struct {
	Name          Name
	Description   Description
	Address       Address
	Website       Website
	Wall          Wall
	AdminsDisplay AdminsDisplay
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
	edited.website = changes.Website
	edited.wall = changes.Wall
	edited.adminsDisplay = changes.AdminsDisplay

	return &edited, nil
}

// WithAvatar returns c showing the photo avatarID (uuid.Nil: none), set by
// someone who is role in it: only the admins change it.
func (c *Club) WithAvatar(role Role, avatarID uuid.UUID) (*Club, error) {
	if !role.Manages() {
		return nil, ErrCannotEdit
	}

	changed := *c
	changed.avatarID = avatarID

	return &changed, nil
}

// CheckPin: only those who run a club put it in their left menu, as the
// "Добавить в левое меню" of openvk's "Управление".
func CheckPin(role Role) error {
	if !role.Manages() {
		return ErrCannotPin
	}

	return nil
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

// CheckTransfer tells whether requester may hand the club over to someone
// who is target in it, openvk's "Назначить владельцем": only the owner,
// and only to one of the admins. The owner stays an admin.
func CheckTransfer(requester, target Role) error {
	if requester != RoleOwner {
		return ErrCannotTransfer
	}

	if target != RoleAdmin {
		return ErrNotAdmin
	}

	return nil
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

func (c *Club) Website() Website {
	return c.website
}

func (c *Club) AdminsDisplay() AdminsDisplay {
	return c.adminsDisplay
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

func (c *Club) AvatarID() uuid.UUID {
	return c.avatarID
}

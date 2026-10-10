// Package club is the domain of communities ("группы" of openvk): what
// they are called and say, who is in them and what each member may do.
package club

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("club not found")
	ErrCannotEdit          = errors.New("only the admins can edit the club")
	ErrInvalidBackdropSide = errors.New("invalid backdrop side")
	ErrOwnerCannotLeave    = errors.New("the owner cannot leave the club")
	ErrCannotAppoint       = errors.New("only the owner appoints the admins")
	ErrNotMember           = errors.New("the user is not a member of the club")
	ErrCannotPin           = errors.New("only the admins can add the club to their menu")
	ErrTooManyPinned       = errors.New("at most 10 clubs in the menu")
	ErrCannotTransfer      = errors.New("only the owner hands the club over")
	ErrNotAdmin            = errors.New("the new owner must be an admin of the club")
)

// MaxPinned is how many of the clubs they run a user keeps in the left
// menu, as openvk allows.
const MaxPinned = 10

// Club is a community: a page with a wall that people join. Whoever
// creates it is its owner. A club does not change once made: Edited
// returns a new one.
type Club struct {
	id uuid.UUID
	// kind and schedule: a group, or an event with the time it starts.
	kind     Kind
	schedule Schedule
	// maybes counts the members of an event who only might come.
	maybes      int
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
	// openTopics: anyone starts topics in its discussions, not only the
	// admins ("Все могут создавать темы" of openvk); topicsAboveWall: the
	// page lists them above the wall rather than in the right column.
	openTopics      bool
	topicsAboveWall bool
	// backdrop is "Фон страницы" of openvk: photos in the media service
	// shown on the left and on the right of the page; uuid.Nil for none.
	backdrop Backdrop
}

// Backdrop is the two sides of a club's backdrop; either may be uuid.Nil.
type Backdrop struct {
	Left  uuid.UUID
	Right uuid.UUID
}

// BackdropSide is which side of the page a backdrop photo is on.
type BackdropSide string

const (
	BackdropLeft  BackdropSide = "left"
	BackdropRight BackdropSide = "right"
)

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
		kind:          KindGroup,
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

	OpenTopics      bool
	TopicsAboveWall bool
	Backdrop        Backdrop

	Kind     Kind
	Schedule Schedule
	Maybes   int
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

		openTopics:      s.OpenTopics,
		topicsAboveWall: s.TopicsAboveWall,
		backdrop:        s.Backdrop,

		kind:     s.Kind,
		schedule: s.Schedule,
		maybes:   s.Maybes,
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
	// "Обсуждения": who starts topics and where the page lists them.
	OpenTopics      bool
	TopicsAboveWall bool
	// Schedule is when an event starts and ends; a group has none.
	Schedule Schedule
}

// Edited returns c with changes made by someone who is role in it: only
// the admins edit a club. An event keeps a schedule, a group has none.
func (c *Club) Edited(role Role, changes Changes) (*Club, error) {
	if !role.Manages() {
		return nil, ErrCannotEdit
	}

	edited := *c

	if c.IsEvent() {
		if changes.Schedule.Start.IsZero() {
			return nil, ErrNoStart
		}
		edited.schedule = changes.Schedule
	}

	edited.name = changes.Name
	edited.description = changes.Description
	edited.address = changes.Address
	edited.website = changes.Website
	edited.wall = changes.Wall
	edited.adminsDisplay = changes.AdminsDisplay
	edited.openTopics = changes.OpenTopics
	edited.topicsAboveWall = changes.TopicsAboveWall

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

// WithBackdrop returns c showing photoID (uuid.Nil: none) on side of its
// page, set by someone who is role in it: only the admins change it, as
// openvk's "Фон страницы".
func (c *Club) WithBackdrop(role Role, side BackdropSide, photoID uuid.UUID) (*Club, error) {
	if !role.Manages() {
		return nil, ErrCannotEdit
	}

	changed := *c

	switch side {
	case BackdropLeft:
		changed.backdrop.Left = photoID
	case BackdropRight:
		changed.backdrop.Right = photoID
	default:
		return nil, ErrInvalidBackdropSide
	}

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
// admins speak for the club and write on a wall that is not open; an
// open one takes posts from anyone, members or not, as in openvk.
func (c *Club) CanPostOnWall(role Role, fromClub bool) bool {
	if fromClub || c.wall != WallOpen {
		return role.Manages()
	}

	return true
}

// CanStartTopic tells whether someone who is role in the club may start a
// topic in its discussions: the admins, or anyone when the club lets
// everyone ("Все могут создавать темы"), members or not, as in openvk.
func (c *Club) CanStartTopic(role Role) bool {
	return c.openTopics || role.Manages()
}

// CanSuggest tells whether someone who is role in the club may suggest a
// post for its wall, "Предложить новость" of openvk: anyone but the admins,
// who post themselves, when the wall is "Предложка".
func (c *Club) CanSuggest(role Role) bool {
	return c.wall == WallSuggest && !role.Manages()
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

func (c *Club) OpenTopics() bool {
	return c.openTopics
}

func (c *Club) TopicsAboveWall() bool {
	return c.topicsAboveWall
}

func (c *Club) Backdrop() Backdrop {
	return c.backdrop
}

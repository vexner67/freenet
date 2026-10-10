package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxClubs bounds GetMany: the gateway asks for the clubs of one page.
	MaxClubs = 100
	// MaxClubIDs is how many of a user's clubs the feed reads posts from.
	MaxClubIDs = 1000
	// MaxWallModerators is how many admins of a club hear about a post
	// suggested for its wall; a club rarely has more.
	MaxWallModerators = 100
)

var ErrTooManyIDs = errors.New("too many ids")

type ClubRepository interface {
	// Create stores c with ownerID as its owner and only member.
	Create(ctx context.Context, c *club.Club, ownerID uuid.UUID) error
	// Get and GetByAddress return club.ErrNotFound; the address is
	// matched whatever the case.
	Get(ctx context.Context, id uuid.UUID) (*club.Club, error)
	GetByAddress(ctx context.Context, address club.Address) (*club.Club, error)
	// GetMany returns the clubs found, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*club.Club, error)
	// Update saves the editable fields; club.ErrAddressTaken when another
	// club has the address.
	Update(ctx context.Context, c *club.Club) error
	// SetAvatar saves the photo of c; club.ErrNotFound.
	SetAvatar(ctx context.Context, c *club.Club) error
	// SetBackdrop saves one side of the backdrop of c; club.ErrNotFound.
	SetBackdrop(ctx context.Context, c *club.Club, side club.BackdropSide) error
	// Search returns up to limit clubs that match q with IDs below before
	// (uuid.Nil: from the newest), the newest first.
	Search(ctx context.Context, q club.SearchQuery, before uuid.UUID, limit int) ([]*club.Club, error)
}

type MemberRepository interface {
	// Role is club.RoleNone for a user who is not a member.
	Role(ctx context.Context, clubID, userID uuid.UUID) (club.Role, error)
	// Seat is the role with the title and whether the club hides it; the
	// zero seat for a user who is not a member.
	Seat(ctx context.Context, clubID, userID uuid.UUID) (club.Seat, error)
	// Owner is uuid.Nil when the club has none (its owner was erased).
	Owner(ctx context.Context, clubID uuid.UUID) (uuid.UUID, error)
	// Join is idempotent; club.ErrNotFound when the club is gone. maybe:
	// the member only might come to the event.
	Join(ctx context.Context, clubID, userID uuid.UUID, maybe bool) error
	Leave(ctx context.Context, clubID, userID uuid.UUID) error
	// SetRole changes the role of a member who is not the owner;
	// club.ErrNotMember when there is no such member. An admin made a
	// member again loses the title and is hidden no more.
	SetRole(ctx context.Context, clubID, userID uuid.UUID, role club.Role) error
	// Describe gives one of those who run the club a title and hides or
	// shows them; club.ErrNotManager when the user does not run it.
	Describe(ctx context.Context, clubID, userID uuid.UUID, title club.Title, hidden bool) error
	// List returns up to limit members that filter picks who joined
	// before the membership before (uuid.Nil: from the newest), the
	// newest first.
	List(ctx context.Context, clubID, before uuid.UUID, limit int, filter MemberFilter) ([]Member, error)
	// CountAdmins counts the admins and the owner of a club, with
	// shownOnly those of them it does not hide.
	CountAdmins(ctx context.Context, clubID uuid.UUID, shownOnly bool) (int, error)
	// Transfer makes newOwnerID, an admin, the owner of the club and
	// ownerID, its owner, an admin, both at once; club.ErrCannotTransfer
	// when ownerID no longer owns it, club.ErrNotAdmin when newOwnerID is
	// no longer an admin.
	Transfer(ctx context.Context, clubID, ownerID, newOwnerID uuid.UUID) error
	// ListClubs is the same for the clubs of a user that filter picks.
	ListClubs(ctx context.Context, userID, before uuid.UUID, limit int, filter ClubFilter) ([]Membership, error)
	// CountClubs counts the clubs of a user that filter picks.
	CountClubs(ctx context.Context, userID uuid.UUID, filter ClubFilter) (int, error)
	// Pin puts a club the user runs in their menu, or takes it out; false
	// when limit clubs are there already.
	Pin(ctx context.Context, clubID, userID uuid.UUID, pinned bool, limit int) (bool, error)
	// Pinned are the clubs in the user's menu that they still run, in the
	// order of joining.
	Pinned(ctx context.Context, userID uuid.UUID) ([]*club.Club, error)
	// ClubIDs are the newest limit clubs of a user.
	ClubIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error)
	// DeleteByUser takes a user out of every club.
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// MemberFilter picks the members a list shows.
type MemberFilter int

const (
	// AllMembers are everyone in the club.
	AllMembers MemberFilter = iota
	// Managers are its owner and admins.
	Managers
	// ShownManagers are those of them the club does not hide.
	ShownManagers
	// SureComers and MaybeComers are the members of an event who come for
	// sure ("Точно придут") and who only might ("Возможно придут").
	SureComers
	MaybeComers
)

// MemberPick is the list of members a caller asks for.
type MemberPick int

const (
	PickAll MemberPick = iota
	// PickAdmins are the owner and the admins, as the caller sees them.
	PickAdmins
	PickSure
	PickMaybe
)

// ClubFilter picks the clubs of a user: only those they run with Managed,
// only those of Kind unless it is empty.
type ClubFilter struct {
	Managed bool
	Kind    club.Kind
}

// Member is one user in a club; ID is the membership, the cursor of lists.
type Member struct {
	ID     uuid.UUID
	UserID uuid.UUID
	club.Seat
}

// Membership is one club of a user and what the user is in it; Pinned:
// in their left menu.
type Membership struct {
	ID     uuid.UUID
	Club   *club.Club
	Role   club.Role
	Pinned bool
}

// View is a club as a viewer sees it: what they are in it and who owns it
// (uuid.Nil if nobody, or if the owner is hidden from the viewer), with
// the owner's title. Maybe: the viewer only might come to the event.
type View struct {
	Club       *club.Club
	Role       club.Role
	Maybe      bool
	OwnerID    uuid.UUID
	OwnerTitle club.Title
}

// MemberPage and ClubPage are one screen of a list; NextCursor is
// uuid.Nil on the last.
type MemberPage struct {
	Members    []Member
	NextCursor uuid.UUID
	// Total counts the members, or the admins, on all pages.
	Total int
}

type ClubPage struct {
	Clubs      []Membership
	NextCursor uuid.UUID
	Total      int
}

// SearchPage is one screen of found clubs.
type SearchPage struct {
	Clubs      []*club.Club
	NextCursor uuid.UUID
}

// Ref names a club by its ID or, if ID is uuid.Nil, by its address.
type Ref struct {
	ID      uuid.UUID
	Address club.Address
}

// ClubDeps are what a ClubService is made of.
type ClubDeps struct {
	Clubs    ClubRepository
	Members  MemberRepository
	Tx       TxManager
	Notifier Notifier
}

type ClubService struct {
	clubs    ClubRepository
	members  MemberRepository
	tx       TxManager
	notifier Notifier
	now      func() time.Time
}

func NewClubService(deps ClubDeps) *ClubService {
	return &ClubService{
		clubs:    deps.Clubs,
		members:  deps.Members,
		tx:       deps.Tx,
		notifier: deps.Notifier,
		now:      time.Now,
	}
}

// Create makes a club with ownerID as its owner, as "Создать группу" in
// openvk.
func (s *ClubService) Create(ctx context.Context, ownerID uuid.UUID, name club.Name, description club.Description) (View, error) {
	c, err := club.New(name, description, s.now())
	if err != nil {
		return View{}, errs.Wrap("club.New", err)
	}

	if err = s.clubs.Create(ctx, c, ownerID); err != nil {
		return View{}, errs.Wrap("s.clubs.Create", err)
	}

	return s.view(ctx, ownerID, c.ID())
}

// CreateEvent makes an event with ownerID as its owner, who comes for
// sure: "Создать встречу" of openvk.
func (s *ClubService) CreateEvent(ctx context.Context, ownerID uuid.UUID, name club.Name, description club.Description, schedule club.Schedule) (View, error) {
	c, err := club.NewEvent(name, description, schedule, s.now())
	if err != nil {
		return View{}, errs.Wrap("club.NewEvent", err)
	}

	if err = s.clubs.Create(ctx, c, ownerID); err != nil {
		return View{}, errs.Wrap("s.clubs.Create", err)
	}

	return s.view(ctx, ownerID, c.ID())
}

// Find is the club ref names as viewerID sees it; uuid.Nil is an
// anonymous viewer.
func (s *ClubService) Find(ctx context.Context, viewerID uuid.UUID, ref Ref) (View, error) {
	id := ref.ID
	if id == uuid.Nil {
		c, err := s.clubs.GetByAddress(ctx, ref.Address)
		if err != nil {
			return View{}, errs.Wrap("s.clubs.GetByAddress", err)
		}
		id = c.ID()
	}

	return s.view(ctx, viewerID, id)
}

func (s *ClubService) view(ctx context.Context, viewerID, clubID uuid.UUID) (View, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return View{}, errs.Wrap("s.clubs.Get", err)
	}

	v := View{Club: c}

	if v.OwnerID, err = s.members.Owner(ctx, clubID); err != nil {
		return View{}, errs.Wrap("s.members.Owner", err)
	}

	if viewerID != uuid.Nil {
		viewer, err := s.members.Seat(ctx, clubID, viewerID)
		if err != nil {
			return View{}, errs.Wrap("s.members.Seat", err)
		}

		v.Role, v.Maybe = viewer.Role, viewer.Maybe
	}

	if v.OwnerID == uuid.Nil {
		return v, nil
	}

	owner, err := s.members.Seat(ctx, clubID, v.OwnerID)
	if err != nil {
		return View{}, errs.Wrap("s.members.Seat", err)
	}

	if !owner.Seen(v.Role) {
		v.OwnerID = uuid.Nil
		return v, nil
	}

	v.OwnerTitle = owner.Title

	return v, nil
}

// GetMany returns the clubs found among ids, for a page that names them.
func (s *ClubService) GetMany(ctx context.Context, ids []uuid.UUID) ([]*club.Club, error) {
	if len(ids) > MaxClubs {
		return nil, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return nil, nil
	}

	clubs, err := s.clubs.GetMany(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.clubs.GetMany", err)
	}

	return clubs, nil
}

// Update changes a club with its admins' settings form.
func (s *ClubService) Update(ctx context.Context, userID, clubID uuid.UUID, changes club.Changes) (View, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return View{}, errs.Wrap("s.clubs.Get", err)
	}

	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return View{}, errs.Wrap("s.members.Role", err)
	}

	edited, err := c.Edited(role, changes)
	if err != nil {
		return View{}, errs.Wrap("c.Edited", err)
	}

	if err = s.clubs.Update(ctx, edited); err != nil {
		return View{}, errs.Wrap("s.clubs.Update", err)
	}

	return s.view(ctx, userID, clubID)
}

// Join makes userID a member ("Вступить в группу", "Я приду"); joining
// twice is fine. With maybe they only might come ("Возможно приду"),
// which only an event that has not started takes.
func (s *ClubService) Join(ctx context.Context, userID, clubID uuid.UUID, maybe bool) (View, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return View{}, errs.Wrap("s.clubs.Get", err)
	}

	if err = s.members.Join(ctx, clubID, userID, c.JoinsAsMaybe(maybe, s.now())); err != nil {
		return View{}, errs.Wrap("s.members.Join", err)
	}

	return s.view(ctx, userID, clubID)
}

// Leave takes userID out of the club ("Выйти из группы"), unless they own
// it; leaving a club one is not in is fine.
func (s *ClubService) Leave(ctx context.Context, userID, clubID uuid.UUID) (View, error) {
	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return View{}, errs.Wrap("s.members.Role", err)
	}

	if err = club.CheckLeave(role); err != nil {
		return View{}, errs.Wrap("club.CheckLeave", err)
	}

	if err = s.members.Leave(ctx, clubID, userID); err != nil {
		return View{}, errs.Wrap("s.members.Leave", err)
	}

	return s.view(ctx, userID, clubID)
}

// Appoint makes a member of the club an admin, or an admin a member
// again: the owner's call. The new admin hears about it, as in openvk;
// the dismissal takes the notification back.
func (s *ClubService) Appoint(ctx context.Context, requesterID, clubID, userID uuid.UUID, role club.Role) error {
	if _, err := s.clubs.Get(ctx, clubID); err != nil {
		return errs.Wrap("s.clubs.Get", err)
	}

	requester, err := s.members.Role(ctx, clubID, requesterID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	target, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	if err = club.CheckAppoint(requester, target); err != nil {
		return errs.Wrap("club.CheckAppoint", err)
	}

	if role == target {
		return nil
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.members.SetRole(ctx, clubID, userID, role); err != nil {
			return errs.Wrap("s.members.SetRole", err)
		}

		if role == club.RoleAdmin {
			return errs.Wrap("s.notifier.Appointed", s.notifier.Appointed(ctx, clubID, userID))
		}

		return errs.Wrap("s.notifier.Dismissed", s.notifier.Dismissed(ctx, clubID, userID))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Transfer hands the club over to one of its admins, openvk's "Назначить
// владельцем": the owner's call. The owner stays an admin.
func (s *ClubService) Transfer(ctx context.Context, requesterID, clubID, userID uuid.UUID) error {
	if _, err := s.clubs.Get(ctx, clubID); err != nil {
		return errs.Wrap("s.clubs.Get", err)
	}

	requester, err := s.members.Role(ctx, clubID, requesterID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	target, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	if err = club.CheckTransfer(requester, target); err != nil {
		return errs.Wrap("club.CheckTransfer", err)
	}

	// Roles may have changed since: the repository checks them again.
	return errs.Wrap("s.members.Transfer", s.members.Transfer(ctx, clubID, requesterID, userID))
}

// Describe gives userID, one of those who run the club, a title and hides
// or shows them ("Изменить комментарий" and "Скрыт" of openvk): those who
// run the club do it for each other.
func (s *ClubService) Describe(ctx context.Context, requesterID, clubID, userID uuid.UUID, title club.Title, hidden bool) error {
	if _, err := s.clubs.Get(ctx, clubID); err != nil {
		return errs.Wrap("s.clubs.Get", err)
	}

	requester, err := s.members.Role(ctx, clubID, requesterID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	target, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	if err = club.CheckDescribe(requester, target); err != nil {
		return errs.Wrap("club.CheckDescribe", err)
	}

	return errs.Wrap("s.members.Describe", s.members.Describe(ctx, clubID, userID, title, hidden))
}

// Members pages the members of a club as viewerID (uuid.Nil: anonymous)
// sees them, the newest first: those the club hides are plain members to
// everyone who does not run it. cursor is the last membership of the
// previous page.
func (s *ClubService) Members(ctx context.Context, viewerID, clubID, cursor uuid.UUID, size int, pick MemberPick) (MemberPage, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return MemberPage{}, errs.Wrap("s.clubs.Get", err)
	}

	viewer := club.RoleNone
	if viewerID != uuid.Nil {
		if viewer, err = s.members.Role(ctx, clubID, viewerID); err != nil {
			return MemberPage{}, errs.Wrap("s.members.Role", err)
		}
	}

	filter, total := memberFilter(c, pick, viewer)

	size = pageSize(size)

	// One extra tells whether there is a next page.
	members, err := s.members.List(ctx, clubID, cursor, size+1, filter)
	if err != nil {
		return MemberPage{}, errs.Wrap("s.members.List", err)
	}

	page := MemberPage{Members: members, Total: total}
	if len(members) > size {
		page.Members = members[:size]
		page.NextCursor = members[size-1].ID
	}

	for i := range page.Members {
		page.Members[i].Seat = page.Members[i].Shown(viewer)
	}

	if pick == PickAdmins {
		if page.Total, err = s.members.CountAdmins(ctx, clubID, filter == ShownManagers); err != nil {
			return MemberPage{}, errs.Wrap("s.members.CountAdmins", err)
		}
	}

	return page, nil
}

// memberFilter is how pick lists the members of c for someone who is
// viewer in it, and how many the club counts of them; the admins are
// counted apart.
func memberFilter(c *club.Club, pick MemberPick, viewer club.Role) (MemberFilter, int) {
	switch pick {
	case PickAdmins:
		if viewer.Manages() {
			return Managers, 0
		}

		return ShownManagers, 0
	case PickSure:
		return SureComers, c.Members() - c.Maybes()
	case PickMaybe:
		return MaybeComers, c.Maybes()
	default:
		return AllMembers, c.Members()
	}
}

// Clubs pages the clubs of a user ("Мои Группы", "Мои Встречи"), the
// newest joined first, as filter picks: only those they run
// ("Управление"), only groups or only events. Total counts them all.
func (s *ClubService) Clubs(ctx context.Context, userID, cursor uuid.UUID, size int, filter ClubFilter) (ClubPage, error) {
	size = pageSize(size)

	clubs, err := s.members.ListClubs(ctx, userID, cursor, size+1, filter)
	if err != nil {
		return ClubPage{}, errs.Wrap("s.members.ListClubs", err)
	}

	page := ClubPage{Clubs: clubs}
	if len(clubs) > size {
		page.Clubs = clubs[:size]
		page.NextCursor = clubs[size-1].ID
	}

	if page.Total, err = s.members.CountClubs(ctx, userID, filter); err != nil {
		return ClubPage{}, errs.Wrap("s.members.CountClubs", err)
	}

	return page, nil
}

// Search pages the clubs whose name or address has every word of query,
// the newest first.
func (s *ClubService) Search(ctx context.Context, query club.SearchQuery, cursor uuid.UUID, size int) (SearchPage, error) {
	size = pageSize(size)

	clubs, err := s.clubs.Search(ctx, query, cursor, size+1)
	if err != nil {
		return SearchPage{}, errs.Wrap("s.clubs.Search", err)
	}

	page := SearchPage{Clubs: clubs}
	if len(clubs) > size {
		page.Clubs = clubs[:size]
		page.NextCursor = clubs[size-1].ID()
	}

	return page, nil
}

// SetAvatar shows a photo as the club's, uuid.Nil none: its admins'
// call. The media service has stored it already.
func (s *ClubService) SetAvatar(ctx context.Context, userID, clubID, avatarID uuid.UUID) (View, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return View{}, errs.Wrap("s.clubs.Get", err)
	}

	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return View{}, errs.Wrap("s.members.Role", err)
	}

	changed, err := c.WithAvatar(role, avatarID)
	if err != nil {
		return View{}, errs.Wrap("c.WithAvatar", err)
	}

	if err = s.clubs.SetAvatar(ctx, changed); err != nil {
		return View{}, errs.Wrap("s.clubs.SetAvatar", err)
	}

	return s.view(ctx, userID, clubID)
}

// SetBackdrop shows photoID (uuid.Nil: none) on side of the club's page,
// "Фон страницы" of openvk: only its admins change it.
func (s *ClubService) SetBackdrop(ctx context.Context, userID, clubID uuid.UUID, side club.BackdropSide, photoID uuid.UUID) (View, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return View{}, errs.Wrap("s.clubs.Get", err)
	}

	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return View{}, errs.Wrap("s.members.Role", err)
	}

	changed, err := c.WithBackdrop(role, side, photoID)
	if err != nil {
		return View{}, errs.Wrap("c.WithBackdrop", err)
	}

	if err = s.clubs.SetBackdrop(ctx, changed, side); err != nil {
		return View{}, errs.Wrap("s.clubs.SetBackdrop", err)
	}

	return s.view(ctx, userID, clubID)
}

// Pin puts a club the user runs in their left menu, at most
// club.MaxPinned of them, or takes it out.
func (s *ClubService) Pin(ctx context.Context, userID, clubID uuid.UUID, pinned bool) error {
	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return errs.Wrap("s.members.Role", err)
	}

	if pinned {
		if err = club.CheckPin(role); err != nil {
			return errs.Wrap("club.CheckPin", err)
		}
	}

	ok, err := s.members.Pin(ctx, clubID, userID, pinned, club.MaxPinned)
	if err != nil {
		return errs.Wrap("s.members.Pin", err)
	}

	if !ok {
		return club.ErrTooManyPinned
	}

	return nil
}

// Pinned are the clubs in the user's left menu.
func (s *ClubService) Pinned(ctx context.Context, userID uuid.UUID) ([]*club.Club, error) {
	clubs, err := s.members.Pinned(ctx, userID)
	if err != nil {
		return nil, errs.Wrap("s.members.Pinned", err)
	}

	return clubs, nil
}

// ClubIDs are the clubs whose walls the feed of userID shows.
func (s *ClubService) ClubIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.members.ClubIDs(ctx, userID, MaxClubIDs)
	if err != nil {
		return nil, errs.Wrap("s.members.ClubIDs", err)
	}

	return ids, nil
}

// WallAccess is what a user may do on a club's wall: Post, or else
// Suggest a post for its admins to publish ("Предложка").
type WallAccess struct {
	Post    bool
	Suggest bool
}

// CanPostOnWall tells whether userID may write on the club's wall, on
// their own behalf or, fromClub, on the club's, or else suggest a post.
func (s *ClubService) CanPostOnWall(ctx context.Context, userID, clubID uuid.UUID, fromClub bool) (WallAccess, error) {
	c, err := s.clubs.Get(ctx, clubID)
	if err != nil {
		return WallAccess{}, errs.Wrap("s.clubs.Get", err)
	}

	role, err := s.members.Role(ctx, clubID, userID)
	if err != nil {
		return WallAccess{}, errs.Wrap("s.members.Role", err)
	}

	return WallAccess{Post: c.CanPostOnWall(role, fromClub), Suggest: c.CanSuggest(role)}, nil
}

// CanModerateWall tells whether userID keeps the wall of wallID clean:
// deletes and pins others' posts and comments there. Only a club's admins
// do; for any other wall, a user's, the answer is no.
func (s *ClubService) CanModerateWall(ctx context.Context, wallID, userID uuid.UUID) (bool, error) {
	role, err := s.members.Role(ctx, wallID, userID)
	if err != nil {
		return false, errs.Wrap("s.members.Role", err)
	}

	return role.Manages(), nil
}

// WallModerators are the owner and admins of the club wallID, the newest
// appointed first, up to MaxWallModerators: they hear about the posts
// suggested for its wall. Any other wall, a user's, has none.
func (s *ClubService) WallModerators(ctx context.Context, wallID uuid.UUID) ([]uuid.UUID, error) {
	admins, err := s.members.List(ctx, wallID, uuid.Nil, MaxWallModerators, Managers)
	if err != nil {
		return nil, errs.Wrap("s.members.List", err)
	}

	ids := make([]uuid.UUID, 0, len(admins))
	for _, m := range admins {
		ids = append(ids, m.UserID)
	}

	return ids, nil
}

// PurgeUser takes a user whose page was erased out of every club. A club
// they owned stays, with its admins but no owner.
func (s *ClubService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	return errs.Wrap("s.members.DeleteByUser", s.members.DeleteByUser(ctx, userID))
}

func pageSize(size int) int {
	if size <= 0 || size > MaxPageSize {
		return DefaultPageSize
	}

	return size
}

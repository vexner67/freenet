package club_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
)

func TestNewName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, input string
		want        club.Name
		err         error
	}{
		{"plain", "Любители котов", "Любители котов", nil},
		{"one line, spaces squeezed", "  Любители\n котов  ", "Любители котов", nil},
		{"empty", " \n ", "", club.ErrNameEmpty},
		{"at the limit", strings.Repeat("я", 64), club.Name(strings.Repeat("я", 64)), nil},
		{"too long", strings.Repeat("я", 65), "", club.ErrNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, err := club.NewName(tt.input); got != tt.want || !errors.Is(err, tt.err) {
				t.Fatalf("NewName = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewDescription(t *testing.T) {
	t.Parallel()

	if got, err := club.NewDescription("  про котов\r\nи кошек "); err != nil || got != "про котов\nи кошек" {
		t.Fatalf("NewDescription = %q, %v", got, err)
	}
	if got, err := club.NewDescription(""); err != nil || got != "" {
		t.Fatalf("empty = %q, %v", got, err)
	}
	if _, err := club.NewDescription(strings.Repeat("я", 1001)); !errors.Is(err, club.ErrDescriptionTooLong) {
		t.Fatalf("too long: %v", err)
	}
}

func TestNewAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		err   error
	}{
		{"", nil},
		{"cats", nil},
		{"Cats_Club2", nil},
		{"2cats", club.ErrAddressInvalid},
		{"cats_", club.ErrAddressInvalid},
		{"кошки", club.ErrAddressInvalid},
		{"01a11aab-708d-7645", club.ErrAddressInvalid},
		{strings.Repeat("a", 33), club.ErrAddressTooLong},
	}

	for _, tt := range tests {
		if got, err := club.NewAddress(tt.input); !errors.Is(err, tt.err) || (err == nil && got.String() != tt.input) {
			t.Errorf("NewAddress(%q) = %q, %v; want %v", tt.input, got, err, tt.err)
		}
	}
}

func TestNewWallAndRole(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"open", "suggest", "closed"} {
		if _, err := club.NewWall(s); err != nil {
			t.Errorf("NewWall(%q): %v", s, err)
		}
	}
	if _, err := club.NewWall("limited"); !errors.Is(err, club.ErrInvalidWall) {
		t.Errorf("unknown wall: %v", err)
	}

	for _, s := range []string{"member", "admin"} {
		if _, err := club.NewRole(s); err != nil {
			t.Errorf("NewRole(%q): %v", s, err)
		}
	}
	for _, s := range []string{"owner", "", "moderator"} {
		if _, err := club.NewRole(s); !errors.Is(err, club.ErrInvalidRole) {
			t.Errorf("NewRole(%q): %v; the owner is never given", s, err)
		}
	}
}

func newClub(t *testing.T) *club.Club {
	t.Helper()

	c, err := club.New("Любители котов", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestEdited(t *testing.T) {
	t.Parallel()

	c := newClub(t)
	if c.Wall() != club.WallOpen || c.Address() != "" || c.Website() != "" || c.AdminsDisplay() != club.AdminsDisplayCreator || c.Members() != 0 {
		t.Fatalf("new club = %+v; open, showing its creator, with no address", c)
	}

	changes := club.Changes{
		Name: "Кошки", Description: "мяу", Address: "cats", Website: "https://cats.example",
		Wall: club.WallClosed, AdminsDisplay: club.AdminsDisplayAll, OpenTopics: true, TopicsAboveWall: true,
	}

	for _, role := range []club.Role{club.RoleAdmin, club.RoleOwner} {
		edited, err := c.Edited(role, changes)
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if edited.Name() != "Кошки" || edited.Description() != "мяу" || edited.Address() != "cats" || edited.Wall() != club.WallClosed || edited.ID() != c.ID() ||
			edited.Website() != "https://cats.example" || edited.AdminsDisplay() != club.AdminsDisplayAll || !edited.OpenTopics() || !edited.TopicsAboveWall() {
			t.Fatalf("edited = %+v", edited)
		}
	}
	if c.Name() != "Любители котов" {
		t.Fatal("the original changed")
	}

	for _, role := range []club.Role{club.RoleMember, club.RoleNone} {
		if _, err := c.Edited(role, changes); !errors.Is(err, club.ErrCannotEdit) {
			t.Errorf("%q edits: %v", role, err)
		}
	}
}

func TestCanPostOnWall(t *testing.T) {
	t.Parallel()

	open := newClub(t)
	closed, err := open.Edited(club.RoleOwner, club.Changes{Name: open.Name(), Wall: club.WallClosed})
	if err != nil {
		t.Fatal(err)
	}

	suggest, err := open.Edited(club.RoleOwner, club.Changes{Name: open.Name(), Wall: club.WallSuggest})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		c        *club.Club
		role     club.Role
		fromClub bool
		want     bool
	}{
		{"suggest: a member", suggest, club.RoleMember, false, false},
		{"suggest: an admin", suggest, club.RoleAdmin, false, true},
		{"open: a stranger", open, club.RoleNone, false, true},
		{"open: a member", open, club.RoleMember, false, true},
		{"open: a member for the club", open, club.RoleMember, true, false},
		{"open: an admin for the club", open, club.RoleAdmin, true, true},
		{"closed: a member", closed, club.RoleMember, false, false},
		{"closed: an admin", closed, club.RoleAdmin, false, true},
		{"closed: the owner for the club", closed, club.RoleOwner, true, true},
	}

	for _, tt := range tests {
		if got := tt.c.CanPostOnWall(tt.role, tt.fromClub); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}

	// Only those who may not post suggest, and only on a "Предложка".
	for _, tt := range []struct {
		name string
		c    *club.Club
		role club.Role
		want bool
	}{
		{"suggest: a stranger", suggest, club.RoleNone, true},
		{"suggest: a member", suggest, club.RoleMember, true},
		{"suggest: an admin", suggest, club.RoleAdmin, false},
		{"suggest: the owner", suggest, club.RoleOwner, false},
		{"open: a member", open, club.RoleMember, false},
		{"closed: a member", closed, club.RoleMember, false},
	} {
		if got := tt.c.CanSuggest(tt.role); got != tt.want {
			t.Errorf("CanSuggest, %s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCanStartTopic(t *testing.T) {
	t.Parallel()

	c := newClub(t)
	if c.OpenTopics() || c.TopicsAboveWall() {
		t.Fatal("a new club lets only its admins start topics, listed beside the wall")
	}
	open, err := c.Edited(club.RoleOwner, club.Changes{Name: c.Name(), Wall: club.WallOpen, AdminsDisplay: club.AdminsDisplayCreator, OpenTopics: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name string
		c    *club.Club
		role club.Role
		want bool
	}{
		{"admins only: an admin", c, club.RoleAdmin, true},
		{"admins only: the owner", c, club.RoleOwner, true},
		{"admins only: a member", c, club.RoleMember, false},
		{"everyone: a stranger", open, club.RoleNone, true},
	} {
		if got := tt.c.CanStartTopic(tt.role); got != tt.want {
			t.Errorf("CanStartTopic, %s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCheckLeaveAndAppoint(t *testing.T) {
	t.Parallel()

	if err := club.CheckLeave(club.RoleOwner); !errors.Is(err, club.ErrOwnerCannotLeave) {
		t.Errorf("owner leaves: %v", err)
	}
	if err := club.CheckLeave(club.RoleAdmin); err != nil {
		t.Errorf("admin leaves: %v", err)
	}

	tests := []struct {
		requester, target club.Role
		err               error
	}{
		{club.RoleOwner, club.RoleMember, nil},
		{club.RoleOwner, club.RoleAdmin, nil},
		{club.RoleOwner, club.RoleOwner, club.ErrCannotAppoint},
		{club.RoleOwner, club.RoleNone, club.ErrNotMember},
		{club.RoleAdmin, club.RoleMember, club.ErrCannotAppoint},
		{club.RoleMember, club.RoleMember, club.ErrCannotAppoint},
	}

	for _, tt := range tests {
		if err := club.CheckAppoint(tt.requester, tt.target); !errors.Is(err, tt.err) {
			t.Errorf("%q appoints %q: %v, want %v", tt.requester, tt.target, err, tt.err)
		}
	}
}

func TestWithBackdrop(t *testing.T) {
	t.Parallel()

	c := newClub(t)
	left := uuid.New()

	changed, err := c.WithBackdrop(club.RoleAdmin, club.BackdropLeft, left)
	if err != nil || changed.Backdrop() != (club.Backdrop{Left: left}) || c.Backdrop() != (club.Backdrop{}) {
		t.Fatalf("WithBackdrop = %+v, %v", changed, err)
	}
	if _, err = c.WithBackdrop(club.RoleMember, club.BackdropLeft, left); !errors.Is(err, club.ErrCannotEdit) {
		t.Fatalf("a member: %v", err)
	}
	if _, err = c.WithBackdrop(club.RoleOwner, "top", left); !errors.Is(err, club.ErrInvalidBackdropSide) {
		t.Fatalf("a bad side: %v", err)
	}
}

func TestWithAvatarAndPin(t *testing.T) {
	t.Parallel()

	c := newClub(t)
	photo := uuid.New()

	changed, err := c.WithAvatar(club.RoleAdmin, photo)
	if err != nil || changed.AvatarID() != photo || c.AvatarID() != uuid.Nil {
		t.Fatalf("WithAvatar = %v, %v", changed, err)
	}
	if _, err = c.WithAvatar(club.RoleMember, photo); !errors.Is(err, club.ErrCannotEdit) {
		t.Fatalf("a member sets the photo: %v", err)
	}

	if err = club.CheckPin(club.RoleOwner); err != nil {
		t.Fatalf("the owner pins: %v", err)
	}
	if err = club.CheckPin(club.RoleMember); !errors.Is(err, club.ErrCannotPin) {
		t.Fatalf("a member pins: %v", err)
	}
}

func TestSearchQuery(t *testing.T) {
	t.Parallel()

	if _, err := club.NewSearchQuery("  "); !errors.Is(err, club.ErrSearchQueryEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := club.NewSearchQuery(strings.Repeat("я", 65)); !errors.Is(err, club.ErrSearchQueryTooLong) {
		t.Fatalf("too long: %v", err)
	}

	c, err := newClub(t).Edited(club.RoleOwner, club.Changes{Name: "Любители Котов", Address: "cats", Wall: club.WallOpen})
	if err != nil {
		t.Fatal(err)
	}

	for query, want := range map[string]bool{"котов": true, "КОТ любит": true, "cat": true, "собак": false, "котов собак": false} {
		q, err := club.NewSearchQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if got := q.Matches(c); got != want {
			t.Errorf("%q matches = %v, want %v", query, got, want)
		}
	}
}

func TestNewWebsite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  club.Website
		err   error
	}{
		{"", "", nil},
		{"  ", "", nil},
		{"cats.example", "https://cats.example", nil},
		{" http://cats.example/about?a=1 ", "http://cats.example/about?a=1", nil},
		{"https://кошки.рф", "https://кошки.рф", nil},
		{"javascript://alert(1)", "", club.ErrWebsiteInvalid},
		{"ftp://cats.example", "", club.ErrWebsiteInvalid},
		{"https://", "", club.ErrWebsiteInvalid},
		{"cats example", "", club.ErrWebsiteInvalid},
		{"https://me:secret@cats.example", "", club.ErrWebsiteInvalid},
		{"cats.example/" + strings.Repeat("я", 240), "", club.ErrWebsiteTooLong},
	}

	for _, tt := range tests {
		if got, err := club.NewWebsite(tt.input); got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("NewWebsite(%q) = %q, %v; want %q, %v", tt.input, got, err, tt.want, tt.err)
		}
	}
}

func TestNewAdminsDisplay(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"creator", "all", "none"} {
		if got, err := club.NewAdminsDisplay(s); err != nil || got.String() != s {
			t.Errorf("NewAdminsDisplay(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "admins", "ALL"} {
		if _, err := club.NewAdminsDisplay(s); !errors.Is(err, club.ErrInvalidAdminsDisplay) {
			t.Errorf("NewAdminsDisplay(%q): %v", s, err)
		}
	}
}

func TestCheckTransfer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		requester, target club.Role
		err               error
	}{
		{club.RoleOwner, club.RoleAdmin, nil},
		{club.RoleOwner, club.RoleMember, club.ErrNotAdmin},
		{club.RoleOwner, club.RoleNone, club.ErrNotAdmin},
		{club.RoleOwner, club.RoleOwner, club.ErrNotAdmin},
		{club.RoleAdmin, club.RoleAdmin, club.ErrCannotTransfer},
		{club.RoleMember, club.RoleAdmin, club.ErrCannotTransfer},
	}

	for _, tt := range tests {
		if err := club.CheckTransfer(tt.requester, tt.target); !errors.Is(err, tt.err) {
			t.Errorf("CheckTransfer(%q, %q) = %v; want %v", tt.requester, tt.target, err, tt.err)
		}
	}
}

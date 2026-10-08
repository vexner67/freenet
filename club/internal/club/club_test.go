package club_test

import (
	"errors"
	"strings"
	"testing"
	"time"

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

	for _, s := range []string{"open", "closed"} {
		if _, err := club.NewWall(s); err != nil {
			t.Errorf("NewWall(%q): %v", s, err)
		}
	}
	if _, err := club.NewWall("suggest"); !errors.Is(err, club.ErrInvalidWall) {
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
	if c.Wall() != club.WallOpen || c.Address() != "" || c.Members() != 0 {
		t.Fatalf("new club = %+v; open, with no address", c)
	}

	changes := club.Changes{Name: "Кошки", Description: "мяу", Address: "cats", Wall: club.WallClosed}

	for _, role := range []club.Role{club.RoleAdmin, club.RoleOwner} {
		edited, err := c.Edited(role, changes)
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if edited.Name() != "Кошки" || edited.Description() != "мяу" || edited.Address() != "cats" || edited.Wall() != club.WallClosed || edited.ID() != c.ID() {
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

	tests := []struct {
		name     string
		c        *club.Club
		role     club.Role
		fromClub bool
		want     bool
	}{
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

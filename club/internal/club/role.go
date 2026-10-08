package club

import "errors"

var ErrInvalidRole = errors.New("invalid role")

// Role is what a user is in a community. RoleNone is not a member.
type Role string

const (
	RoleNone   Role = ""
	RoleMember Role = "member"
	// RoleAdmin edits the community, writes on its behalf and keeps its
	// wall clean.
	RoleAdmin Role = "admin"
	// RoleOwner created the community: an admin who appoints the others
	// and cannot leave.
	RoleOwner Role = "owner"
)

// NewRole reads a role the owner may give: a member or an admin.
func NewRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleMember, RoleAdmin:
		return r, nil
	default:
		return "", ErrInvalidRole
	}
}

// Manages: an admin or the owner.
func (r Role) Manages() bool {
	return r == RoleAdmin || r == RoleOwner
}

func (r Role) String() string {
	return string(r)
}

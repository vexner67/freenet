package user

// IsVerified tells whether the administrators of the site confirmed the
// page: a checkmark by the name, openvk's "Подтверждённая страница".
func (u *User) IsVerified() bool {
	return u.verified
}

// WithVerified is u with the checkmark given or taken, for storage that
// keeps users whole.
func (u *User) WithVerified(verified bool) *User {
	changed := *u
	changed.verified = verified

	return &changed
}

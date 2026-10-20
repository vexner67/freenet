package user

// IsForeignAgent tells whether the administrators of the site marked the
// page as a foreign agent's: openvk's orange plaque ("user-alert") above
// the status.
func (u *User) IsForeignAgent() bool {
	return u.foreignAgent
}

// WithForeignAgent is u with the mark put or taken, for storage that
// keeps users whole.
func (u *User) WithForeignAgent(agent bool) *User {
	changed := *u
	changed.foreignAgent = agent

	return &changed
}

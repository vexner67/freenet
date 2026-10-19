package club

// IsVerified tells whether the administrators of the site confirmed the
// club: it wears the checkmark after its name, as a verified page does.
func (c *Club) IsVerified() bool {
	return c.verified
}

// WithVerified returns c confirmed by the administrators of the site, or
// no longer. Whether the caller is one of them is decided outside the
// club: the site's administrators are known to the user service.
func (c *Club) WithVerified(verified bool) *Club {
	changed := *c
	changed.verified = verified

	return &changed
}

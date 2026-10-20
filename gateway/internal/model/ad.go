package model

// Ad is a promoted post of the global feed: a link with a title, a text
// and a cover, which the administrators of the site make.
type Ad struct {
	ID    string
	URL   string
	Title string
	Text  string
	// PhotoOwnerID and PhotoID are the cover in media; empty for none.
	PhotoOwnerID string
	PhotoID      string
	Active       bool
}

// AdDraft is a new ad; PhotoID is an upload of the administrator, empty
// for no cover.
type AdDraft struct {
	URL     string
	Title   string
	Text    string
	PhotoID string
	Active  bool
}

// AdChange is what an edit of an ad says; nil leaves a part as it is, and
// a PhotoID of "" takes the cover away.
type AdChange struct {
	URL     *string
	Title   *string
	Text    *string
	PhotoID *string
	Active  *bool
}

// AdPhoto is a cover that no ad shows any more: its file is to go. Empty
// for none.
type AdPhoto struct {
	OwnerID string
	PhotoID string
}

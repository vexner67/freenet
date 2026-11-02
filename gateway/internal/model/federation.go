package model

// Federation is whether a page is open to the Fediverse: open, how many
// people of other servers follow it and the domain its address there ends
// with (@nickname@domain).
type Federation struct {
	Open      bool
	Followers int
	Domain    string
}

// FederationDocument is an ActivityPub document, passed on as it is, and
// the page a browser is sent to instead.
type FederationDocument struct {
	Body    []byte
	PageURL string
}

// InboxRequest is a request another server made to an inbox, as it came:
// its signature covers the method, the path and the headers.
type InboxRequest struct {
	Method string
	Path   string
	// Headers have their names in lower case.
	Headers map[string]string
	Body    []byte
	// UserID is the page whose inbox it is, "" for the shared inbox.
	UserID string
}

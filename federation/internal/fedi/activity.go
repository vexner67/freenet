package fedi

// Activity types the service reads and writes.
const (
	TypeFollow = "Follow"
	TypeAccept = "Accept"
	TypeUndo   = "Undo"
	TypeCreate = "Create"
	TypeUpdate = "Update"
	TypeDelete = "Delete"
	TypeReject = "Reject"
)

// Public is the audience of everyone: what is addressed to it may be
// shown to anyone.
const Public = "https://www.w3.org/ns/activitystreams#Public"

// Incoming is an activity another server sent to an inbox here, as far
// as the service reads it. Object is the object's ID; Inner is the
// activity an Undo undoes.
type Incoming struct {
	Type   string
	ID     string
	Actor  URL
	Object string
	Inner  *Incoming
	// Note is the note a Create or an Update makes, nil for others.
	Note *NoteDoc
}

// NewIncoming checks what every activity needs: who did it.
func NewIncoming(typ, id, actor, object string, inner *Incoming) (Incoming, error) {
	who, err := NewURL(actor)
	if err != nil || typ == "" {
		return Incoming{}, ErrInvalidActivity
	}

	return Incoming{Type: typ, ID: id, Actor: who, Object: object, Inner: inner}, nil
}

// DeletesActor: the actor tells it is deleted.
func (a Incoming) DeletesActor() bool {
	return a.Type == TypeDelete && URL(a.Object).WithoutFragment() == a.Actor
}

// Outgoing is an activity a page here sends. Exactly one of Note,
// Tombstone, Follow and Person is its object; Follow is the follow an
// Accept accepts or an Undo undoes.
type Outgoing struct {
	Type  string
	ID    URL
	Actor URL
	To    []string
	CC    []string
	Note  *Note
	// Tombstone is the ID of a deleted note.
	Tombstone URL
	// Follow is the follow an Accept accepts, as its sender told it.
	Follow *Incoming
	// Person is the actor the activity is about: the one deleted, or
	// the one followed.
	Person URL
}

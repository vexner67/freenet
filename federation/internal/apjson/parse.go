package apjson

import (
	"encoding/json"

	"github.com/vexner67/freenet/federation/internal/fedi"
)

// ref is a property that is either an ID or an object with one, as
// ActivityPub allows almost everywhere.
type ref struct {
	ID     string
	Type   string
	Actor  string
	Object *ref
}

func (r *ref) UnmarshalJSON(data []byte) error {
	var id string
	if json.Unmarshal(data, &id) == nil {
		r.ID = id
		return nil
	}

	var obj struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Actor  ref    `json:"actor"`
		Object *ref   `json:"object"`
	}

	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	r.ID, r.Type, r.Actor, r.Object = obj.ID, obj.Type, obj.Actor.ID, obj.Object

	return nil
}

// Incoming reads an activity another server sent. An Undo carries the
// activity it undoes; other embedded objects are known by their IDs.
func Incoming(body []byte) (fedi.Incoming, error) {
	var doc struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Actor  ref    `json:"actor"`
		Object *ref   `json:"object"`
	}

	if err := json.Unmarshal(body, &doc); err != nil {
		return fedi.Incoming{}, fedi.ErrInvalidActivity
	}

	var (
		object string
		inner  *fedi.Incoming
	)

	if doc.Object != nil {
		object = doc.Object.ID

		if doc.Type == fedi.TypeUndo && doc.Object.Type != "" {
			var target string
			if doc.Object.Object != nil {
				target = doc.Object.Object.ID
			}

			actor := doc.Object.Actor
			if actor == "" {
				actor = doc.Actor.ID
			}

			in, err := fedi.NewIncoming(doc.Object.Type, doc.Object.ID, actor, target, nil)
			if err != nil {
				return fedi.Incoming{}, err
			}
			inner = &in
		}
	}

	return fedi.NewIncoming(doc.Type, doc.ID, doc.Actor.ID, object, inner)
}

// Actor reads the document of a remote actor.
func Actor(body []byte) (fedi.RemoteDoc, error) {
	var doc struct {
		ID        string `json:"id"`
		Inbox     string `json:"inbox"`
		Endpoints struct {
			SharedInbox string `json:"sharedInbox"`
		} `json:"endpoints"`
		PublicKey struct {
			ID           string `json:"id"`
			PublicKeyPem string `json:"publicKeyPem"`
		} `json:"publicKey"`
	}

	if err := json.Unmarshal(body, &doc); err != nil {
		return fedi.RemoteDoc{}, fedi.ErrInvalidActivity
	}

	return fedi.RemoteDoc{
		ID:          doc.ID,
		Inbox:       doc.Inbox,
		SharedInbox: doc.Endpoints.SharedInbox,
		KeyID:       doc.PublicKey.ID,
		PublicKey:   doc.PublicKey.PublicKeyPem,
	}, nil
}

package fedi

import (
	"time"

	"github.com/google/uuid"
)

// Following is an account of another server the site follows: for the
// news of everyone when the administrators chose it (Featured), for the
// news of the people here who subscribed to it otherwise. One follow of
// the site serves them all.
type Following struct {
	actor     URL
	followID  uuid.UUID
	accepted  bool
	featured  bool
	createdAt time.Time
}

func NewFollowing(actor URL, featured bool, now time.Time) (*Following, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Following{actor: actor, followID: id, featured: featured, createdAt: now.UTC()}, nil
}

func RestoreFollowing(actor URL, followID uuid.UUID, accepted, featured bool, createdAt time.Time) *Following {
	return &Following{actor: actor, followID: followID, accepted: accepted, featured: featured, createdAt: createdAt}
}

func (f *Following) Actor() URL           { return f.actor }
func (f *Following) FollowID() uuid.UUID  { return f.followID }
func (f *Following) Accepted() bool       { return f.accepted }
func (f *Following) Featured() bool       { return f.featured }
func (f *Following) CreatedAt() time.Time { return f.createdAt }

// WithAccepted, WithFeatured are the follow changed.
func (f *Following) WithAccepted(accepted bool) *Following {
	c := *f
	c.accepted = accepted
	return &c
}

func (f *Following) WithFeatured(featured bool) *Following {
	c := *f
	c.featured = featured
	return &c
}

package user

import "errors"

type Relationship string

const (
	RelationshipNone    Relationship = ""
	RelationshipSingle  Relationship = "single"
	RelationshipDating  Relationship = "dating"
	RelationshipMarried Relationship = "married"
	RelationshipInLove  Relationship = "in_love"
	RelationshipLooking Relationship = "looking"
)

var ErrInvalidRelationship = errors.New("invalid relationship")

func NewRelationship(s string) (Relationship, error) {
	switch Relationship(s) {
	case RelationshipNone,
		RelationshipSingle,
		RelationshipDating,
		RelationshipMarried,
		RelationshipInLove,
		RelationshipLooking:
		return Relationship(s), nil
	default:
		return "", ErrInvalidRelationship
	}
}

func (r Relationship) String() string {
	return string(r)
}

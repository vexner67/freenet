package user

import "errors"

type Relationship string

const (
	RelationshipNone        Relationship = ""
	RelationshipSingle      Relationship = "single"
	RelationshipDating      Relationship = "dating"
	RelationshipEngaged     Relationship = "engaged"
	RelationshipMarried     Relationship = "married"
	RelationshipCivilUnion  Relationship = "civil_union"
	RelationshipInLove      Relationship = "in_love"
	RelationshipComplicated Relationship = "complicated"
	RelationshipLooking     Relationship = "looking"
)

var ErrInvalidRelationship = errors.New("invalid relationship")

func NewRelationship(s string) (Relationship, error) {
	switch Relationship(s) {
	case RelationshipNone,
		RelationshipSingle,
		RelationshipDating,
		RelationshipEngaged,
		RelationshipMarried,
		RelationshipCivilUnion,
		RelationshipInLove,
		RelationshipComplicated,
		RelationshipLooking:
		return Relationship(s), nil
	default:
		return "", ErrInvalidRelationship
	}
}

func (r Relationship) String() string {
	return string(r)
}

// TakesPartner tells whether the relationship names someone, as openvk's
// "Встречается с …", "Замужем за …": all but none, single and looking.
func (r Relationship) TakesPartner() bool {
	switch r {
	case RelationshipNone, RelationshipSingle, RelationshipLooking:
		return false
	default:
		return true
	}
}

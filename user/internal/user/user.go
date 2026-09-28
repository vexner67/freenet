package user

import (
	"github.com/google/uuid"
)

type User struct {
	id             uuid.UUID
	name           Name
	surname        Surname
	nickname       Nickname
	status         Status
	relationship   Relationship
	worldview      Worldview
	politicalViews PoliticalViews
	email          Email
	birthday       Birthday
	gender         Gender
}

func New(
	name Name,
	surname Surname,
	nickname Nickname,
	status Status,
	relationship Relationship,
	worldview Worldview,
	politicalViews PoliticalViews,
	email Email,
	birthday Birthday,
	gender Gender,
) (*User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &User{
		id:             id,
		name:           name,
		surname:        surname,
		nickname:       nickname,
		status:         status,
		relationship:   relationship,
		worldview:      worldview,
		politicalViews: politicalViews,
		email:          email,
		birthday:       birthday,
		gender:         gender,
	}, nil
}

func (u *User) ID() uuid.UUID {
	return u.id
}

func (u *User) Name() Name {
	return u.name
}

func (u *User) Surname() Surname {
	return u.surname
}

func (u *User) Nickname() Nickname {
	return u.nickname
}

func (u *User) Status() Status {
	return u.status
}

func (u *User) Relationship() Relationship {
	return u.relationship
}

func (u *User) Worldview() Worldview {
	return u.worldview
}

func (u *User) PoliticalViews() PoliticalViews {
	return u.politicalViews
}

func (u *User) Email() Email {
	return u.email
}

func (u *User) Birthday() Birthday {
	return u.birthday
}

func (u *User) Gender() Gender {
	return u.gender
}

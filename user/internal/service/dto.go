package service

import "github.com/vexner67/freenet/user/internal/user"

type CreateUserInput struct {
	Name           user.Name
	Surname        user.Surname
	Nickname       user.Nickname
	Status         user.Status
	Gender         user.Gender
	Relationship   user.Relationship
	Worldview      user.Worldview
	PoliticalViews user.PoliticalViews
	Birthday       user.Birthday
	Email          user.Email
}

package rest

import (
	"net/url"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const dateLayout = "2006-01-02"

type codeRequest struct {
	Email string `json:"email"`
}

type verifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type tokensResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func toTokensResponse(t model.Tokens) tokensResponse {
	return tokensResponse{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
	}
}

// profileJSON is both the body of PUT /me/profile (it replaces the whole
// profile) and the profile part of responses.
type profileJSON struct {
	Name           string  `json:"name"`
	Surname        string  `json:"surname"`
	Nickname       string  `json:"nickname"`
	Status         string  `json:"status"`
	Gender         string  `json:"gender"`
	Relationship   string  `json:"relationship"`
	Worldview      string  `json:"worldview"`
	PoliticalViews string  `json:"politicalViews"`
	Birthday       *string `json:"birthday"`
}

func (p profileJSON) toModel() (model.Profile, error) {
	var birthday time.Time

	if p.Birthday != nil && *p.Birthday != "" {
		t, err := time.Parse(dateLayout, *p.Birthday)
		if err != nil {
			return model.Profile{}, failure.New(failure.InvalidArgument, "birthday must be a date in YYYY-MM-DD format")
		}

		birthday = t
	}

	return model.Profile{
		Name:           p.Name,
		Surname:        p.Surname,
		Nickname:       p.Nickname,
		Status:         p.Status,
		Gender:         p.Gender,
		Relationship:   p.Relationship,
		Worldview:      p.Worldview,
		PoliticalViews: p.PoliticalViews,
		Birthday:       birthday,
	}, nil
}

func toProfileJSON(p model.Profile) profileJSON {
	var birthday *string

	if !p.Birthday.IsZero() {
		s := p.Birthday.UTC().Format(dateLayout)
		birthday = &s
	}

	return profileJSON{
		Name:           p.Name,
		Surname:        p.Surname,
		Nickname:       p.Nickname,
		Status:         p.Status,
		Gender:         p.Gender,
		Relationship:   p.Relationship,
		Worldview:      p.Worldview,
		PoliticalViews: p.PoliticalViews,
		Birthday:       birthday,
	}
}

type privacyJSON struct {
	ShowBirthday       bool `json:"showBirthday"`
	ShowRelationship   bool `json:"showRelationship"`
	ShowWorldview      bool `json:"showWorldview"`
	ShowPoliticalViews bool `json:"showPoliticalViews"`
}

func (p privacyJSON) toModel() model.Privacy {
	return model.Privacy{
		ShowBirthday:       p.ShowBirthday,
		ShowRelationship:   p.ShowRelationship,
		ShowWorldview:      p.ShowWorldview,
		ShowPoliticalViews: p.ShowPoliticalViews,
	}
}

// avatarJSON are the URLs of a photo, served by the gateway under /media.
type avatarJSON struct {
	Large string `json:"large"`
	Small string `json:"small"`
}

// toAvatarJSON is nil (null in JSON) without a photo. The avatar ID in the
// path makes the URLs change with the photo, so they are cached forever.
func toAvatarJSON(userID, avatarID string) *avatarJSON {
	if avatarID == "" {
		return nil
	}

	base := "/media/avatars/" + url.PathEscape(userID) + "/" + url.PathEscape(avatarID) + "/"

	return &avatarJSON{
		Large: base + largeImageFile,
		Small: base + smallImageFile,
	}
}

type userResponse struct {
	ID     string      `json:"id"`
	Avatar *avatarJSON `json:"avatar"`
	Email  string      `json:"email"`
	profileJSON
	ProfileCompleted bool        `json:"profileCompleted"`
	Privacy          privacyJSON `json:"privacy"`
}

func toUserResponse(u model.User) userResponse {
	return userResponse{
		ID:               u.ID,
		Avatar:           toAvatarJSON(u.ID, u.AvatarID),
		Email:            u.Email,
		profileJSON:      toProfileJSON(u.Profile),
		ProfileCompleted: u.ProfileCompleted,
		Privacy: privacyJSON{
			ShowBirthday:       u.Privacy.ShowBirthday,
			ShowRelationship:   u.Privacy.ShowRelationship,
			ShowWorldview:      u.Privacy.ShowWorldview,
			ShowPoliticalViews: u.Privacy.ShowPoliticalViews,
		},
	}
}

type publicProfileResponse struct {
	ID     string      `json:"id"`
	Avatar *avatarJSON `json:"avatar"`
	profileJSON
}

func toPublicProfileResponse(p model.PublicProfile) publicProfileResponse {
	return publicProfileResponse{
		ID:          p.ID,
		Avatar:      toAvatarJSON(p.ID, p.AvatarID),
		profileJSON: toProfileJSON(p.Profile),
	}
}

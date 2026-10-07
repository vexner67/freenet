package user

import (
	"github.com/google/uuid"
)

// Profile is what a user tells about themselves. Name and nickname are
// required once the profile is filled in; everything else is optional.
type Profile struct {
	Name           Name
	Surname        Surname
	Nickname       Nickname
	Status         Status
	Gender         Gender
	Relationship   Relationship
	Worldview      Worldview
	PoliticalViews PoliticalViews
	Birthday       Birthday
}

func (p Profile) validate() error {
	if p.Name == "" {
		return ErrNameEmpty
	}

	if p.Nickname == "" {
		return ErrNicknameEmpty
	}

	return nil
}

// Privacy says which sensitive fields other users may see and who may
// write on the wall. Worldview, political views, relationship and birthday
// are special categories of personal data, so all of them are hidden until
// the owner opens them.
type Privacy struct {
	ShowBirthday       bool
	ShowRelationship   bool
	ShowWorldview      bool
	ShowPoliticalViews bool
	WallWrite          WallWrite
	MessagesWrite      MessagesWrite
}

// PublicProfile is the part of a profile other users may see. Hidden
// fields hold their zero values ("not set"), so a viewer cannot tell
// a hidden field from an empty one. Email is never public.
type PublicProfile struct {
	ID uuid.UUID
	// AvatarID is uuid.Nil without a photo; photos are always public.
	AvatarID       uuid.UUID
	Name           Name
	Surname        Surname
	Nickname       Nickname
	Status         Status
	Gender         Gender
	Relationship   Relationship
	Worldview      Worldview
	PoliticalViews PoliticalViews
	Birthday       Birthday
}

type User struct {
	id       uuid.UUID
	email    Email
	profile  Profile
	privacy  Privacy
	avatarID uuid.UUID
}

// New creates a user with only an email set, as happens on the first
// sign-in; the profile is filled in later with UpdateProfile.
func New(email Email) (*User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &User{
		id:      id,
		email:   email,
		privacy: Privacy{WallWrite: DefaultWallWrite, MessagesWrite: DefaultMessagesWrite},
	}, nil
}

// Restore rebuilds a user from storage without validation or new IDs.
func Restore(id uuid.UUID, email Email, profile Profile, privacy Privacy, avatarID uuid.UUID) *User {
	return &User{
		id:       id,
		email:    email,
		profile:  profile,
		privacy:  privacy,
		avatarID: avatarID,
	}
}

// UpdateProfile replaces the whole profile.
func (u *User) UpdateProfile(profile Profile) error {
	if err := profile.validate(); err != nil {
		return err
	}

	u.profile = profile

	return nil
}

// UpdatePrivacy replaces all privacy settings; WallWrite and
// MessagesWrite must be set.
func (u *User) UpdatePrivacy(privacy Privacy) error {
	if _, err := NewWallWrite(privacy.WallWrite.String()); err != nil {
		return err
	}

	if _, err := NewMessagesWrite(privacy.MessagesWrite.String()); err != nil {
		return err
	}

	u.privacy = privacy

	return nil
}

// SetAvatar points to the photo stored by the media service; uuid.Nil
// removes it.
func (u *User) SetAvatar(avatarID uuid.UUID) {
	u.avatarID = avatarID
}

// IsProfileCompleted reports whether the user has filled in the profile
// after the first sign-in.
func (u *User) IsProfileCompleted() bool {
	return u.profile.Nickname != ""
}

func (u *User) PublicProfile() PublicProfile {
	p := PublicProfile{
		ID:       u.id,
		AvatarID: u.avatarID,
		Name:     u.profile.Name,
		Surname:  u.profile.Surname,
		Nickname: u.profile.Nickname,
		Status:   u.profile.Status,
		Gender:   u.profile.Gender,
	}

	if u.privacy.ShowBirthday {
		p.Birthday = u.profile.Birthday
	}

	if u.privacy.ShowRelationship {
		p.Relationship = u.profile.Relationship
	}

	if u.privacy.ShowWorldview {
		p.Worldview = u.profile.Worldview
	}

	if u.privacy.ShowPoliticalViews {
		p.PoliticalViews = u.profile.PoliticalViews
	}

	return p
}

func (u *User) ID() uuid.UUID {
	return u.id
}

func (u *User) Email() Email {
	return u.email
}

func (u *User) Profile() Profile {
	return u.profile
}

func (u *User) Privacy() Privacy {
	return u.privacy
}

func (u *User) AvatarID() uuid.UUID {
	return u.avatarID
}

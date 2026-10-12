package user

import (
	"time"

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
	// HideBirthYear shows only the day and the month of the birthday
	// ("Показывать только день и месяц").
	HideBirthYear bool
	Hometown      Place
	// Partner is whom the relationship names ("Встречается с …"),
	// uuid.Nil for nobody; only a relationship that TakesPartner has one.
	Partner uuid.UUID
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
	// ChatsInvite is "Кто может добавлять меня в чаты"; empty in
	// UpdatePrivacy, as from a client that does not know it, means the
	// default.
	ChatsInvite MessagesWrite
	ClubsRead   ClubsRead
	// PageRead is "Кому в интернете видно мою страницу": everyone or users;
	// InfoRead, PhotosRead and FriendsRead are "Кому видно основную
	// информацию моей страницы", "… мои фотографии" and "… моих друзей"
	// (the follows here).
	PageRead    Audience
	InfoRead    Audience
	PhotosRead  Audience
	FriendsRead Audience
	// OnlineRead is "Кто видит мою неактивность": who sees when the owner
	// was last online after a long absence (see LastSeen).
	OnlineRead Audience
	// NotesRead is "Кому видно мои заметки"; empty in UpdatePrivacy, as
	// from a client that does not know it, means everyone.
	NotesRead Audience
	// GiftsRead is "Кому видны мои подарки"; empty in UpdatePrivacy, as
	// from a client that does not know it, means everyone.
	GiftsRead Audience
	// Closed is openvk's "Тип профиля: закрытый": only the owner and the
	// people the owner follows see the page (see AccessFor).
	Closed bool
}

// PublicProfile is the part of a profile other users may see. Hidden
// fields hold their zero values ("not set"), so a viewer cannot tell
// a hidden field from an empty one. Email is never public. Of a deleted
// page only the name and the photo are left, as on openvk.
type PublicProfile struct {
	ID          uuid.UUID
	Deactivated bool
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
	// BirthYearHidden: Birthday is told without its year, which is then
	// birthYearMask.
	BirthYearHidden bool
	Hometown        Place
	// Partner is shown with the relationship, uuid.Nil when hidden too.
	Partner     uuid.UUID
	Contacts    Contacts
	Interests   Interests
	ExtraFields []ExtraField
	// PageHidden: the viewer may not see the page (Access.Page), only the
	// names and the photo are told; InfoHidden: nor its information
	// (Access.Info), which is left empty.
	PageHidden bool
	InfoHidden bool
	// Closed: the page is a closed one ("Закрытая страница"), told on the
	// hidden page too.
	Closed bool
	// LastSeen is when the person last visited ("был в сети"), the zero
	// time if never or hidden; Online: within OnlineFor ("Онлайн").
	LastSeen time.Time
	Online   bool
	// Backdrop is shown on the sides of the page ("Фон страницы").
	Backdrop Backdrop
	// Completeness is how full the page is, in percent; 0 when the page is
	// deleted or hidden. The parts it weighs may be hidden, the sum is not.
	Completeness int
}

// birthYearMask is the year of a birthday told without one: a leap year,
// so that February 29 stays a date.
const birthYearMask = 2000

// RegisteredAt is when the page was made ("Дата регистрации"): the time
// of its UUIDv7.
func RegisteredAt(id uuid.UUID) time.Time {
	sec, nsec := id.Time().UnixTime()

	return time.Unix(sec, nsec).UTC()
}

type User struct {
	id           uuid.UUID
	email        Email
	profile      Profile
	contacts     Contacts
	interests    Interests
	extraFields  []ExtraField
	privacy      Privacy
	avatarID     uuid.UUID
	deactivation Deactivation
	lastSeen     time.Time
	backdrop     Backdrop
	// ui is how the site looks to the user ("Интерфейс").
	ui Interface
	// wallet is changed by storage alone, never by saving the user.
	wallet Wallet
}

// New creates a user with only an email set, as happens on the first
// sign-in; the profile is filled in later with UpdateProfile.
func New(email Email) (*User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &User{
		id:    id,
		email: email,
		privacy: Privacy{
			WallWrite: DefaultWallWrite, MessagesWrite: DefaultMessagesWrite, ClubsRead: DefaultClubsRead,
			PageRead: AudienceEveryone, InfoRead: AudienceEveryone, PhotosRead: AudienceEveryone, FriendsRead: AudienceEveryone,
			OnlineRead: AudienceEveryone, NotesRead: AudienceEveryone, GiftsRead: AudienceEveryone,
			ChatsInvite: DefaultChatsInvite,
		},
		ui: DefaultInterface(),
	}, nil
}

// Stored is a user as storage keeps it, for Restore.
type Stored struct {
	ID           uuid.UUID
	Email        Email
	Profile      Profile
	Contacts     Contacts
	Interests    Interests
	ExtraFields  []ExtraField
	Privacy      Privacy
	AvatarID     uuid.UUID
	Deactivation Deactivation
	LastSeen     time.Time
	Backdrop     Backdrop
	Interface    Interface
	Wallet       Wallet
}

// Restore rebuilds a user from storage without validation or new IDs.
func Restore(s Stored) *User {
	return &User{
		id:           s.ID,
		email:        s.Email,
		profile:      s.Profile,
		contacts:     s.Contacts,
		interests:    s.Interests,
		extraFields:  s.ExtraFields,
		privacy:      s.Privacy,
		avatarID:     s.AvatarID,
		deactivation: s.Deactivation,
		lastSeen:     s.LastSeen,
		backdrop:     s.Backdrop,
		ui:           s.Interface,
		wallet:       s.Wallet,
	}
}

// UpdateProfile replaces the whole profile ("Основное"). A partner is kept
// only by a relationship that takes one, as openvk does.
func (u *User) UpdateProfile(profile Profile) error {
	if err := profile.validate(); err != nil {
		return err
	}

	if !profile.Relationship.TakesPartner() {
		profile.Partner = uuid.Nil
	}

	if profile.Partner == u.id {
		return ErrPartnerIsSelf
	}

	u.profile = profile

	return nil
}

// UpdateContacts replaces "Контактная информация".
func (u *User) UpdateContacts(contacts Contacts) {
	u.contacts = contacts
}

// UpdateInterests replaces "Личная информация".
func (u *User) UpdateInterests(interests Interests) {
	u.interests = interests
}

// UpdateExtraFields replaces "Дополнительно": up to MaxExtraFields.
func (u *User) UpdateExtraFields(fields []ExtraField) error {
	if len(fields) > MaxExtraFields {
		return ErrTooManyExtraFields
	}

	u.extraFields = fields

	return nil
}

// UpdatePrivacy replaces all privacy settings; all but the Show ones must
// be set.
func (u *User) UpdatePrivacy(privacy Privacy) error {
	if _, err := NewWallWrite(privacy.WallWrite.String()); err != nil {
		return err
	}

	if _, err := NewMessagesWrite(privacy.MessagesWrite.String()); err != nil {
		return err
	}

	if _, err := NewClubsRead(privacy.ClubsRead.String()); err != nil {
		return err
	}

	if _, err := NewPageRead(privacy.PageRead.String()); err != nil {
		return err
	}

	if privacy.NotesRead == "" {
		privacy.NotesRead = AudienceEveryone
	}

	if privacy.GiftsRead == "" {
		privacy.GiftsRead = AudienceEveryone
	}

	if privacy.ChatsInvite == "" {
		privacy.ChatsInvite = DefaultChatsInvite
	}

	if _, err := NewMessagesWrite(privacy.ChatsInvite.String()); err != nil {
		return err
	}

	for _, a := range []Audience{privacy.InfoRead, privacy.PhotosRead, privacy.FriendsRead, privacy.OnlineRead, privacy.NotesRead, privacy.GiftsRead} {
		if _, err := NewAudience(a.String()); err != nil {
			return err
		}
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

// Deactivate deletes the page, as its owner asks: restorable until
// keepFor has passed since now, then erased.
func (u *User) Deactivate(reason DeactivationReason, now time.Time, keepFor time.Duration) error {
	if u.IsDeactivated() {
		return ErrDeactivated
	}

	u.deactivation = Deactivation{At: now, PurgeAt: now.Add(keepFor), Reason: reason}

	return nil
}

// Reactivate restores a deleted page, until it is due to be erased.
func (u *User) Reactivate(now time.Time) error {
	if !u.IsDeactivated() {
		return ErrNotDeactivated
	}

	if !now.Before(u.deactivation.PurgeAt) {
		return ErrRestoreExpired
	}

	u.deactivation = Deactivation{}

	return nil
}

// IsDeactivated reports whether the owner has deleted the page.
func (u *User) IsDeactivated() bool {
	return !u.deactivation.At.IsZero()
}

func (u *User) Deactivation() Deactivation {
	return u.deactivation
}

// PublicProfile is what anyone may see of u without knowing who looks, as
// in lists: the information only when it is open to everyone.
func (u *User) PublicProfile() PublicProfile {
	return u.publicProfile(u.privacy.InfoRead == AudienceEveryone && !u.privacy.Closed)
}

// PublicProfileFor is the page of u as viewerID (uuid.Nil: anonymous),
// tied to u by t, sees it at now (see AccessFor), with when u was last
// online (see lastSeenFor).
func (u *User) PublicProfileFor(viewerID uuid.UUID, t Ties, now time.Time) PublicProfile {
	access := u.AccessFor(viewerID, t)
	if !access.Page {
		return PublicProfile{
			ID: u.id, AvatarID: u.avatarID, Name: u.profile.Name, Surname: u.profile.Surname,
			Nickname: u.profile.Nickname, Gender: u.profile.Gender, PageHidden: true, InfoHidden: true,
			Closed: u.privacy.Closed,
		}
	}

	p := u.publicProfile(access.Info)
	if p.LastSeen = u.lastSeenFor(viewerID, t, now); !p.LastSeen.IsZero() {
		p.Online = u.IsOnline(now)
	}

	if !u.IsDeactivated() {
		p.Completeness, _ = u.Completeness()
	}

	return p
}

func (u *User) publicProfile(showInfo bool) PublicProfile {
	if u.IsDeactivated() {
		return PublicProfile{
			ID:          u.id,
			Deactivated: true,
			AvatarID:    u.avatarID,
			Name:        u.profile.Name,
			Surname:     u.profile.Surname,
			Nickname:    u.profile.Nickname,
		}
	}

	p := PublicProfile{
		ID:       u.id,
		AvatarID: u.avatarID,
		Name:     u.profile.Name,
		Surname:  u.profile.Surname,
		Nickname: u.profile.Nickname,
		Status:   u.profile.Status,
		// The pronouns speak of the person: as on openvk, they stay with
		// the names.
		Gender:   u.profile.Gender,
		Backdrop: u.backdrop,
		Closed:   u.privacy.Closed,
	}

	if !showInfo {
		p.InfoHidden = true
		return p
	}

	p.Hometown = u.profile.Hometown
	p.Contacts = u.contacts
	p.Interests = u.interests
	p.ExtraFields = u.extraFields

	if u.privacy.ShowBirthday {
		p.Birthday = u.profile.Birthday
		if u.profile.HideBirthYear && !p.Birthday.Time().IsZero() {
			b := p.Birthday.Time()
			p.Birthday = Birthday(time.Date(birthYearMask, b.Month(), b.Day(), 0, 0, 0, 0, b.Location()))
			p.BirthYearHidden = true
		}
	}

	if u.privacy.ShowRelationship {
		p.Relationship = u.profile.Relationship
		p.Partner = u.profile.Partner
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

func (u *User) Contacts() Contacts {
	return u.contacts
}

func (u *User) Interests() Interests {
	return u.interests
}

func (u *User) ExtraFields() []ExtraField {
	return u.extraFields
}

func (u *User) Privacy() Privacy {
	return u.privacy
}

func (u *User) AvatarID() uuid.UUID {
	return u.avatarID
}

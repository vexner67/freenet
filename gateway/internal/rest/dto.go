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
	Name           string `json:"name"`
	Surname        string `json:"surname"`
	Nickname       string `json:"nickname"`
	Status         string `json:"status"`
	Gender         string `json:"gender"`
	Relationship   string `json:"relationship"`
	Worldview      string `json:"worldview"`
	PoliticalViews string `json:"politicalViews"`
	// Birthday is "YYYY-MM-DD", or "--MM-DD" when the owner shows only the
	// day and the month (in public profiles).
	Birthday      *string `json:"birthday"`
	HideBirthYear bool    `json:"hideBirthYear"`
	Hometown      string  `json:"hometown"`
}

// profileRequest is PUT /me/profile: the profile with the nickname of
// whom the relationship names, "" for nobody.
type profileRequest struct {
	profileJSON
	Partner string `json:"partner"`
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
		HideBirthYear:  p.HideBirthYear,
		Hometown:       p.Hometown,
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
		HideBirthYear:  p.HideBirthYear,
		Hometown:       p.Hometown,
	}
}

type contactsJSON struct {
	Email    string `json:"email"`
	Telegram string `json:"telegram"`
	Website  string `json:"website"`
	City     string `json:"city"`
	Address  string `json:"address"`
}

type interestsJSON struct {
	Interests string `json:"interests"`
	Music     string `json:"music"`
	Films     string `json:"films"`
	Shows     string `json:"shows"`
	Books     string `json:"books"`
	Quotes    string `json:"quotes"`
	Games     string `json:"games"`
	About     string `json:"about"`
}

// extraFieldJSON is a field of "Дополнительно"; place is "contacts" or
// "interests".
type extraFieldJSON struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	Place string `json:"place"`
}

type extraFieldsRequest struct {
	Fields []extraFieldJSON `json:"fields"`
}

// aboutJSON is the rest of the page and the partner of the relationship
// (null for nobody, when hidden, and when the partner's page is deleted).
type aboutJSON struct {
	Contacts     contactsJSON     `json:"contacts"`
	Interests    interestsJSON    `json:"interests"`
	Fields       []extraFieldJSON `json:"fields"`
	RegisteredAt string           `json:"registeredAt"`
	Partner      *authorJSON      `json:"partner"`
}

func toAboutJSON(a model.About, partner *authorJSON) aboutJSON {
	fields := make([]extraFieldJSON, 0, len(a.ExtraFields))
	for _, f := range a.ExtraFields {
		fields = append(fields, extraFieldJSON(f))
	}

	return aboutJSON{
		Contacts:     contactsJSON(a.Contacts),
		Interests:    interestsJSON(a.Interests),
		Fields:       fields,
		RegisteredAt: timeString(a.RegisteredAt),
		Partner:      partner,
	}
}

func (f extraFieldsRequest) toModel() []model.ExtraField {
	fields := make([]model.ExtraField, 0, len(f.Fields))
	for _, field := range f.Fields {
		fields = append(fields, model.ExtraField(field))
	}

	return fields
}

type privacyJSON struct {
	ShowBirthday       bool   `json:"showBirthday"`
	ShowRelationship   bool   `json:"showRelationship"`
	ShowWorldview      bool   `json:"showWorldview"`
	ShowPoliticalViews bool   `json:"showPoliticalViews"`
	WallWrite          string `json:"wallWrite"`
	MessagesWrite      string `json:"messagesWrite"`
	ClubsRead          string `json:"clubsRead"`
	PageRead           string `json:"pageRead"`
	InfoRead           string `json:"infoRead"`
	PhotosRead         string `json:"photosRead"`
	FriendsRead        string `json:"friendsRead"`
	OnlineRead         string `json:"onlineRead"`
	NotesRead          string `json:"notesRead"`
}

func (p privacyJSON) toModel() model.Privacy {
	return model.Privacy{
		ShowBirthday:       p.ShowBirthday,
		ShowRelationship:   p.ShowRelationship,
		ShowWorldview:      p.ShowWorldview,
		ShowPoliticalViews: p.ShowPoliticalViews,
		WallWrite:          p.WallWrite,
		MessagesWrite:      p.MessagesWrite,
		ClubsRead:          p.ClubsRead,
		PageRead:           p.PageRead,
		InfoRead:           p.InfoRead,
		PhotosRead:         p.PhotosRead,
		FriendsRead:        p.FriendsRead,
		OnlineRead:         p.OnlineRead,
		NotesRead:          p.NotesRead,
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
	// Deactivated: the owner deleted the page; PurgeAt is when it can no
	// longer be restored (RFC 3339, only for a deleted page).
	Deactivated bool   `json:"deactivated"`
	PurgeAt     string `json:"purgeAt,omitempty"`
	aboutJSON
	// Backdrop is "Фон страницы", null for none.
	Backdrop *backdropJSON `json:"backdrop"`
	// Completeness is "Заполненность страницы" in percent, Unfilled what
	// is missing for openvk's hints.
	Completeness int      `json:"completeness"`
	Unfilled     []string `json:"unfilled"`
	// Interface is how the site looks to the owner ("Интерфейс").
	Interface interfaceJSON `json:"interface"`
	// PinnedClubs are the clubs in the left menu ("Добавить в левое
	// меню"); GET /me alone tells them.
	PinnedClubs []clubJSON `json:"pinnedClubs,omitempty"`
}

// nsfwHide is the setting that leaves posts marked as adult out of the
// news.
const nsfwHide = "hide"

// interfaceJSON: avatarStyle is "square" or "round", mainPage "page" or
// "feed", hiddenMenu the links taken out of the left menu, theme
// "default", "blue" or "dark", nsfw "blur", "hide" or "show", postView
// "old" or "microblog", timezone minutes east of UTC or null for the
// browser's clock.
type interfaceJSON struct {
	AvatarStyle string   `json:"avatarStyle"`
	MainPage    string   `json:"mainPage"`
	HiddenMenu  []string `json:"hiddenMenu"`
	Theme       string   `json:"theme"`
	HideRating  bool     `json:"hideRating"`
	NSFW        string   `json:"nsfw"`
	PostView    string   `json:"postView"`
	Timezone    *int     `json:"timezone"`
}

// toUserResponse: partner is whom the relationship names, nil for nobody.
func toUserResponse(u model.User, partner *authorJSON) userResponse {
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
			WallWrite:          u.Privacy.WallWrite,
			MessagesWrite:      u.Privacy.MessagesWrite,
			ClubsRead:          u.Privacy.ClubsRead,
			PageRead:           u.Privacy.PageRead,
			InfoRead:           u.Privacy.InfoRead,
			PhotosRead:         u.Privacy.PhotosRead,
			FriendsRead:        u.Privacy.FriendsRead,
			OnlineRead:         u.Privacy.OnlineRead,
			NotesRead:          u.Privacy.NotesRead,
		},
		Deactivated: u.Deactivated,
		PurgeAt:     timeString(u.PurgeAt),
		aboutJSON:   toAboutJSON(u.About, partner),
		Backdrop:    toBackdropJSON(u.ID, u.Backdrop),

		Completeness: u.Completeness,
		Unfilled:     append([]string{}, u.Unfilled...),
		Interface: interfaceJSON{
			AvatarStyle: u.Interface.AvatarStyle,
			MainPage:    u.Interface.MainPage,
			HiddenMenu:  append([]string{}, u.Interface.HiddenMenu...),
			Theme:       u.Interface.Theme,
			HideRating:  u.Interface.HideRating,
			NSFW:        u.Interface.NSFW,
			PostView:    u.Interface.PostView,
			Timezone:    u.Interface.Timezone,
		},
	}
}

// timeString is "" for the zero time.
func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.UTC().Format(time.RFC3339)
}

type publicProfileResponse struct {
	ID     string      `json:"id"`
	Avatar *avatarJSON `json:"avatar"`
	profileJSON
	// Deactivated: the page is deleted; only names and the photo are left.
	Deactivated bool `json:"deactivated"`
	// PageHidden: the owner hides the page from the viewer ("Кому в
	// интернете видно мою страницу"), only the names and the photo are
	// told; InfoHidden: they hide its information, left empty.
	PageHidden bool `json:"pageHidden"`
	InfoHidden bool `json:"infoHidden"`
	// LastSeenAt is when the user last visited ("был в сети"), null if
	// never, hidden or in a list; Online: just now ("Онлайн"), which a
	// list tells too if the page is open to everyone.
	LastSeenAt *string `json:"lastSeenAt"`
	Online     bool    `json:"online"`
	// Backdrop is "Фон страницы", null for none.
	Backdrop *backdropJSON `json:"backdrop"`
	// Completeness is "Заполненность страницы" in percent; only on the page
	// itself, not in lists, and not on a deleted or hidden one.
	Completeness int `json:"completeness,omitempty"`
}

func toPublicProfileResponse(p model.PublicProfile) publicProfileResponse {
	profile := toProfileJSON(p.Profile)
	if p.BirthYearHidden && profile.Birthday != nil {
		monthDay := "--" + p.Profile.Birthday.UTC().Format("01-02")
		profile.Birthday = &monthDay
	}

	// A visitor learns what is shown, not how the owner chose it.
	profile.HideBirthYear = false

	return publicProfileResponse{
		ID:          p.ID,
		Avatar:      toAvatarJSON(p.ID, p.AvatarID),
		profileJSON: profile,
		Deactivated: p.Deactivated,
		PageHidden:  p.PageHidden,
		InfoHidden:  p.InfoHidden,
		LastSeenAt:  optionalTime(p.LastSeen),
		Online:      p.Online,
		Backdrop:    toBackdropJSON(p.ID, p.Backdrop),

		Completeness: p.Completeness,
	}
}

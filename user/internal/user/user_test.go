package user_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_UpdateProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile user.Profile
		want    error
	}{
		{"name and nickname", user.Profile{Name: "Ann", Nickname: "ann"}, nil},
		{"no name", user.Profile{Nickname: "ann"}, user.ErrNameEmpty},
		{"no nickname", user.Profile{Name: "Ann"}, user.ErrNicknameEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u, err := user.New("ann@example.com")
			if err != nil {
				t.Fatal(err)
			}

			err = u.UpdateProfile(tt.profile)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
			if u.IsProfileCompleted() != (tt.want == nil) {
				t.Fatalf("IsProfileCompleted = %v after err %v", u.IsProfileCompleted(), err)
			}
		})
	}
}

func TestUser_PublicProfile(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err = u.UpdateProfile(user.Profile{
		Name:           "Ann",
		Nickname:       "ann",
		Gender:         user.GenderFemale,
		Worldview:      user.WorldviewBuddhism,
		PoliticalViews: user.PoliticalViewsLiberal,
		Relationship:   user.RelationshipMarried,
	}); err != nil {
		t.Fatal(err)
	}

	got := u.PublicProfile()
	want := user.PublicProfile{
		ID:       u.ID(),
		Name:     "Ann",
		Nickname: "ann",
		Gender:   user.GenderFemale,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestUser_PublicProfile_Privacy(t *testing.T) {
	t.Parallel()

	birthday, err := user.NewBirthday(time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err = u.UpdateProfile(user.Profile{
		Name:           "Ann",
		Nickname:       "ann",
		Relationship:   user.RelationshipMarried,
		Worldview:      user.WorldviewBuddhism,
		PoliticalViews: user.PoliticalViewsLiberal,
		Birthday:       birthday,
	}); err != nil {
		t.Fatal(err)
	}

	if err = u.UpdatePrivacy(user.Privacy{ShowWorldview: true, ShowBirthday: true, WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone, PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone}); err != nil {
		t.Fatal(err)
	}
	got := u.PublicProfile()

	if got.Worldview != user.WorldviewBuddhism || got.Birthday != birthday {
		t.Fatalf("opened fields must be visible: %+v", got)
	}
	if got.Relationship != user.RelationshipNone || got.PoliticalViews != user.PoliticalViewsNone {
		t.Fatalf("closed fields must stay hidden: %+v", got)
	}
}

func TestUser_Partner(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	bob := uuid.New()

	if err := u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann", Relationship: user.RelationshipMarried, Partner: bob}); err != nil {
		t.Fatal(err)
	}
	if u.Profile().Partner != bob {
		t.Fatalf("partner = %v, want %v", u.Profile().Partner, bob)
	}

	// The relationship shows its partner only when shown itself.
	if p := u.PublicProfile(); p.Partner != uuid.Nil {
		t.Fatalf("hidden partner shown: %v", p.Partner)
	}
	_ = u.UpdatePrivacy(user.Privacy{ShowRelationship: true, WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone, PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone})
	if p := u.PublicProfile(); p.Partner != bob || p.Relationship != user.RelationshipMarried {
		t.Fatalf("partner = %v, relationship %q", p.Partner, p.Relationship)
	}

	// Single names nobody.
	if err := u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann", Relationship: user.RelationshipSingle, Partner: bob}); err != nil {
		t.Fatal(err)
	}
	if u.Profile().Partner != uuid.Nil {
		t.Fatal("a single has a partner")
	}

	if err := u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann", Relationship: user.RelationshipDating, Partner: u.ID()}); !errors.Is(err, user.ErrPartnerIsSelf) {
		t.Fatalf("dating oneself: %v", err)
	}
}

func TestUser_BirthYearHidden(t *testing.T) {
	t.Parallel()

	birthday, _ := user.NewBirthday(time.Date(1992, 2, 29, 0, 0, 0, 0, time.UTC))
	u, _ := user.New("ann@example.com")
	_ = u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann", Birthday: birthday, HideBirthYear: true})
	_ = u.UpdatePrivacy(user.Privacy{ShowBirthday: true, WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone, PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone})

	p := u.PublicProfile()
	if b := p.Birthday.Time(); !p.BirthYearHidden || b.Year() == 1992 || b.Month() != time.February || b.Day() != 29 {
		t.Fatalf("birthday = %v, hidden year %v", b, p.BirthYearHidden)
	}
}

func TestUser_AboutTheUser(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	_ = u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann", Hometown: "Тверь"})
	u.UpdateContacts(user.Contacts{Telegram: "ann_tg", City: "Москва"})
	u.UpdateInterests(user.Interests{Films: "Брат, Брат 2"})

	field, err := user.NewExtraField("Любимый цвет", "синий", user.FieldPlaceInterests)
	if err != nil {
		t.Fatal(err)
	}
	if err = u.UpdateExtraFields([]user.ExtraField{field}); err != nil {
		t.Fatal(err)
	}
	if err = u.UpdateExtraFields(make([]user.ExtraField, user.MaxExtraFields+1)); !errors.Is(err, user.ErrTooManyExtraFields) {
		t.Fatalf("8 fields: %v", err)
	}

	// Not special categories: anyone sees them.
	p := u.PublicProfile()
	if p.Hometown != "Тверь" || p.Contacts.City != "Москва" || p.Interests.Films != "Брат, Брат 2" || len(p.ExtraFields) != 1 {
		t.Fatalf("public = %+v", p)
	}

	if at := user.RegisteredAt(u.ID()); time.Since(at) > time.Minute || at.After(time.Now()) {
		t.Fatalf("RegisteredAt = %v", at)
	}
}

func TestNewTelegram(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"":                    "",
		"  @ann_tg ":          "ann_tg",
		"https://t.me/ann_tg": "ann_tg",
		"t.me/ann_tg":         "ann_tg",
		"ann":                 "!",
		"1annnn":              "!",
		"ann tg":              "!",
	} {
		got, err := user.NewTelegram(in)
		if want == "!" {
			if !errors.Is(err, user.ErrTelegramInvalid) {
				t.Errorf("NewTelegram(%q) = %q, %v; want ErrTelegramInvalid", in, got, err)
			}
			continue
		}
		if err != nil || got.String() != want {
			t.Errorf("NewTelegram(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestNewWebsite(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"":                    "",
		"example.com":         "https://example.com",
		"http://example.com/": "http://example.com/",
		"ftp://example.com":   "!",
		"https://":            "!",
		"exa mple.com":        "!",
	} {
		got, err := user.NewWebsite(in)
		if want == "!" {
			if err == nil {
				t.Errorf("NewWebsite(%q) = %q; want an error", in, got)
			}
			continue
		}
		if err != nil || got.String() != want {
			t.Errorf("NewWebsite(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	if _, err := user.NewWebsite("example.com/" + strings.Repeat("a", 256)); !errors.Is(err, user.ErrWebsiteTooLong) {
		t.Errorf("long website: %v", err)
	}
}

func TestProfileTexts(t *testing.T) {
	t.Parallel()

	if p, err := user.NewPlace("  Нижний   Новгород "); err != nil || p != "Нижний Новгород" {
		t.Errorf("NewPlace = %q, %v", p, err)
	}
	if _, err := user.NewPlace(strings.Repeat("я", 65)); !errors.Is(err, user.ErrPlaceTooLong) {
		t.Errorf("long place: %v", err)
	}
	if a, err := user.NewAbout(" строка\r\nвторая "); err != nil || a != "строка\nвторая" {
		t.Errorf("NewAbout = %q, %v", a, err)
	}
	if _, err := user.NewAbout(strings.Repeat("я", 1001)); !errors.Is(err, user.ErrAboutTooLong) {
		t.Errorf("long about: %v", err)
	}
	if e, err := user.NewContactEmail(" "); err != nil || e != "" {
		t.Errorf("empty contact email = %q, %v", e, err)
	}
	if _, err := user.NewContactEmail("nope"); !errors.Is(err, user.ErrEmailInvalid) {
		t.Errorf("bad contact email: %v", err)
	}
	if _, err := user.NewExtraField(" ", "x", user.FieldPlaceContacts); !errors.Is(err, user.ErrExtraFieldNameEmpty) {
		t.Errorf("unnamed field: %v", err)
	}
	if _, err := user.NewExtraField("Цвет", " ", user.FieldPlaceContacts); !errors.Is(err, user.ErrExtraFieldTextEmpty) {
		t.Errorf("empty field: %v", err)
	}
	if _, err := user.NewFieldPlace("wall"); !errors.Is(err, user.ErrInvalidExtraFieldPos) {
		t.Errorf("bad place: %v", err)
	}
	if !user.RelationshipCivilUnion.TakesPartner() || user.RelationshipLooking.TakesPartner() {
		t.Error("TakesPartner")
	}
	if g, err := user.NewGender("neutral"); err != nil || g != user.GenderNeutral {
		t.Errorf("neutral = %q, %v", g, err)
	}
}

func TestUser_AccessFor(t *testing.T) {
	t.Parallel()

	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann", Hometown: "Тверь"})
	ann.UpdateContacts(user.Contacts{City: "Москва"})

	bob := uuid.New()
	friend := user.Ties{Followed: true}

	set := func(page, info, photos, friends user.Audience, clubs user.ClubsRead) {
		t.Helper()
		if err := ann.UpdatePrivacy(user.Privacy{
			WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: clubs,
			PageRead: page, InfoRead: info, PhotosRead: photos, FriendsRead: friends, OnlineRead: user.AudienceEveryone,
			NotesRead: photos,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// By default everything is open, as on openvk.
	everything := user.Access{Page: true, Info: true, Photos: true, Friends: true, Clubs: true, Notes: true, Gifts: true}
	if got := ann.AccessFor(uuid.Nil, user.Ties{}); got != everything {
		t.Fatalf("default = %+v", got)
	}

	set(user.AudienceUsers, user.AudienceUsers, user.AudienceFollowing, user.AudienceOnlyMe, user.ClubsReadUsers)
	for _, tt := range []struct {
		name   string
		viewer uuid.UUID
		ties   user.Ties
		want   user.Access
	}{
		{"anonymous", uuid.Nil, user.Ties{}, user.Access{}},
		{"a user", bob, user.Ties{}, user.Access{Page: true, Info: true, Clubs: true, Gifts: true}},
		{"whom ann follows", bob, friend, user.Access{Page: true, Info: true, Photos: true, Clubs: true, Notes: true, Gifts: true}},
		{"ann", ann.ID(), user.Ties{}, everything},
	} {
		if got := ann.AccessFor(tt.viewer, tt.ties); got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, got, tt.want)
		}
	}

	// The page anonymous people may not see tells only the names.
	if p := ann.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()); !p.PageHidden || !p.InfoHidden || p.Hometown != "" || p.Name != "Ann" {
		t.Fatalf("hidden page = %+v", p)
	}
	// Lists do not know the viewer: the information open to users only is
	// not told there.
	if p := ann.PublicProfile(); !p.InfoHidden || p.Contacts.City != "" || p.Gender != ann.Profile().Gender {
		t.Fatalf("list profile = %+v", p)
	}

	set(user.AudienceEveryone, user.AudienceOnlyMe, user.AudienceEveryone, user.AudienceEveryone, user.ClubsReadEveryone)
	if p := ann.PublicProfileFor(bob, friend, time.Now()); p.PageHidden || !p.InfoHidden || p.Hometown != "" {
		t.Fatalf("info for ann only = %+v", p)
	}
	if p := ann.PublicProfileFor(ann.ID(), user.Ties{}, time.Now()); p.InfoHidden || p.Hometown != "Тверь" {
		t.Fatalf("ann's own = %+v", p)
	}

	if err := ann.UpdatePrivacy(user.Privacy{
		WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone,
		PageRead: user.AudienceFollowing, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone,
	}); !errors.Is(err, user.ErrInvalidPageRead) {
		t.Fatalf("a page for whom ann follows: %v", err)
	}
	if err := ann.UpdatePrivacy(user.Privacy{
		WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone,
		PageRead: user.AudienceEveryone, InfoRead: "friends", PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone,
	}); !errors.Is(err, user.ErrInvalidAudience) {
		t.Fatalf("an unknown audience: %v", err)
	}
	if err := ann.UpdatePrivacy(user.Privacy{
		WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone,
		PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone,
		NotesRead: "friends",
	}); !errors.Is(err, user.ErrInvalidAudience) {
		t.Fatalf("an unknown audience of the notes: %v", err)
	}

	// A client that does not know the notes leaves them to everyone.
	set(user.AudienceEveryone, user.AudienceEveryone, user.AudienceEveryone, user.AudienceEveryone, user.ClubsReadEveryone)
	if err := ann.UpdatePrivacy(user.Privacy{
		WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone,
		PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone,
	}); err != nil || ann.Privacy().NotesRead != user.AudienceEveryone {
		t.Fatalf("notes left out: %v, %q", err, ann.Privacy().NotesRead)
	}
}

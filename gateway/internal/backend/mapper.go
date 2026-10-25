package backend

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Proto enum values are PREFIX_NAME; the API uses lowercase name and ""
// for the zero ("not set") value.
const (
	genderPrefix         = "GENDER_"
	relationshipPrefix   = "RELATIONSHIP_"
	worldviewPrefix      = "WORLDVIEW_"
	politicalViewsPrefix = "POLITICAL_VIEWS_"
	wallWritePrefix      = "WALL_WRITE_"
	messagesWritePrefix  = "MESSAGES_WRITE_"
	clubsReadPrefix      = "CLUBS_READ_"
	fieldPlacePrefix     = "FIELD_PLACE_"
	audiencePrefix       = "AUDIENCE_"
	kindPrefix           = "KIND_"
	changePrefix         = "CHANGE_"

	notSet = "NONE"
)

func enumName(protoName, prefix string) string {
	name := strings.TrimPrefix(protoName, prefix)
	if name == notSet {
		return ""
	}

	return strings.ToLower(name)
}

// enumValue resolves an API name to a proto value; field is for the message.
func enumValue(name, prefix, field string, values map[string]int32) (int32, error) {
	if name == "" {
		return values[prefix+notSet], nil
	}

	v, ok := values[prefix+strings.ToUpper(name)]
	if !ok || name != strings.ToLower(name) || strings.EqualFold(name, notSet) {
		return 0, failure.New(failure.InvalidArgument, fmt.Sprintf("unknown %s %q", field, name))
	}

	return v, nil
}

// toUpdateProfileRequest: partner is the nickname of whom the
// relationship names, "" for nobody.
func toUpdateProfileRequest(p model.Profile, partner string) (*userv1.UpdateProfileRequest, error) {
	gender, err := enumValue(p.Gender, genderPrefix, "gender", userv1.Gender_value)
	if err != nil {
		return nil, err
	}

	relationship, err := enumValue(p.Relationship, relationshipPrefix, "relationship", userv1.Relationship_value)
	if err != nil {
		return nil, err
	}

	worldview, err := enumValue(p.Worldview, worldviewPrefix, "worldview", userv1.Worldview_value)
	if err != nil {
		return nil, err
	}

	politicalViews, err := enumValue(p.PoliticalViews, politicalViewsPrefix, "political views", userv1.PoliticalViews_value)
	if err != nil {
		return nil, err
	}

	return &userv1.UpdateProfileRequest{
		Name:           p.Name,
		Surname:        p.Surname,
		Nickname:       p.Nickname,
		Status:         p.Status,
		Gender:         userv1.Gender(gender),
		Relationship:   userv1.Relationship(relationship),
		Worldview:      userv1.Worldview(worldview),
		PoliticalViews: userv1.PoliticalViews(politicalViews),
		Birthday:       toTimestamp(p.Birthday),
		Hometown:       p.Hometown,
		HideBirthYear:  p.HideBirthYear,

		RelationshipPartner: partner,
	}, nil
}

// profileMessage is implemented by both User and PublicProfile.
type profileMessage interface {
	GetName() string
	GetSurname() string
	GetNickname() string
	GetStatus() string
	GetGender() userv1.Gender
	GetRelationship() userv1.Relationship
	GetWorldview() userv1.Worldview
	GetPoliticalViews() userv1.PoliticalViews
	GetBirthday() *timestamppb.Timestamp
	GetHometown() string
	GetRelationshipPartnerId() string
	GetContacts() *userv1.Contacts
	GetInterests() *userv1.Interests
	GetExtraFields() []*userv1.ExtraField
	GetRegisteredAt() *timestamppb.Timestamp
}

func toProfile(m profileMessage) model.Profile {
	return model.Profile{
		Name:           m.GetName(),
		Surname:        m.GetSurname(),
		Nickname:       m.GetNickname(),
		Status:         m.GetStatus(),
		Gender:         enumName(m.GetGender().String(), genderPrefix),
		Relationship:   enumName(m.GetRelationship().String(), relationshipPrefix),
		Worldview:      enumName(m.GetWorldview().String(), worldviewPrefix),
		PoliticalViews: enumName(m.GetPoliticalViews().String(), politicalViewsPrefix),
		Birthday:       fromTimestamp(m.GetBirthday()),
		Hometown:       m.GetHometown(),
		PartnerID:      m.GetRelationshipPartnerId(),
	}
}

func toAbout(m profileMessage) model.About {
	c, in := m.GetContacts(), m.GetInterests()

	about := model.About{
		Contacts: model.Contacts{
			Email: c.GetEmail(), Telegram: c.GetTelegram(), Website: c.GetWebsite(), City: c.GetCity(), Address: c.GetAddress(),
		},
		Interests: model.Interests{
			Interests: in.GetInterests(), Music: in.GetMusic(), Films: in.GetFilms(), Shows: in.GetShows(),
			Books: in.GetBooks(), Quotes: in.GetQuotes(), Games: in.GetGames(), About: in.GetAbout(),
		},
		RegisteredAt: fromTimestamp(m.GetRegisteredAt()),
	}

	for _, f := range m.GetExtraFields() {
		about.ExtraFields = append(about.ExtraFields, model.ExtraField{
			Name: f.GetName(), Text: f.GetText(), Place: enumName(f.GetPlace().String(), fieldPlacePrefix),
		})
	}

	return about
}

func toProtoContacts(c model.Contacts) *userv1.Contacts {
	return &userv1.Contacts{Email: c.Email, Telegram: c.Telegram, Website: c.Website, City: c.City, Address: c.Address}
}

func toProtoInterests(in model.Interests) *userv1.Interests {
	return &userv1.Interests{
		Interests: in.Interests, Music: in.Music, Films: in.Films, Shows: in.Shows,
		Books: in.Books, Quotes: in.Quotes, Games: in.Games, About: in.About,
	}
}

func toProtoExtraFields(fields []model.ExtraField) ([]*userv1.ExtraField, error) {
	out := make([]*userv1.ExtraField, 0, len(fields))
	for _, f := range fields {
		place, err := enumValue(f.Place, fieldPlacePrefix, "field place", userv1.FieldPlace_value)
		if err != nil {
			return nil, err
		}
		out = append(out, &userv1.ExtraField{Name: f.Name, Text: f.Text, Place: userv1.FieldPlace(place)})
	}

	return out, nil
}

func toUser(u *userv1.User) model.User {
	return model.User{
		ID:               u.GetId(),
		AvatarID:         u.GetAvatarId(),
		Email:            u.GetEmail(),
		Profile:          withHiddenYear(toProfile(u), u.GetHideBirthYear()),
		About:            toAbout(u),
		ProfileCompleted: u.GetProfileCompleted(),
		Deactivated:      u.GetDeactivated(),
		Backdrop:         model.Backdrop{Left: u.GetBackdropLeft(), Right: u.GetBackdropRight()},
		Completeness:     int(u.GetCompleteness()),
		Unfilled:         u.GetUnfilled(),
		Votes:            u.GetVotes(),
		Rating:           int(u.GetRating()),
		Admin:            u.GetAdmin(),
		Verified:         u.GetVerified(),
		Ban:              toBan(u.GetBan()),
		Alert:            u.GetAlert(),
		PrimeUntil:       fromTimestamp(u.GetPrimeUntil()),
		PrimeInvisible:   u.GetPrimeInvisible(),
		PrimeStyle:       toPrimeStyle(u.GetPrimeStyle()),
		Interface: model.Interface{
			AvatarStyle: u.GetInterface().GetAvatarStyle(),
			MainPage:    u.GetInterface().GetMainPage(),
			HiddenMenu:  u.GetInterface().GetHiddenMenu(),
			Theme:       u.GetInterface().GetTheme(),
			HideRating:  u.GetInterface().GetHideRating(),
			NSFW:        u.GetInterface().GetNsfw(),
			PostView:    u.GetInterface().GetPostView(),
			Timezone:    fromTimezoneOffset(u.GetInterface()),
		},
		PurgeAt: fromTimestamp(u.GetPurgeAt()),
		Privacy: model.Privacy{
			ShowBirthday:       u.GetPrivacy().GetShowBirthday(),
			ShowRelationship:   u.GetPrivacy().GetShowRelationship(),
			ShowWorldview:      u.GetPrivacy().GetShowWorldview(),
			ShowPoliticalViews: u.GetPrivacy().GetShowPoliticalViews(),
			WallWrite:          enumName(u.GetPrivacy().GetWallWrite().String(), wallWritePrefix),
			MessagesWrite:      enumName(u.GetPrivacy().GetMessagesWrite().String(), messagesWritePrefix),
			ClubsRead:          enumName(u.GetPrivacy().GetClubsRead().String(), clubsReadPrefix),
			PageRead:           enumName(u.GetPrivacy().GetPageRead().String(), audiencePrefix),
			InfoRead:           enumName(u.GetPrivacy().GetInfoRead().String(), audiencePrefix),
			PhotosRead:         enumName(u.GetPrivacy().GetPhotosRead().String(), audiencePrefix),
			FriendsRead:        enumName(u.GetPrivacy().GetFriendsRead().String(), audiencePrefix),
			OnlineRead:         enumName(u.GetPrivacy().GetOnlineRead().String(), audiencePrefix),
			GiftsRead:          enumName(u.GetPrivacy().GetGiftsRead().String(), audiencePrefix),
			ChatsInvite:        enumName(u.GetPrivacy().GetChatsInvite().String(), messagesWritePrefix),
			Closed:             u.GetPrivacy().GetClosed(),
			MessagesPrime:      u.GetPrivacy().GetMessagesPrime(),
			MessagePrice:       u.GetPrivacy().GetMessagePrice(),
		},
	}
}

func withHiddenYear(p model.Profile, hide bool) model.Profile {
	p.HideBirthYear = hide
	return p
}

func toTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

func fromTimestamp(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}

	return ts.AsTime()
}

// toTimezoneOffset and fromTimezoneOffset: nil is the browser's clock.
func toTimezoneOffset(minutes *int) *int32 {
	if minutes == nil {
		return nil
	}

	// The user service checks the range; clamp what int32 cannot hold.
	offset := int32(max(min(*minutes, math.MaxInt16), math.MinInt16))

	return &offset
}

func fromTimezoneOffset(ui *userv1.InterfaceSettings) *int {
	if ui == nil || ui.TimezoneOffset == nil {
		return nil
	}

	minutes := int(ui.GetTimezoneOffset())

	return &minutes
}

// toBan is nil for a page that is not blocked.
func toBan(b *userv1.Ban) *model.Ban {
	if b == nil {
		return nil
	}

	return &model.Ban{Reason: b.GetReason(), At: fromTimestamp(b.GetBannedAt()), Until: fromTimestamp(b.GetUntil())}
}

func toPrimeStyle(s *userv1.PrimeStyle) model.PrimeStyle {
	return model.PrimeStyle{Name: s.GetName(), Frame: s.GetFrame(), Background: s.GetBackground(), Mood: s.GetMood()}
}

// toShownPrimeStyle is nil when the page tells no style.
func toShownPrimeStyle(s *userv1.PrimeStyle) *model.PrimeStyle {
	if s == nil {
		return nil
	}

	style := toPrimeStyle(s)

	return &style
}

package grpc

import (
	"cmp"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoUser(u *user.User) *userv1.User {
	p := u.Profile()
	completeness, unfilled := u.Completeness()

	return &userv1.User{
		Id:                    u.ID().String(),
		Name:                  p.Name.String(),
		Surname:               p.Surname.String(),
		Nickname:              p.Nickname.String(),
		Status:                p.Status.String(),
		Gender:                toProtoGender(p.Gender),
		Relationship:          toProtoRelationship(p.Relationship),
		Worldview:             toProtoWorldview(p.Worldview),
		PoliticalViews:        toProtoPoliticalViews(p.PoliticalViews),
		Birthday:              toProtoBirthday(p.Birthday),
		Email:                 u.Email().String(),
		ProfileCompleted:      u.IsProfileCompleted(),
		Privacy:               toProtoPrivacy(u.Privacy()),
		AvatarId:              avatarString(u.AvatarID()),
		Deactivated:           u.IsDeactivated(),
		PurgeAt:               toProtoTime(u.Deactivation().PurgeAt),
		Hometown:              p.Hometown.String(),
		RelationshipPartnerId: avatarString(p.Partner),
		HideBirthYear:         p.HideBirthYear,
		Contacts:              toProtoContacts(u.Contacts()),
		Interests:             toProtoInterests(u.Interests()),
		ExtraFields:           toProtoExtraFields(u.ExtraFields()),
		RegisteredAt:          timestamppb.New(user.RegisteredAt(u.ID())),
		BackdropLeft:          avatarString(u.Backdrop().Left),
		BackdropRight:         avatarString(u.Backdrop().Right),
		Interface:             toProtoInterface(u.Interface()),
		Completeness:          int32(completeness), //nolint:gosec // a percent
		Unfilled:              unfilled,
		Votes:                 u.Wallet().Votes,
		Rating:                int32(u.Wallet().Rating), //nolint:gosec // a percent
		Verified:              u.IsVerified(),
		Ban:                   toProtoBan(u.BanAt(time.Now())),
		Alert:                 u.Alert().String(),
		PrimeUntil:            toProtoPrimeUntil(u, time.Now()),
		PrimeInvisible:        u.Prime().Invisible,
		PrimeStyle:            toProtoPrimeStyle(u.PrimeStyle()),
	}
}

func toProtoPrimeStyle(s user.PrimeStyle) *userv1.PrimeStyle {
	return &userv1.PrimeStyle{
		Name:       string(s.Name),
		Frame:      string(s.Frame),
		Background: string(s.Background),
		Mood:       string(s.Mood),
	}
}

// toProtoPrimeUntil is nil unless u has Prime now.
func toProtoPrimeUntil(u *user.User, now time.Time) *timestamppb.Timestamp {
	if !u.IsPrimeAt(now) {
		return nil
	}

	return timestamppb.New(u.Prime().Until)
}

// toProtoBan is nil for a page that is not blocked.
func toProtoBan(b user.Ban) *userv1.Ban {
	if b.At.IsZero() {
		return nil
	}

	return &userv1.Ban{Reason: b.Reason.String(), BannedAt: timestamppb.New(b.At), Until: toProtoTime(b.Until)}
}

func toProtoContacts(c user.Contacts) *userv1.Contacts {
	return &userv1.Contacts{
		Email:    c.Email.String(),
		Telegram: c.Telegram.String(),
		Website:  c.Website.String(),
		City:     c.City.String(),
		Address:  c.Address.String(),
	}
}

// fromProtoContacts validates every field; its errors are meant for the client.
func fromProtoContacts(c *userv1.Contacts) (user.Contacts, error) {
	email, err := user.NewContactEmail(c.GetEmail())
	if err != nil {
		return user.Contacts{}, err
	}

	telegram, err := user.NewTelegram(c.GetTelegram())
	if err != nil {
		return user.Contacts{}, err
	}

	website, err := user.NewWebsite(c.GetWebsite())
	if err != nil {
		return user.Contacts{}, err
	}

	city, err := user.NewPlace(c.GetCity())
	if err != nil {
		return user.Contacts{}, err
	}

	address, err := user.NewAddress(c.GetAddress())
	if err != nil {
		return user.Contacts{}, err
	}

	return user.Contacts{Email: email, Telegram: telegram, Website: website, City: city, Address: address}, nil
}

func toProtoInterests(in user.Interests) *userv1.Interests {
	return &userv1.Interests{
		Interests: in.Interests.String(),
		Music:     in.Music.String(),
		Films:     in.Films.String(),
		Shows:     in.Shows.String(),
		Books:     in.Books.String(),
		Quotes:    in.Quotes.String(),
		Games:     in.Games.String(),
		About:     in.About.String(),
	}
}

// fromProtoInterests validates every field; its errors are meant for the client.
func fromProtoInterests(in *userv1.Interests) (user.Interests, error) {
	var out user.Interests

	for _, f := range []struct {
		to   *user.About
		from string
	}{
		{&out.Interests, in.GetInterests()},
		{&out.Music, in.GetMusic()},
		{&out.Films, in.GetFilms()},
		{&out.Shows, in.GetShows()},
		{&out.Books, in.GetBooks()},
		{&out.Quotes, in.GetQuotes()},
		{&out.Games, in.GetGames()},
		{&out.About, in.GetAbout()},
	} {
		about, err := user.NewAbout(f.from)
		if err != nil {
			return user.Interests{}, err
		}
		*f.to = about
	}

	return out, nil
}

func toProtoExtraFields(fields []user.ExtraField) []*userv1.ExtraField {
	out := make([]*userv1.ExtraField, 0, len(fields))
	for _, f := range fields {
		place := userv1.FieldPlace_FIELD_PLACE_CONTACTS
		if f.Place == user.FieldPlaceInterests {
			place = userv1.FieldPlace_FIELD_PLACE_INTERESTS
		}
		out = append(out, &userv1.ExtraField{Name: f.Name, Text: f.Text.String(), Place: place})
	}

	return out
}

// fromProtoExtraFields validates every field; its errors are meant for the client.
func fromProtoExtraFields(fields []*userv1.ExtraField) ([]user.ExtraField, error) {
	if len(fields) > user.MaxExtraFields {
		return nil, user.ErrTooManyExtraFields
	}

	out := make([]user.ExtraField, 0, len(fields))
	for _, f := range fields {
		var place string
		switch f.GetPlace() {
		case userv1.FieldPlace_FIELD_PLACE_CONTACTS:
			place = user.FieldPlaceContacts.String()
		case userv1.FieldPlace_FIELD_PLACE_INTERESTS:
			place = user.FieldPlaceInterests.String()
		}

		fp, err := user.NewFieldPlace(place)
		if err != nil {
			return nil, err
		}

		field, err := user.NewExtraField(f.GetName(), f.GetText(), fp)
		if err != nil {
			return nil, err
		}
		out = append(out, field)
	}

	return out, nil
}

// toProtoTime leaves the zero time unset.
func toProtoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

// avatarString is "" for no photo, as the proto field says.
func avatarString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}

func toProtoPrivacy(p user.Privacy) *userv1.PrivacySettings {
	return &userv1.PrivacySettings{
		ShowBirthday:       p.ShowBirthday,
		ShowRelationship:   p.ShowRelationship,
		ShowWorldview:      p.ShowWorldview,
		ShowPoliticalViews: p.ShowPoliticalViews,
		WallWrite:          toProtoWallWrite(p.WallWrite),
		MessagesWrite:      toProtoMessagesWrite(p.MessagesWrite),
		ClubsRead:          toProtoClubsRead(p.ClubsRead),
		PageRead:           toProtoAudience(p.PageRead),
		InfoRead:           toProtoAudience(p.InfoRead),
		PhotosRead:         toProtoAudience(p.PhotosRead),
		FriendsRead:        toProtoAudience(p.FriendsRead),
		OnlineRead:         toProtoAudience(p.OnlineRead),
		GiftsRead:          toProtoAudience(p.GiftsRead),
		Closed:             p.Closed,
		ChatsInvite:        toProtoMessagesWrite(p.ChatsInvite),
		MessagesPrime:      p.MessagesPrime,
		MessagePrice:       p.MessagePrice.Int64(),
	}
}

var audiences = map[user.Audience]userv1.Audience{
	user.AudienceEveryone:  userv1.Audience_AUDIENCE_EVERYONE,
	user.AudienceUsers:     userv1.Audience_AUDIENCE_USERS,
	user.AudienceFollowing: userv1.Audience_AUDIENCE_FOLLOWING,
	user.AudienceOnlyMe:    userv1.Audience_AUDIENCE_ONLY_ME,
}

func toProtoAudience(a user.Audience) userv1.Audience {
	return audiences[a]
}

// fromProtoAudience is "" for an unknown value, which the domain refuses.
func fromProtoAudience(a userv1.Audience) string {
	for domain, proto := range audiences {
		if proto == a {
			return domain.String()
		}
	}

	return ""
}

// fromProtoPrivacy validates the settings; its errors are meant for the client.
func fromProtoPrivacy(p *userv1.PrivacySettings) (user.Privacy, error) {
	wallWrite, err := user.NewWallWrite(fromProtoWallWrite(p.GetWallWrite()))
	if err != nil {
		return user.Privacy{}, err
	}

	messagesWrite, err := user.NewMessagesWrite(fromProtoMessagesWrite(p.GetMessagesWrite()))
	if err != nil {
		return user.Privacy{}, err
	}

	clubsRead, err := user.NewClubsRead(fromProtoClubsRead(p.GetClubsRead()))
	if err != nil {
		return user.Privacy{}, err
	}

	pageRead, err := user.NewPageRead(fromProtoAudience(p.GetPageRead()))
	if err != nil {
		return user.Privacy{}, err
	}

	var reads [4]user.Audience
	for i, a := range []userv1.Audience{p.GetInfoRead(), p.GetPhotosRead(), p.GetFriendsRead(), p.GetOnlineRead()} {
		if reads[i], err = user.NewAudience(fromProtoAudience(a)); err != nil {
			return user.Privacy{}, err
		}
	}

	// A client that does not know the gifts leaves them to everyone.
	giftsRead := user.AudienceEveryone
	if p.GetGiftsRead() != userv1.Audience_AUDIENCE_UNSPECIFIED {
		if giftsRead, err = user.NewAudience(fromProtoAudience(p.GetGiftsRead())); err != nil {
			return user.Privacy{}, err
		}
	}

	// A client that does not know the group chats leaves the default.
	chatsInvite := user.DefaultChatsInvite
	if p.GetChatsInvite() != userv1.MessagesWrite_MESSAGES_WRITE_UNSPECIFIED {
		if chatsInvite, err = user.NewMessagesWrite(fromProtoMessagesWrite(p.GetChatsInvite())); err != nil {
			return user.Privacy{}, err
		}
	}

	messagePrice, err := user.NewMessagePrice(p.GetMessagePrice())
	if err != nil {
		return user.Privacy{}, err
	}

	return user.Privacy{
		MessagesPrime:      p.GetMessagesPrime(),
		MessagePrice:       messagePrice,
		ShowBirthday:       p.GetShowBirthday(),
		ShowRelationship:   p.GetShowRelationship(),
		ShowWorldview:      p.GetShowWorldview(),
		ShowPoliticalViews: p.GetShowPoliticalViews(),
		WallWrite:          wallWrite,
		MessagesWrite:      messagesWrite,
		ClubsRead:          clubsRead,
		PageRead:           pageRead,
		InfoRead:           reads[0],
		PhotosRead:         reads[1],
		FriendsRead:        reads[2],
		OnlineRead:         reads[3],
		GiftsRead:          giftsRead,
		ChatsInvite:        chatsInvite,
		Closed:             p.GetClosed(),
	}, nil
}

func toProtoClubsRead(r user.ClubsRead) userv1.ClubsRead {
	switch r {
	case user.ClubsReadEveryone:
		return userv1.ClubsRead_CLUBS_READ_EVERYONE
	case user.ClubsReadUsers:
		return userv1.ClubsRead_CLUBS_READ_USERS
	case user.ClubsReadFollowing:
		return userv1.ClubsRead_CLUBS_READ_FOLLOWING
	case user.ClubsReadOnlyMe:
		return userv1.ClubsRead_CLUBS_READ_ONLY_ME
	default:
		return userv1.ClubsRead_CLUBS_READ_UNSPECIFIED
	}
}

func fromProtoClubsRead(r userv1.ClubsRead) string {
	switch r {
	case userv1.ClubsRead_CLUBS_READ_EVERYONE:
		return "everyone"
	case userv1.ClubsRead_CLUBS_READ_USERS:
		return "users"
	case userv1.ClubsRead_CLUBS_READ_FOLLOWING:
		return "following"
	case userv1.ClubsRead_CLUBS_READ_ONLY_ME:
		return "only_me"
	default:
		return ""
	}
}

func toProtoMessagesWrite(w user.MessagesWrite) userv1.MessagesWrite {
	switch w {
	case user.MessagesWriteEveryone:
		return userv1.MessagesWrite_MESSAGES_WRITE_EVERYONE
	case user.MessagesWriteFollowing:
		return userv1.MessagesWrite_MESSAGES_WRITE_FOLLOWING
	case user.MessagesWriteNobody:
		return userv1.MessagesWrite_MESSAGES_WRITE_NOBODY
	default:
		return userv1.MessagesWrite_MESSAGES_WRITE_UNSPECIFIED
	}
}

func fromProtoMessagesWrite(w userv1.MessagesWrite) string {
	switch w {
	case userv1.MessagesWrite_MESSAGES_WRITE_EVERYONE:
		return "everyone"
	case userv1.MessagesWrite_MESSAGES_WRITE_FOLLOWING:
		return "following"
	case userv1.MessagesWrite_MESSAGES_WRITE_NOBODY:
		return "nobody"
	default:
		return ""
	}
}

func toProtoWallWrite(w user.WallWrite) userv1.WallWrite {
	switch w {
	case user.WallWriteEveryone:
		return userv1.WallWrite_WALL_WRITE_EVERYONE
	case user.WallWriteFollowing:
		return userv1.WallWrite_WALL_WRITE_FOLLOWING
	case user.WallWriteOnlyMe:
		return userv1.WallWrite_WALL_WRITE_ONLY_ME
	default:
		return userv1.WallWrite_WALL_WRITE_UNSPECIFIED
	}
}

func fromProtoWallWrite(w userv1.WallWrite) string {
	switch w {
	case userv1.WallWrite_WALL_WRITE_EVERYONE:
		return "everyone"
	case userv1.WallWrite_WALL_WRITE_FOLLOWING:
		return "following"
	case userv1.WallWrite_WALL_WRITE_ONLY_ME:
		return "only_me"
	default:
		return ""
	}
}

func toProtoPublicProfile(p user.PublicProfile) *userv1.PublicProfile {
	pp := &userv1.PublicProfile{
		Id:                    p.ID.String(),
		Name:                  p.Name.String(),
		Surname:               p.Surname.String(),
		Nickname:              p.Nickname.String(),
		Status:                p.Status.String(),
		Gender:                toProtoGender(p.Gender),
		Relationship:          toProtoRelationship(p.Relationship),
		Worldview:             toProtoWorldview(p.Worldview),
		PoliticalViews:        toProtoPoliticalViews(p.PoliticalViews),
		Birthday:              toProtoBirthday(p.Birthday),
		AvatarId:              avatarString(p.AvatarID),
		Deactivated:           p.Deactivated,
		BirthYearHidden:       p.BirthYearHidden,
		Hometown:              p.Hometown.String(),
		RelationshipPartnerId: avatarString(p.Partner),
		Contacts:              toProtoContacts(p.Contacts),
		Interests:             toProtoInterests(p.Interests),
		ExtraFields:           toProtoExtraFields(p.ExtraFields),
		PageHidden:            p.PageHidden,
		Closed:                p.Closed,
		InfoHidden:            p.InfoHidden,
		Verified:              p.Verified,
		Ban:                   toProtoBan(p.Ban),
		Alert:                 p.Alert.String(),
		Prime:                 p.Prime,
	}
	if p.PrimeStyle != nil {
		pp.PrimeStyle = toProtoPrimeStyle(*p.PrimeStyle)
	}

	// The date of registration is information too. A list tells who is
	// online, but not when the others last visited.
	pp.Online = p.Online
	if !p.LastSeen.IsZero() {
		pp.LastSeenAt = timestamppb.New(p.LastSeen)
	}

	pp.BackdropLeft, pp.BackdropRight = avatarString(p.Backdrop.Left), avatarString(p.Backdrop.Right)
	pp.Completeness = int32(p.Completeness) //nolint:gosec // a percent

	if !p.InfoHidden {
		pp.RegisteredAt = timestamppb.New(user.RegisteredAt(p.ID))
	}

	return pp
}

// profileRequest is implemented by every request that carries a profile.
type profileRequest interface {
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
	GetHideBirthYear() bool
}

// fromProtoProfile validates every field; its errors are meant for the client.
func fromProtoProfile(req profileRequest) (user.Profile, error) {
	name, err := user.NewName(req.GetName())
	if err != nil {
		return user.Profile{}, err
	}

	surname, err := user.NewSurname(req.GetSurname())
	if err != nil {
		return user.Profile{}, err
	}

	nickname, err := user.NewNickname(req.GetNickname())
	if err != nil {
		return user.Profile{}, err
	}

	st, err := user.NewStatus(req.GetStatus())
	if err != nil {
		return user.Profile{}, err
	}

	gender, err := user.NewGender(fromProtoGender(req.GetGender()))
	if err != nil {
		return user.Profile{}, err
	}

	relationship, err := user.NewRelationship(fromProtoRelationship(req.GetRelationship()))
	if err != nil {
		return user.Profile{}, err
	}

	worldview, err := user.NewWorldview(fromProtoWorldview(req.GetWorldview()))
	if err != nil {
		return user.Profile{}, err
	}

	politicalViews, err := user.NewPoliticalViews(fromProtoPoliticalViews(req.GetPoliticalViews()))
	if err != nil {
		return user.Profile{}, err
	}

	birthday, err := user.NewBirthday(fromProtoBirthday(req.GetBirthday()))
	if err != nil {
		return user.Profile{}, err
	}

	hometown, err := user.NewPlace(req.GetHometown())
	if err != nil {
		return user.Profile{}, err
	}

	return user.Profile{
		Name:           name,
		Surname:        surname,
		Nickname:       nickname,
		Status:         st,
		Gender:         gender,
		Relationship:   relationship,
		Worldview:      worldview,
		PoliticalViews: politicalViews,
		Birthday:       birthday,
		HideBirthYear:  req.GetHideBirthYear(),
		Hometown:       hometown,
	}, nil
}

func toProtoGender(g user.Gender) userv1.Gender {
	switch g {
	case user.GenderMale:
		return userv1.Gender_GENDER_MALE
	case user.GenderFemale:
		return userv1.Gender_GENDER_FEMALE
	case user.GenderNeutral:
		return userv1.Gender_GENDER_NEUTRAL
	default:
		return userv1.Gender_GENDER_NONE
	}
}

func fromProtoGender(g userv1.Gender) string {
	switch g {
	case userv1.Gender_GENDER_MALE:
		return "male"
	case userv1.Gender_GENDER_FEMALE:
		return "female"
	case userv1.Gender_GENDER_NEUTRAL:
		return "neutral"
	default:
		return ""
	}
}

func toProtoRelationship(r user.Relationship) userv1.Relationship {
	switch r {
	case user.RelationshipSingle:
		return userv1.Relationship_RELATIONSHIP_SINGLE
	case user.RelationshipDating:
		return userv1.Relationship_RELATIONSHIP_DATING
	case user.RelationshipMarried:
		return userv1.Relationship_RELATIONSHIP_MARRIED
	case user.RelationshipInLove:
		return userv1.Relationship_RELATIONSHIP_IN_LOVE
	case user.RelationshipLooking:
		return userv1.Relationship_RELATIONSHIP_LOOKING
	case user.RelationshipEngaged:
		return userv1.Relationship_RELATIONSHIP_ENGAGED
	case user.RelationshipCivilUnion:
		return userv1.Relationship_RELATIONSHIP_CIVIL_UNION
	case user.RelationshipComplicated:
		return userv1.Relationship_RELATIONSHIP_COMPLICATED
	default:
		return userv1.Relationship_RELATIONSHIP_NONE
	}
}

func fromProtoRelationship(r userv1.Relationship) string {
	switch r {
	case userv1.Relationship_RELATIONSHIP_SINGLE:
		return "single"
	case userv1.Relationship_RELATIONSHIP_DATING:
		return "dating"
	case userv1.Relationship_RELATIONSHIP_MARRIED:
		return "married"
	case userv1.Relationship_RELATIONSHIP_IN_LOVE:
		return "in_love"
	case userv1.Relationship_RELATIONSHIP_LOOKING:
		return "looking"
	case userv1.Relationship_RELATIONSHIP_ENGAGED:
		return "engaged"
	case userv1.Relationship_RELATIONSHIP_CIVIL_UNION:
		return "civil_union"
	case userv1.Relationship_RELATIONSHIP_COMPLICATED:
		return "complicated"
	default:
		return ""
	}
}

func toProtoWorldview(w user.Worldview) userv1.Worldview {
	switch w {
	case user.WorldviewIslam:
		return userv1.Worldview_WORLDVIEW_ISLAM
	case user.WorldviewOrthodox:
		return userv1.Worldview_WORLDVIEW_ORTHODOX
	case user.WorldviewCatholicism:
		return userv1.Worldview_WORLDVIEW_CATHOLICISM
	case user.WorldviewJudaism:
		return userv1.Worldview_WORLDVIEW_JUDAISM
	case user.WorldviewProtestantism:
		return userv1.Worldview_WORLDVIEW_PROTESTANTISM
	case user.WorldviewBuddhism:
		return userv1.Worldview_WORLDVIEW_BUDDHISM
	case user.WorldviewConfucianism:
		return userv1.Worldview_WORLDVIEW_CONFUCIANISM
	case user.WorldviewSecularHumanism:
		return userv1.Worldview_WORLDVIEW_SECULAR_HUMANISM
	default:
		return userv1.Worldview_WORLDVIEW_NONE
	}
}

func fromProtoWorldview(w userv1.Worldview) string {
	switch w {
	case userv1.Worldview_WORLDVIEW_ISLAM:
		return "islam"
	case userv1.Worldview_WORLDVIEW_ORTHODOX:
		return "orthodox"
	case userv1.Worldview_WORLDVIEW_CATHOLICISM:
		return "catholicism"
	case userv1.Worldview_WORLDVIEW_JUDAISM:
		return "judaism"
	case userv1.Worldview_WORLDVIEW_PROTESTANTISM:
		return "protestantism"
	case userv1.Worldview_WORLDVIEW_BUDDHISM:
		return "buddhism"
	case userv1.Worldview_WORLDVIEW_CONFUCIANISM:
		return "confucianism"
	case userv1.Worldview_WORLDVIEW_SECULAR_HUMANISM:
		return "secular_humanism"
	default:
		return ""
	}
}

func toProtoPoliticalViews(p user.PoliticalViews) userv1.PoliticalViews {
	switch p {
	case user.PoliticalViewsCommunist:
		return userv1.PoliticalViews_POLITICAL_VIEWS_COMMUNIST
	case user.PoliticalViewsSocialist:
		return userv1.PoliticalViews_POLITICAL_VIEWS_SOCIALIST
	case user.PoliticalViewsLiberal:
		return userv1.PoliticalViews_POLITICAL_VIEWS_LIBERAL
	case user.PoliticalViewsConservative:
		return userv1.PoliticalViews_POLITICAL_VIEWS_CONSERVATIVE
	case user.PoliticalViewsMonarchist:
		return userv1.PoliticalViews_POLITICAL_VIEWS_MONARCHIST
	case user.PoliticalViewsUltraconservative:
		return userv1.PoliticalViews_POLITICAL_VIEWS_ULTRACONSERVATIVE
	case user.PoliticalViewsIndifferent:
		return userv1.PoliticalViews_POLITICAL_VIEWS_INDIFFERENT
	case user.PoliticalViewsLibertarian:
		return userv1.PoliticalViews_POLITICAL_VIEWS_LIBERTARIAN
	case user.PoliticalViewsModerateIslamism:
		return userv1.PoliticalViews_POLITICAL_VIEWS_MODERATE_ISLAMISM
	case user.PoliticalViewsRadicalIslamism:
		return userv1.PoliticalViews_POLITICAL_VIEWS_RADICAL_ISLAMISM
	default:
		return userv1.PoliticalViews_POLITICAL_VIEWS_NONE
	}
}

func fromProtoPoliticalViews(p userv1.PoliticalViews) string {
	switch p {
	case userv1.PoliticalViews_POLITICAL_VIEWS_COMMUNIST:
		return "communist"
	case userv1.PoliticalViews_POLITICAL_VIEWS_SOCIALIST:
		return "socialist"
	case userv1.PoliticalViews_POLITICAL_VIEWS_LIBERAL:
		return "liberal"
	case userv1.PoliticalViews_POLITICAL_VIEWS_CONSERVATIVE:
		return "conservative"
	case userv1.PoliticalViews_POLITICAL_VIEWS_MONARCHIST:
		return "monarchist"
	case userv1.PoliticalViews_POLITICAL_VIEWS_ULTRACONSERVATIVE:
		return "ultraconservative"
	case userv1.PoliticalViews_POLITICAL_VIEWS_INDIFFERENT:
		return "indifferent"
	case userv1.PoliticalViews_POLITICAL_VIEWS_LIBERTARIAN:
		return "libertarian"
	case userv1.PoliticalViews_POLITICAL_VIEWS_MODERATE_ISLAMISM:
		return "moderate_islamism"
	case userv1.PoliticalViews_POLITICAL_VIEWS_RADICAL_ISLAMISM:
		return "radical_islamism"
	default:
		return ""
	}
}

func toProtoBirthday(b user.Birthday) *timestamppb.Timestamp {
	if b.Time().IsZero() {
		return nil
	}

	return timestamppb.New(b.Time())
}

func fromProtoBirthday(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.AsTime()
}

func toProtoInterface(ui user.Interface) *userv1.InterfaceSettings {
	hidden := make([]string, 0, len(ui.HiddenMenu))
	for _, item := range ui.HiddenMenu {
		hidden = append(hidden, item.String())
	}

	out := &userv1.InterfaceSettings{
		AvatarStyle: ui.AvatarStyle.String(),
		MainPage:    ui.MainPage.String(),
		HiddenMenu:  hidden,
		Theme:       ui.Theme.String(),
		HideRating:  ui.HideRating,
		Nsfw:        ui.NSFW.String(),
		PostView:    ui.PostView.String(),
	}
	if minutes, fixed := ui.Timezone.Offset(); fixed {
		offset := int32(minutes)
		out.TimezoneOffset = &offset
	}

	return out
}

// fromProtoInterface is the settings form. A client older than a setting
// says nothing of it and keeps the default.
func fromProtoInterface(in *userv1.InterfaceSettings) user.InterfaceForm {
	defaults := user.DefaultInterface()

	form := user.InterfaceForm{
		AvatarStyle: in.GetAvatarStyle(),
		Theme:       cmp.Or(in.GetTheme(), defaults.Theme.String()),
		HideRating:  in.GetHideRating(),
		NSFW:        cmp.Or(in.GetNsfw(), defaults.NSFW.String()),
		PostView:    cmp.Or(in.GetPostView(), defaults.PostView.String()),
		MainPage:    in.GetMainPage(),
		HiddenMenu:  in.GetHiddenMenu(),
	}
	if in != nil && in.TimezoneOffset != nil {
		offset := int(in.GetTimezoneOffset())
		form.Timezone = &offset
	}

	return form
}

func toProtoAd(ad *user.Ad) *userv1.Ad {
	owner, photo := toProtoAdPhoto(ad.Photo())

	return &userv1.Ad{
		Id:           ad.ID().String(),
		Url:          ad.URL().String(),
		Title:        ad.Title().String(),
		Text:         ad.Text().String(),
		PhotoOwnerId: owner,
		PhotoId:      photo,
		Active:       ad.IsActive(),
	}
}

func toProtoAdStats(s user.AdStats) *userv1.AdStats {
	return &userv1.AdStats{Views: s.Views, Viewers: s.Viewers, Clicks: s.Clicks, Clickers: s.Clickers}
}

func toProtoAdPhoto(p user.AdPhoto) (owner, photo string) {
	if p.IsZero() {
		return "", ""
	}

	return p.OwnerID.String(), p.PhotoID.String()
}

// fromProtoAdPhoto is a cover the caller uploaded; the service names its
// owner. Empty is no cover.
func fromProtoAdPhoto(photoID string) (user.AdPhoto, error) {
	if photoID == "" {
		return user.AdPhoto{}, nil
	}

	id, err := parseID(photoID, "photo id")
	if err != nil {
		return user.AdPhoto{}, err
	}

	return user.AdPhoto{PhotoID: id}, nil
}

func fromProtoAdChange(req *userv1.EditAdRequest) (user.AdChange, error) {
	var c user.AdChange

	if req.Url != nil {
		u, err := user.NewAdURL(req.GetUrl())
		if err != nil {
			return c, status.Error(codes.InvalidArgument, err.Error())
		}

		c.URL = &u
	}

	if req.Title != nil {
		t, err := user.NewAdTitle(req.GetTitle())
		if err != nil {
			return c, status.Error(codes.InvalidArgument, err.Error())
		}

		c.Title = &t
	}

	if req.Text != nil {
		t, err := user.NewAdText(req.GetText())
		if err != nil {
			return c, status.Error(codes.InvalidArgument, err.Error())
		}

		c.Text = &t
	}

	if req.PhotoId != nil {
		p, err := fromProtoAdPhoto(req.GetPhotoId())
		if err != nil {
			return c, err
		}

		c.Photo = &p
	}

	if req.Active != nil {
		active := req.GetActive()
		c.Active = &active
	}

	return c, nil
}

func toProtoDecor(d user.Decor) userv1.Decor {
	switch d {
	case user.DecorOn:
		return userv1.Decor_DECOR_ON
	case user.DecorOff:
		return userv1.Decor_DECOR_OFF
	default:
		return userv1.Decor_DECOR_AUTO
	}
}

func fromProtoDecor(d userv1.Decor) (user.Decor, error) {
	switch d {
	case userv1.Decor_DECOR_AUTO:
		return user.DecorAuto, nil
	case userv1.Decor_DECOR_ON:
		return user.DecorOn, nil
	case userv1.Decor_DECOR_OFF:
		return user.DecorOff, nil
	default:
		return "", user.ErrDecorInvalid
	}
}

func toProtoCasinoPrizes(prizes []user.CasinoPrize) []*userv1.CasinoPrize {
	out := make([]*userv1.CasinoPrize, 0, len(prizes))
	for _, p := range prizes {
		out = append(out, &userv1.CasinoPrize{
			Votes:  p.Votes,
			Weight: int32(p.Weight), //nolint:gosec // at most user.MaxCasinoWeight
		})
	}

	return out
}

func fromProtoCasinoPrizes(prizes []*userv1.CasinoPrize) []user.CasinoPrize {
	out := make([]user.CasinoPrize, 0, len(prizes))
	for _, p := range prizes {
		out = append(out, user.CasinoPrize{Votes: p.GetVotes(), Weight: int(p.GetWeight())})
	}

	return out
}

func toProtoPrimePrices(p user.PrimePrices) *userv1.PrimePrices {
	return &userv1.PrimePrices{Month: p.Month, Quarter: p.Quarter, Year: p.Year}
}

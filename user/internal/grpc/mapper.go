package grpc

import (
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoUser(u *user.User) *userv1.User {
	p := u.Profile()

	return &userv1.User{
		Id:               u.ID().String(),
		Name:             p.Name.String(),
		Surname:          p.Surname.String(),
		Nickname:         p.Nickname.String(),
		Status:           p.Status.String(),
		Gender:           toProtoGender(p.Gender),
		Relationship:     toProtoRelationship(p.Relationship),
		Worldview:        toProtoWorldview(p.Worldview),
		PoliticalViews:   toProtoPoliticalViews(p.PoliticalViews),
		Birthday:         toProtoBirthday(p.Birthday),
		Email:            u.Email().String(),
		ProfileCompleted: u.IsProfileCompleted(),
		Privacy:          toProtoPrivacy(u.Privacy()),
		AvatarId:         avatarString(u.AvatarID()),
	}
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
	}
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

	return user.Privacy{
		ShowBirthday:       p.GetShowBirthday(),
		ShowRelationship:   p.GetShowRelationship(),
		ShowWorldview:      p.GetShowWorldview(),
		ShowPoliticalViews: p.GetShowPoliticalViews(),
		WallWrite:          wallWrite,
		MessagesWrite:      messagesWrite,
	}, nil
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
	return &userv1.PublicProfile{
		Id:             p.ID.String(),
		Name:           p.Name.String(),
		Surname:        p.Surname.String(),
		Nickname:       p.Nickname.String(),
		Status:         p.Status.String(),
		Gender:         toProtoGender(p.Gender),
		Relationship:   toProtoRelationship(p.Relationship),
		Worldview:      toProtoWorldview(p.Worldview),
		PoliticalViews: toProtoPoliticalViews(p.PoliticalViews),
		Birthday:       toProtoBirthday(p.Birthday),
		AvatarId:       avatarString(p.AvatarID),
	}
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
	}, nil
}

func toProtoGender(g user.Gender) userv1.Gender {
	switch g {
	case user.GenderMale:
		return userv1.Gender_GENDER_MALE
	case user.GenderFemale:
		return userv1.Gender_GENDER_FEMALE
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
	case user.PoliticalViewsModerate:
		return userv1.PoliticalViews_POLITICAL_VIEWS_MODERATE
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
	case userv1.PoliticalViews_POLITICAL_VIEWS_MODERATE:
		return "moderate"
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

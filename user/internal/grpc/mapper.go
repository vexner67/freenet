package grpc

import (
	"time"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoUser(u *user.User) *userv1.User {
	return &userv1.User{
		Id:             u.ID().String(),
		Name:           u.Name().String(),
		Surname:        u.Surname().String(),
		Nickname:       u.Nickname().String(),
		Status:         u.Status().String(),
		Gender:         toProtoGender(u.Gender()),
		Relationship:   toProtoRelationship(u.Relationship()),
		Worldview:      toProtoWorldview(u.Worldview()),
		PoliticalViews: toProtoPoliticalViews(u.PoliticalViews()),
		Birthday:       timestamppb.New(u.Birthday().Time()),
		Email:          u.Email().String(),
	}
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

func fromProtoBirthday(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.AsTime()
}

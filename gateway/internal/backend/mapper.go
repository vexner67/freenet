package backend

import (
	"fmt"
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
	kindPrefix           = "KIND_"

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

func toUpdateProfileRequest(p model.Profile) (*userv1.UpdateProfileRequest, error) {
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
	}
}

func toUser(u *userv1.User) model.User {
	return model.User{
		ID:               u.GetId(),
		AvatarID:         u.GetAvatarId(),
		Email:            u.GetEmail(),
		Profile:          toProfile(u),
		ProfileCompleted: u.GetProfileCompleted(),
		Privacy: model.Privacy{
			ShowBirthday:       u.GetPrivacy().GetShowBirthday(),
			ShowRelationship:   u.GetPrivacy().GetShowRelationship(),
			ShowWorldview:      u.GetPrivacy().GetShowWorldview(),
			ShowPoliticalViews: u.GetPrivacy().GetShowPoliticalViews(),
			WallWrite:          enumName(u.GetPrivacy().GetWallWrite().String(), wallWritePrefix),
			MessagesWrite:      enumName(u.GetPrivacy().GetMessagesWrite().String(), messagesWritePrefix),
		},
	}
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

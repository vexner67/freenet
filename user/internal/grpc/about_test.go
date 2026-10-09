package grpc_test

import (
	"context"
	"testing"
	"time"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAboutTheUser(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	_, bobID := s.member(t, "bob")

	updated, err := s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{
		Name: "Анна", Nickname: "ann", Gender: userv1.Gender_GENDER_NEUTRAL, Hometown: " Тверь ",
		Relationship: userv1.Relationship_RELATIONSHIP_ENGAGED, RelationshipPartner: "Bob",
		Birthday: timestamppb.New(time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)), HideBirthYear: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if u := updated.GetUser(); u.GetHometown() != "Тверь" || u.GetRelationshipPartnerId() != bobID || !u.GetHideBirthYear() || u.GetGender() != userv1.Gender_GENDER_NEUTRAL {
		t.Fatalf("UpdateProfile = %v", u)
	}

	for name, partner := range map[string]string{"nobody": "nobody", "a bad nickname": "1 bad", "oneself": "ann"} {
		_, err = s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{
			Name: "Анна", Nickname: "ann", Relationship: userv1.Relationship_RELATIONSHIP_DATING, RelationshipPartner: partner,
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("partner %s: %v", name, err)
		}
	}

	if _, err = s.client.UpdateContacts(ann, &userv1.UpdateContactsRequest{Contacts: &userv1.Contacts{Telegram: "@a"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad telegram: %v", err)
	}
	contacts, err := s.client.UpdateContacts(ann, &userv1.UpdateContactsRequest{Contacts: &userv1.Contacts{
		Telegram: "t.me/ann_tg", Website: "ann.example", City: "Москва",
	}})
	if err != nil || contacts.GetUser().GetContacts().GetWebsite() != "https://ann.example" || contacts.GetUser().GetContacts().GetTelegram() != "ann_tg" {
		t.Fatalf("UpdateContacts = %v, %v", contacts, err)
	}

	if _, err = s.client.UpdateInterests(ann, &userv1.UpdateInterestsRequest{Interests: &userv1.Interests{Films: "Брат, Брат 2"}}); err != nil {
		t.Fatal(err)
	}

	fields := []*userv1.ExtraField{{Name: "Цвет", Text: "синий", Place: userv1.FieldPlace_FIELD_PLACE_INTERESTS}}
	if _, err = s.client.UpdateExtraFields(ann, &userv1.UpdateExtraFieldsRequest{Fields: fields}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.UpdateExtraFields(ann, &userv1.UpdateExtraFieldsRequest{Fields: []*userv1.ExtraField{{Name: "Цвет", Text: "синий"}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a field without a place: %v", err)
	}
	if _, err = s.client.UpdateExtraFields(ann, &userv1.UpdateExtraFieldsRequest{Fields: make([]*userv1.ExtraField, 8)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("8 fields: %v", err)
	}

	// Anyone sees them; the birthday and the relationship stay hidden until
	// opened.
	got, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil {
		t.Fatal(err)
	}
	p := got.GetProfile()
	if p.GetHometown() != "Тверь" || p.GetContacts().GetCity() != "Москва" || p.GetInterests().GetFilms() != "Брат, Брат 2" ||
		len(p.GetExtraFields()) != 1 || p.GetRelationshipPartnerId() != "" || p.GetBirthday() != nil || p.GetRegisteredAt() == nil {
		t.Fatalf("GetProfile = %v", p)
	}

	found, err := s.client.SearchUsers(context.Background(), &userv1.SearchUsersRequest{City: "москва", Film: "брат 2"})
	if err != nil || len(found.GetUsers()) != 1 || found.GetUsers()[0].GetId() != annID {
		t.Fatalf("SearchUsers by filters = %v, %v", found, err)
	}
	if _, err = s.client.SearchUsers(context.Background(), &userv1.SearchUsersRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an empty search: %v", err)
	}

	// Shown to users only, with the information to ann alone.
	if _, err = s.client.UpdatePrivacy(ann, &userv1.UpdatePrivacyRequest{Privacy: &userv1.PrivacySettings{
		WallWrite: userv1.WallWrite_WALL_WRITE_EVERYONE, MessagesWrite: userv1.MessagesWrite_MESSAGES_WRITE_EVERYONE, ClubsRead: userv1.ClubsRead_CLUBS_READ_USERS,
		PageRead: userv1.Audience_AUDIENCE_USERS, InfoRead: userv1.Audience_AUDIENCE_ONLY_ME, PhotosRead: userv1.Audience_AUDIENCE_EVERYONE, FriendsRead: userv1.Audience_AUDIENCE_EVERYONE,
	}}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || !got.GetProfile().GetPageHidden() {
		t.Fatalf("anonymously = %v, %v", got, err)
	}
	bob := s.signIn(t, "bob@example.com")
	got, err = s.client.GetProfile(bob, &userv1.GetProfileRequest{Nickname: "ann"})
	if p := got.GetProfile(); err != nil || p.GetPageHidden() || !p.GetInfoHidden() || p.GetHometown() != "" || p.GetRegisteredAt() != nil {
		t.Fatalf("by bob = %v, %v", p, err)
	}
	// Filters look into the information, which ann now hides.
	if found, err = s.client.SearchUsers(context.Background(), &userv1.SearchUsersRequest{City: "москва"}); err != nil || len(found.GetUsers()) != 0 {
		t.Fatalf("SearchUsers by a hidden city = %v, %v", found, err)
	}
}

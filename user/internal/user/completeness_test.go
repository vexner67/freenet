package user_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

// fullPage fills in every field the site asks for, with a photo.
func fullPage(t *testing.T) *user.User {
	t.Helper()

	u, _ := user.New("ann@example.com")
	birthday, _ := user.NewBirthday(time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC))
	if err := u.UpdateProfile(user.Profile{
		Name: "Ann", Surname: "Lee", Nickname: "ann", Status: "Привет", Gender: user.GenderFemale,
		Relationship: user.RelationshipSingle, Worldview: user.WorldviewOrthodox, PoliticalViews: user.PoliticalViewsCommunist,
		Birthday: birthday, Hometown: "Тверь",
	}); err != nil {
		t.Fatal(err)
	}
	u.UpdateContacts(user.Contacts{Email: "ann@mail.example", Telegram: "ann_lee", Website: "https://ann.example", City: "Москва", Address: "Арбат, 1"})
	u.UpdateInterests(user.Interests{
		Interests: "кошки", Music: "джаз", Films: "Брат", Shows: "Друзья", Books: "Мастер и Маргарита",
		Quotes: "…", Games: "шахматы", About: "я",
	})
	u.SetAvatar(uuid.New())

	return u
}

func TestUser_Completeness(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	_ = u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann"})

	// The name and the address are required, so they add nothing.
	all := []user.Unfilled{{"photo", 20}, {"status", 5}, {"main", 25}, {"contacts", 20}, {"interests", 30}}
	if p, unfilled := u.Completeness(); p != 0 || !slices.Equal(unfilled, all) {
		t.Fatalf("empty page: %d %v", p, unfilled)
	}

	// Each field adds its weight by itself.
	u.UpdateStatus("Привет")
	u.UpdateContacts(user.Contacts{City: "Москва", Telegram: "ann_lee"})
	u.SetAvatar(uuid.New())
	want := []user.Unfilled{{"main", 25}, {"contacts", 12}, {"interests", 30}}
	if p, unfilled := u.Completeness(); p != 33 || !slices.Equal(unfilled, want) || u.Fill() != 33 {
		t.Fatalf("partly filled: %d %v", p, unfilled)
	}

	if u.Profile().Status != "Привет" || u.PublicProfile().Completeness != 0 {
		t.Fatal("UpdateStatus keeps the status; only a viewer's page tells the completeness")
	}

	// Every field with the photo is exactly 100.
	full := fullPage(t)
	if p, unfilled := full.Completeness(); p != 100 || len(unfilled) != 0 {
		t.Fatalf("full: %d %v", p, unfilled)
	}

	// The extra fields add nothing.
	field, err := user.NewExtraField("Кот", "Барсик", user.FieldPlaceInterests)
	if err != nil {
		t.Fatal(err)
	}
	if err = full.UpdateExtraFields([]user.ExtraField{field}); err != nil {
		t.Fatal(err)
	}
	if p, _ := full.Completeness(); p != 100 {
		t.Fatalf("with an extra field: %d", p)
	}
}

func TestUser_CompletenessWithRating(t *testing.T) {
	t.Parallel()

	full := fullPage(t)

	tests := []struct {
		rating, want int
	}{
		{0, 100},
		{1, 101},
		{900, 1000},
		{user.MaxRating - 100, user.MaxRating},
		// What vouchers gave past the limit still shows the limit.
		{user.MaxRating, user.MaxRating},
	}
	for _, tt := range tests {
		if p, _ := full.WithWallet(user.Wallet{Rating: tt.rating}).Completeness(); p != tt.want {
			t.Errorf("rating %d: %d, want %d", tt.rating, p, tt.want)
		}
	}

	if most := full.RatingMost(); most != user.MaxRating-100 {
		t.Fatalf("RatingMost of a full page = %d", most)
	}
}

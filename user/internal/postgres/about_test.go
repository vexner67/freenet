package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUserRepository_About(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func(prefix string) *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique(prefix) + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}

	ann, bob := newUser("ab"), newUser("bb")
	city := user.Place(unique("Город"))
	film := unique("фильм")

	profile := user.Profile{
		Name: "Ann", Nickname: user.Nickname(unique("ab")), Hometown: "Тверь",
		Relationship: user.RelationshipDating, Partner: bob.ID(), HideBirthYear: true,
	}
	if err := ann.UpdateProfile(profile); err != nil {
		t.Fatal(err)
	}
	contacts := user.Contacts{Email: "ann@mail.example", Telegram: "ann_tg", Website: "https://ann.example", City: city, Address: "ул. Ленина, 1"}
	interests := user.Interests{
		Interests: "кошки", Music: "рок", Films: user.About("Брат, " + film), Shows: "Друзья",
		Books: "Мастер и Маргарита", Quotes: "Без труда\nне выловишь", Games: "Тетрис", About: "Обо мне",
	}
	field, _ := user.NewExtraField("Цвет", "синий", user.FieldPlaceInterests)
	ann.UpdateContacts(contacts)
	ann.UpdateInterests(interests)
	if err := ann.UpdateExtraFields([]user.ExtraField{field}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Update(ctx, ann); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, ann.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile() != profile || got.Contacts() != contacts || got.Interests() != interests || !reflect.DeepEqual(got.ExtraFields(), []user.ExtraField{field}) {
		t.Fatalf("round trip:\n%+v\n%+v\n%+v\n%+v", got.Profile(), got.Contacts(), got.Interests(), got.ExtraFields())
	}

	// A partner erased meanwhile.
	gone := profile
	gone.Partner = uuid.New()
	if err = ann.UpdateProfile(gone); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, ann); !errors.Is(err, user.ErrPartnerNotFound) {
		t.Fatalf("Update with no partner: %v", err)
	}

	for _, f := range []user.SearchFilters{{City: city.String()}, {Film: film, Hometown: "твЕрь"}} {
		q, err := user.NewSearchQuery("", f)
		if err != nil {
			t.Fatal(err)
		}
		found, err := repo.Search(ctx, q, uuid.Nil, 10)
		if err != nil || len(found) != 1 || found[0].ID() != ann.ID() {
			t.Fatalf("Search(%+v) = %v, %v", f, found, err)
		}
	}
}

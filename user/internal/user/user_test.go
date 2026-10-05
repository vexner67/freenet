package user_test

import (
	"errors"
	"testing"
	"time"

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

	if got != want {
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

	u.UpdatePrivacy(user.Privacy{ShowWorldview: true, ShowBirthday: true})
	got := u.PublicProfile()

	if got.Worldview != user.WorldviewBuddhism || got.Birthday != birthday {
		t.Fatalf("opened fields must be visible: %+v", got)
	}
	if got.Relationship != user.RelationshipNone || got.PoliticalViews != user.PoliticalViewsNone {
		t.Fatalf("closed fields must stay hidden: %+v", got)
	}
}

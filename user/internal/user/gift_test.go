package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestGiftCatalog(t *testing.T) {
	t.Parallel()

	seen := map[user.GiftKind]bool{}

	for _, c := range user.GiftCatalog() {
		if c.ID == "" || len(c.Gifts) == 0 {
			t.Fatalf("collection %q is empty", c.ID)
		}

		for _, kind := range c.Gifts {
			if seen[kind] {
				t.Fatalf("gift %q is in the catalogue twice", kind)
			}
			seen[kind] = true

			if got, err := user.NewGiftKind(kind.String()); err != nil || got != kind {
				t.Fatalf("NewGiftKind(%q) = %q, %v", kind, got, err)
			}
		}
	}

	// The catalogue handed out is a copy.
	user.GiftCatalog()[0].Gifts[0] = "broken"
	if _, err := user.NewGiftKind("broken"); !errors.Is(err, user.ErrUnknownGift) {
		t.Fatalf("a gift out of the catalogue: %v", err)
	}

	if _, err := user.NewGiftKind(""); !errors.Is(err, user.ErrUnknownGift) {
		t.Fatalf("no gift: %v", err)
	}
}

func TestNewGiftCaption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{"empty", "", "", nil},
		{"trimmed", "  С днём рождения! \n", "С днём рождения!", nil},
		{"at the limit, in runes", strings.Repeat("я", user.MaxGiftCaptionLength), strings.Repeat("я", user.MaxGiftCaptionLength), nil},
		{"too long", strings.Repeat("я", user.MaxGiftCaptionLength+1), "", user.ErrGiftCaptionTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewGiftCaption(tt.in)
			if !errors.Is(err, tt.err) || got.String() != tt.want {
				t.Fatalf("NewGiftCaption = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestUser_GiftFrom(t *testing.T) {
	t.Parallel()

	sender := uuid.New()

	page := func(t *testing.T, change func(*user.Privacy)) *user.User {
		t.Helper()

		u, err := user.New("ann@example.com")
		if err != nil {
			t.Fatal(err)
		}

		privacy := u.Privacy()
		change(&privacy)
		if err = u.UpdatePrivacy(privacy); err != nil {
			t.Fatal(err)
		}

		return u
	}

	tests := []struct {
		name   string
		change func(*user.Privacy)
		ties   user.Ties
		want   error
	}{
		{"a stranger", func(*user.Privacy) {}, user.Ties{}, nil},
		{"hidden gifts are still taken", func(p *user.Privacy) { p.GiftsRead = user.AudienceOnlyMe }, user.Ties{}, nil},
		{"blocked", func(*user.Privacy) {}, user.Ties{Blocked: true, Followed: true}, user.ErrGiftRefused},
		{"a closed page, a stranger", func(p *user.Privacy) { p.Closed = true }, user.Ties{}, user.ErrGiftRefused},
		{"a closed page, followed", func(p *user.Privacy) { p.Closed = true }, user.Ties{Followed: true}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := page(t, tt.change)

			g, err := u.GiftFrom(sender, tt.ties, "rose", "Привет", false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("GiftFrom: %v, want %v", err, tt.want)
			}
			if err != nil {
				return
			}

			if g.RecipientID() != u.ID() || g.SenderID() != sender || g.Kind() != "rose" || g.Caption() != "Привет" || g.Anonymous() {
				t.Fatalf("gift = %+v", g)
			}
			if time.Since(g.SentAt()) > time.Minute || !g.Notifies() {
				t.Fatalf("sent at %v, notifies %v", g.SentAt(), g.Notifies())
			}
			if !g.CanBeDeletedBy(u.ID()) || g.CanBeDeletedBy(sender) {
				t.Fatal("a gift is its recipient's to delete")
			}
		})
	}

	t.Run("to oneself, even on a closed page", func(t *testing.T) {
		t.Parallel()

		u := page(t, func(p *user.Privacy) { p.Closed = true })

		g, err := u.GiftFrom(u.ID(), user.Ties{}, "cake", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if g.Notifies() {
			t.Fatal("one's own gift notifies nobody")
		}
	})

	t.Run("a deleted page", func(t *testing.T) {
		t.Parallel()

		u := page(t, func(*user.Privacy) {})
		if err := u.Deactivate("", time.Now(), time.Hour); err != nil {
			t.Fatal(err)
		}

		if _, err := u.GiftFrom(sender, user.Ties{Followed: true}, "rose", "", false); !errors.Is(err, user.ErrDeactivated) {
			t.Fatalf("GiftFrom: %v", err)
		}
	})
}

func TestGift_SignedFor(t *testing.T) {
	t.Parallel()

	recipient, sender, stranger := uuid.New(), uuid.New(), uuid.New()

	open := user.RestoreGift(uuid.New(), recipient, sender, "rose", "", false)
	for _, viewer := range []uuid.UUID{recipient, sender, stranger, uuid.Nil} {
		if !open.SignedFor(viewer) {
			t.Fatalf("a signed gift hides its sender from %v", viewer)
		}
	}

	anonymous := user.RestoreGift(uuid.New(), recipient, sender, "rose", "", true)
	if !anonymous.SignedFor(recipient) || !anonymous.SignedFor(sender) {
		t.Fatal("an anonymous gift is signed for the two")
	}
	if anonymous.SignedFor(stranger) || anonymous.SignedFor(uuid.Nil) {
		t.Fatal("an anonymous gift is signed for others")
	}

	// The sender's page is erased: nobody is the sender.
	orphan := user.RestoreGift(uuid.New(), recipient, uuid.Nil, "rose", "", true)
	if orphan.SignedFor(uuid.Nil) || orphan.Notifies() {
		t.Fatal("an anonymous gift of nobody")
	}
}

func TestUser_AccessFor_Gifts(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if !u.AccessFor(uuid.Nil, user.Ties{}, time.Now()).Gifts {
		t.Fatal("gifts are shown to everyone by default")
	}

	privacy := u.Privacy()
	privacy.GiftsRead = user.AudienceFollowing
	if err = u.UpdatePrivacy(privacy); err != nil {
		t.Fatal(err)
	}

	viewer := uuid.New()
	if u.AccessFor(viewer, user.Ties{}, time.Now()).Gifts || !u.AccessFor(viewer, user.Ties{Followed: true}, time.Now()).Gifts || !u.AccessFor(u.ID(), user.Ties{}, time.Now()).Gifts {
		t.Fatal("gifts for the followed and the owner only")
	}

	// A client that does not know the setting leaves it to everyone.
	privacy.GiftsRead = ""
	if err = u.UpdatePrivacy(privacy); err != nil || u.Privacy().GiftsRead != user.AudienceEveryone {
		t.Fatalf("an empty setting: %v, %q", err, u.Privacy().GiftsRead)
	}

	privacy.GiftsRead = "friends"
	if err = u.UpdatePrivacy(privacy); !errors.Is(err, user.ErrInvalidAudience) {
		t.Fatalf("an unknown setting: %v", err)
	}
}

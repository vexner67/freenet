package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  post.Text
		err   error
	}{
		{"plain", "привет", "привет", nil},
		{"trimmed, windows line breaks", "  раз\r\nдва \n", "раз\nдва", nil},
		{"emoji count as one character", strings.Repeat("👋", 1000), post.Text(strings.Repeat("👋", 1000)), nil},
		{"empty is fine, photos may say it all", "", "", nil},
		{"only spaces and breaks", " \n\t\r\n ", "", nil},
		{"too long", strings.Repeat("я", 1001), "", post.ErrTextTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewText(tt.input)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewText = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestPost(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	now := time.Date(2026, 10, 4, 18, 30, 0, 123456789, time.FixedZone("MSK", 3*3600))

	p, err := post.New(author, author, textOnly(t, "привет"), now)
	if err != nil {
		t.Fatal(err)
	}

	if p.CreatedAt().Location() != time.UTC || p.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", p.CreatedAt())
	}
	if !p.OnOwnWall() || p.WallID() != author {
		t.Fatalf("wall = %v, want the author's own", p.WallID())
	}
	if !p.CanBeDeletedBy(author) || p.CanBeDeletedBy(uuid.New()) {
		t.Fatal("only the author may delete a post on their own wall")
	}

	later, err := post.New(author, author, textOnly(t, "ещё"), now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= p.ID().String() {
		t.Fatal("IDs must grow, walls are ordered by them")
	}
}

func TestPost_OnSomeoneElsesWall(t *testing.T) {
	t.Parallel()

	owner, author, stranger := uuid.New(), uuid.New(), uuid.New()

	p, err := post.New(owner, author, textOnly(t, "привет"), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if p.OnOwnWall() || p.WallID() != owner || p.AuthorID() != author {
		t.Fatalf("wall = %v, author = %v", p.WallID(), p.AuthorID())
	}
	if !p.CanBeDeletedBy(author) || !p.CanBeDeletedBy(owner) {
		t.Fatal("the author and the owner of the wall may delete")
	}
	if p.CanBeDeletedBy(stranger) {
		t.Fatal("a stranger may not delete")
	}
}

func TestPost_FromWall(t *testing.T) {
	t.Parallel()

	club, admin := uuid.New(), uuid.New()

	p, err := post.NewFromWall(club, admin, textOnly(t, "новости группы"), false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !p.FromWall() || p.AuthorID() != admin || p.WallID() != club || p.Notified() != uuid.Nil {
		t.Fatalf("post = %+v; it speaks for the club and notifies nobody", p.Stored())
	}

	// The author still edits it, and it keeps speaking for the club.
	edited, err := p.Edited(admin, "новости", nil, time.Now())
	if err != nil || !edited.FromWall() {
		t.Fatalf("Edited = %+v, %v", edited, err)
	}
	if !post.Restore(p.Stored()).FromWall() {
		t.Fatal("storage loses fromWall")
	}

	if _, err = post.NewFromWall(admin, admin, textOnly(t, "я"), false, time.Now()); !errors.Is(err, post.ErrFromOwnWall) {
		t.Fatalf("on one's own wall: %v", err)
	}

	signed, err := post.NewFromWall(club, admin, textOnly(t, "с подписью"), true, time.Now())
	if err != nil || !signed.Signed() || !signed.FromWall() || signed.Notified() != uuid.Nil {
		t.Fatalf("signed = %+v, %v; still the club's, notifying nobody", signed, err)
	}
	if edited, err := signed.Edited(admin, "правка", nil, time.Now()); err != nil || !edited.Signed() || !post.Restore(signed.Stored()).Signed() {
		t.Fatalf("the signature does not stay: %v", err)
	}

	own, err := post.New(club, admin, textOnly(t, "от себя"), time.Now())
	if err != nil || own.FromWall() || own.Notified() != admin {
		t.Fatalf("a post of one's own = %+v, %v; its author hears of it", own, err)
	}
}

func TestNewAvatarUpdate(t *testing.T) {
	t.Parallel()

	ann, club, photo := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	content, err := post.NewContent("", post.Photos{photo})
	if err != nil {
		t.Fatal(err)
	}

	own, err := post.NewAvatarUpdate(ann, ann, content, now)
	if err != nil || !own.AvatarUpdate() || own.FromWall() || own.Notified() != ann {
		t.Fatalf("on ann's wall = %+v, %v; hers", own, err)
	}
	if _, err = own.Edited(ann, "подпись", nil, now); !errors.Is(err, post.ErrCannotEdit) {
		t.Fatalf("edited: %v", err)
	}
	if !own.EditableUntil().Equal(own.CreatedAt()) || !post.Restore(own.Stored()).AvatarUpdate() {
		t.Fatalf("editable until %v; storage keeps it", own.EditableUntil())
	}

	clubs, err := post.NewAvatarUpdate(club, ann, content, now)
	if err != nil || !clubs.FromWall() || clubs.Notified() != uuid.Nil {
		t.Fatalf("for the club = %+v, %v; it speaks for the club", clubs, err)
	}

	withText, _ := post.NewContent("новое фото", post.Photos{photo})
	twoPhotos, _ := post.NewContent("", post.Photos{photo, uuid.New()})
	repost, _ := post.NewRepost(uuid.New(), "")
	for _, c := range []post.Content{withText, twoPhotos, repost} {
		if _, err = post.NewAvatarUpdate(ann, ann, c, now); !errors.Is(err, post.ErrAvatarUpdate) {
			t.Errorf("NewAvatarUpdate(%+v) = %v", c, err)
		}
	}
}

// textOnly is the content of a post without photos.
func textOnly(t *testing.T, text string) post.Content {
	t.Helper()

	c, err := post.NewContent(post.Text(text), nil)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestNewPhotos(t *testing.T) {
	t.Parallel()

	a, b := uuid.New(), uuid.New()
	ten := make([]uuid.UUID, 10)
	for i := range ten {
		ten[i] = uuid.New()
	}

	tests := []struct {
		name string
		ids  []uuid.UUID
		err  error
	}{
		{"none", nil, nil},
		{"two in order", []uuid.UUID{b, a}, nil},
		{"ten", ten, nil},
		{"eleven", append(ten, uuid.New()), post.ErrTooManyPhotos},
		{"the same twice", []uuid.UUID{a, b, a}, post.ErrDuplicatePhoto},
		{"zero id", []uuid.UUID{uuid.Nil}, post.ErrInvalidPhoto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			photos, err := post.NewPhotos(tt.ids)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewPhotos = %v, want %v", err, tt.err)
			}
			if err == nil && len(photos) != len(tt.ids) {
				t.Fatalf("photos = %v", photos)
			}
		})
	}
}

func TestNewContent(t *testing.T) {
	t.Parallel()

	photos, err := post.NewPhotos([]uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		text   post.Text
		photos post.Photos
		err    error
	}{
		{"text", "привет", nil, nil},
		{"photos only", "", photos, nil},
		{"both", "смотрите", photos, nil},
		{"nothing", "", nil, post.ErrTextEmpty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := post.NewContent(tt.text, tt.photos)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewContent = %v, want %v", err, tt.err)
			}
			if err == nil && (c.Text() != tt.text || len(c.Photos()) != len(tt.photos)) {
				t.Fatalf("content = %+v", c)
			}
		})
	}
}

func TestNewRepost(t *testing.T) {
	t.Parallel()

	original := uuid.New()

	c, err := post.NewRepost(original, "")
	if err != nil || c.RepostOf() != original || c.Text() != "" {
		t.Fatalf("a repost without a word = %+v, %v", c, err)
	}

	if _, err = post.NewRepost(uuid.Nil, "смотрите"); !errors.Is(err, post.ErrInvalidRepost) {
		t.Fatalf("a repost of nothing: %v, want ErrInvalidRepost", err)
	}

	if plain := textOnly(t, "привет"); plain.RepostOf() != uuid.Nil {
		t.Fatalf("a plain post shares %v", plain.RepostOf())
	}
}

func TestPost_Shared(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	original := uuid.New()

	plain, err := post.New(ann, ann, textOnly(t, "привет"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plain.Shared() != plain.ID() {
		t.Fatal("reposting a post shares the post itself")
	}

	silent, err := post.NewRepost(original, "")
	if err != nil {
		t.Fatal(err)
	}
	passedOn, err := post.New(bob, bob, silent, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if passedOn.Shared() != original {
		t.Fatal("reposting a repost without a word shares its original")
	}

	said, err := post.NewRepost(original, "согласен")
	if err != nil {
		t.Fatal(err)
	}
	commented, err := post.New(bob, bob, said, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if commented.Shared() != commented.ID() {
		t.Fatal("a repost with a word of its own is shared itself")
	}
}

func TestPost_Edited(t *testing.T) {
	t.Parallel()

	author, owner := uuid.New(), uuid.New()
	posted := time.Date(2026, 10, 4, 18, 30, 0, 0, time.UTC)
	photo := uuid.New()

	p, err := post.New(owner, author, textOnly(t, "привет"), posted)
	if err != nil {
		t.Fatal(err)
	}
	repost, err := post.New(author, author, mustRepost(t, uuid.New(), "смотрите"), posted)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		p      *post.Post
		editor uuid.UUID
		text   post.Text
		photos post.Photos
		at     time.Time
		err    error
	}{
		{"the author fixes a typo", p, author, "превед", nil, posted.Add(time.Hour), nil},
		{"photos instead of text", p, author, "", post.Photos{photo}, posted.Add(time.Hour), nil},
		{"on the last second of the week", p, author, "ок", nil, posted.Add(7 * 24 * time.Hour), nil},
		{"after a week", p, author, "поздно", nil, posted.Add(7*24*time.Hour + time.Second), post.ErrEditExpired},
		{"the owner of the wall does not edit others' words", p, owner, "чужое", nil, posted.Add(time.Hour), post.ErrCannotEdit},
		{"nothing left", p, author, "", nil, posted.Add(time.Hour), post.ErrTextEmpty},
		{"a repost changes its words", repost, author, "лучше так", nil, posted.Add(time.Hour), nil},
		{"a repost may say nothing", repost, author, "", nil, posted.Add(time.Hour), nil},
		{"a repost takes no photos", repost, author, "", post.Photos{photo}, posted.Add(time.Hour), post.ErrRepostPhotos},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			edited, err := tt.p.Edited(tt.editor, tt.text, tt.photos, tt.at)
			if !errors.Is(err, tt.err) {
				t.Fatalf("Edited: %v, want %v", err, tt.err)
			}
			if err != nil {
				return
			}

			if c := edited.Content(); c.Text() != tt.text || len(c.Photos()) != len(tt.photos) || c.RepostOf() != tt.p.Content().RepostOf() {
				t.Fatalf("edited content = %+v", c)
			}
			if !edited.EditedAt().Equal(tt.at) || edited.ID() != tt.p.ID() || edited.CreatedAt() != tt.p.CreatedAt() {
				t.Fatalf("edited at %v, id %v; want %v, the same post", edited.EditedAt(), edited.ID(), tt.at)
			}
			if !tt.p.EditedAt().IsZero() {
				t.Fatal("the original post changed: readers may hold it")
			}
		})
	}
}

func TestPost_Pinning(t *testing.T) {
	t.Parallel()

	author, owner := uuid.New(), uuid.New()
	p, err := post.New(owner, author, textOnly(t, "в гостях"), time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if !p.CanBePinnedBy(owner) || p.CanBePinnedBy(author) {
		t.Fatal("the owner of the wall pins on it, whoever wrote the post")
	}
	if p.Pinned() || !p.EditableUntil().Equal(p.CreatedAt().Add(7*24*time.Hour)) {
		t.Fatalf("pinned %v, editable until %v", p.Pinned(), p.EditableUntil())
	}
}

func mustRepost(t *testing.T, of uuid.UUID, text post.Text) post.Content {
	t.Helper()

	c, err := post.NewRepost(of, text)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestPost_InWallAlbum(t *testing.T) {
	t.Parallel()

	ann, bob, club := uuid.New(), uuid.New(), uuid.New()
	photos, _ := post.NewPhotos([]uuid.UUID{uuid.New()})
	content, _ := post.NewContent("", photos)

	own, _ := post.New(ann, ann, content, time.Now())
	guest, _ := post.New(bob, ann, content, time.Now())
	clubs, _ := post.NewFromWall(club, ann, content, false, time.Now())
	avatar, err := post.NewAvatarUpdate(ann, ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if !own.InWallAlbum() {
		t.Error("a post on one's own wall keeps its photos in the album")
	}
	for name, p := range map[string]*post.Post{"on another's wall": guest, "for a community": clubs, "a new photo of the page": avatar} {
		if p.InWallAlbum() {
			t.Errorf("a post %s keeps its photos in the album", name)
		}
	}
}

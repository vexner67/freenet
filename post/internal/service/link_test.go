package service_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// The picture of a card is a photo of the author that goes with the post.
func TestPostService_DeleteTakesTheCardPicture(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{},
		Notifier: &fakeNotifier{}, Photos: photos, Moderators: fakeModerators{}, Switches: store.Posts(),
	})

	ann, picture := uuid.New(), uuid.New()
	link, err := post.NewLink(post.LinkDraft{URL: "https://example.com", Title: "Example", PhotoID: picture})
	if err != nil {
		t.Fatal(err)
	}
	content, err := post.NewContent("https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	p, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: content.WithLink(link)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Published already by the gateway, as the side of a page is.
	if len(photos.published) != 0 {
		t.Fatalf("published = %v, want none", photos.published)
	}

	if err = svc.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if len(photos.deleted) != 1 || photos.deleted[0].owner != ann || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{picture}) {
		t.Fatalf("deleted = %+v, want the picture of ann", photos.deleted)
	}
}

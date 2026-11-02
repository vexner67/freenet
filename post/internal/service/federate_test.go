package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type federated struct {
	change string
	postID uuid.UUID
}

type fakeFederator struct {
	told []federated
}

func (f *fakeFederator) Published(_ context.Context, p *post.Post) error {
	f.told = append(f.told, federated{"published", p.ID()})
	return nil
}

func (f *fakeFederator) Edited(_ context.Context, p *post.Post) error {
	f.told = append(f.told, federated{"edited", p.ID()})
	return nil
}

func (f *fakeFederator) Deleted(_ context.Context, p *post.Post) error {
	f.told = append(f.told, federated{"deleted", p.ID()})
	return nil
}

// Only what authors write on their own walls goes to the federator: a
// post on another's wall or for a community never reaches the Fediverse.
func TestPostService_Federates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	federator := &fakeFederator{}
	ann, bob, club := uuid.New(), uuid.New(), uuid.New()

	svc := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{},
		Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: fakeModerators{{club, ann}: true}, Switches: store.Posts(),
		Federator: federator,
	})

	own, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: textContent(t, "моя стена")})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = svc.Create(ctx, service.CreateInput{WallID: bob, AuthorID: ann, Content: textContent(t, "стена Бориса")}); err != nil {
		t.Fatal(err)
	}

	if _, err = svc.Create(ctx, service.CreateInput{WallID: club, AuthorID: ann, Content: textContent(t, "группа"), ClubWall: true, FromWall: true}); err != nil {
		t.Fatal(err)
	}

	if _, _, err = svc.Edit(ctx, ann, own.ID(), service.PostEdit{Text: post.Text("правка")}); err != nil {
		t.Fatal(err)
	}

	if err = svc.Delete(ctx, ann, own.ID()); err != nil {
		t.Fatal(err)
	}

	want := []federated{{"published", own.ID()}, {"edited", own.ID()}, {"deleted", own.ID()}}
	if len(federator.told) != len(want) {
		t.Fatalf("told %+v, want %+v", federator.told, want)
	}

	for i := range want {
		if federator.told[i] != want[i] {
			t.Fatalf("told %+v, want %+v", federator.told, want)
		}
	}
}

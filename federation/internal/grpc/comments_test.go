package grpc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/fedi"
	federationgrpc "github.com/vexner67/freenet/federation/internal/grpc"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type commentsFake struct {
	newsFake

	written  string
	after    uuid.UUID
	page     service.CommentPage
	deleted  uuid.UUID
	deleter  uuid.UUID
	reader   uuid.UUID
	newsPost service.NewsPost
}

func (f *commentsFake) Comment(_ context.Context, userID, postID uuid.UUID, text string) (*fedi.RemoteComment, error) {
	f.written = text
	return fedi.RestoreRemoteComment(uuid.New(), postID, userID, text, time.Now()), f.err
}

func (f *commentsFake) Comments(_ context.Context, _, after uuid.UUID, _ int) (service.CommentPage, error) {
	f.after = after
	return f.page, f.err
}

func (f *commentsFake) DeleteComment(_ context.Context, userID, id uuid.UUID) error {
	f.deleter, f.deleted = userID, id
	return f.err
}

func (f *commentsFake) NewsPost(_ context.Context, _, reader uuid.UUID) (service.NewsPost, error) {
	f.reader = reader
	return f.newsPost, f.err
}

func TestNewsComments(t *testing.T) {
	t.Parallel()

	reader := uuid.New()
	ctx := jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: reader})
	postID := uuid.New()
	f := &commentsFake{}
	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatal(err)
	}
	hc := federationgrpc.NewFederationHandler(f, f, site)

	written, err := hc.CommentNews(ctx, &federationv1.CommentNewsRequest{PostId: postID.String(), Text: "привет"})
	c := written.GetComment()
	if err != nil || f.written != "привет" || c.GetAuthorId() != reader.String() || c.GetPostId() != postID.String() || c.GetCreatedAt() == nil {
		t.Fatalf("CommentNews = %v, %v", written, err)
	}

	first, next := fedi.RestoreRemoteComment(uuid.New(), postID, reader, "раз", time.Now()), uuid.New()
	f.page = service.CommentPage{Comments: []*fedi.RemoteComment{first}, Next: next, Total: 3}
	cursor := uuid.New()
	list, err := hc.ListNewsComments(ctx, &federationv1.ListNewsCommentsRequest{PostId: postID.String(), Cursor: cursor.String(), Limit: 10})
	if err != nil || len(list.GetComments()) != 1 || list.GetComments()[0].GetText() != "раз" || list.GetNextCursor() != next.String() ||
		list.GetTotal() != 3 || f.after != cursor {
		t.Fatalf("ListNewsComments = %v, %v", list, err)
	}

	id := uuid.New()
	if _, err = hc.DeleteNewsComment(ctx, &federationv1.DeleteNewsCommentRequest{Id: id.String()}); err != nil || f.deleted != id || f.deleter != reader {
		t.Fatalf("DeleteNewsComment: %v; deleted %v by %v", err, f.deleted, f.deleter)
	}

	author := fedi.RestoreRemote("https://m.example/users/ann", "", "", "", "", fedi.Profile{Name: "Ann"}, time.Now())
	post, err := fedi.NewRemotePost(fedi.NoteDoc{
		ID: "https://m.example/notes/1", AttributedTo: author.ID().String(), Text: "hi", Public: true, Published: time.Now(),
	}, author.ID(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.newsPost = service.NewsPost{Post: post, Author: author, Comments: 4}
	got, err := hc.GetNewsPost(ctx, &federationv1.GetNewsPostRequest{Id: post.ID().String()})
	if err != nil || got.GetPost().GetComments() != 4 || got.GetPost().GetId() != post.ID().String() || f.reader != reader {
		t.Fatalf("GetNewsPost = %v, %v", got, err)
	}

	for name, call := range map[string]func() error{
		"comment bad post": func() error {
			_, err := hc.CommentNews(ctx, &federationv1.CommentNewsRequest{PostId: "x", Text: "a"})
			return err
		},
		"list no limit": func() error {
			_, err := hc.ListNewsComments(ctx, &federationv1.ListNewsCommentsRequest{PostId: postID.String()})
			return err
		},
		"list bad cursor": func() error {
			_, err := hc.ListNewsComments(ctx, &federationv1.ListNewsCommentsRequest{PostId: postID.String(), Limit: 5, Cursor: "x"})
			return err
		},
		"delete bad id": func() error {
			_, err := hc.DeleteNewsComment(ctx, &federationv1.DeleteNewsCommentRequest{Id: "x"})
			return err
		},
		"get bad id": func() error {
			_, err := hc.GetNewsPost(ctx, &federationv1.GetNewsPostRequest{Id: "x"})
			return err
		},
	} {
		if err := call(); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: code %v, want InvalidArgument", name, status.Code(err))
		}
	}

	if _, err = hc.CommentNews(context.Background(), &federationv1.CommentNewsRequest{PostId: postID.String(), Text: "a"}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("without a token: code %v", status.Code(err))
	}
}

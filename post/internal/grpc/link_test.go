package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestLinks(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()
	card := &postv1.Link{Url: "https://example.com/a", Title: "Пример", Description: "Страница.", Site: "Example", PhotoId: uuid.NewString()}

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "Смотрите: https://example.com/a", Link: card})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	got, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: created.GetPost().GetId()})
	if err != nil || !proto.Equal(got.GetPost().GetLink(), card) {
		t.Fatalf("the card = %v, %v; want %v", got.GetPost().GetLink(), err, card)
	}

	for name, link := range map[string]*postv1.Link{
		"no title":         {Url: "https://example.com"},
		"another scheme":   {Url: "ftp://example.com", Title: "Example"},
		"a broken picture": {Url: "https://example.com", Title: "Example", PhotoId: "nope"},
	} {
		if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "https://example.com", Link: link}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%s: %v, want InvalidArgument", name, err)
		}
	}
}

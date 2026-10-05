package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	postgrpc "github.com/vexner67/freenet/post/internal/grpc"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const issuer = "freenet-test"

type testServer struct {
	client postv1.PostServiceClient
	key    *rsa.PrivateKey
}

// newServer wires the handler the same way app.New does.
func newServer(t *testing.T) testServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		postgrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer), postgrpc.PublicMethods()...),
	))
	store := inmem.NewStore()
	postv1.RegisterPostServiceServer(srv, postgrpc.NewPostHandler(
		service.NewPostService(store.Posts(), store.Likes()),
		service.NewCommentService(store.Posts(), store.Comments()),
		log,
	))

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return testServer{client: postv1.NewPostServiceClient(conn), key: key}
}

// as returns a context of the given user with a signed access token.
func (s testServer) as(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}

	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestWall(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	for _, text := range []string{"первая", "вторая", "третья"} {
		if _, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: text}); err != nil {
			t.Fatalf("CreatePost: %v", err)
		}
	}

	page, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{AuthorId: ann.String(), Limit: 2})
	if err != nil {
		t.Fatalf("ListPosts without a token: %v", err)
	}
	if page.GetTotal() != 3 || len(page.GetPosts()) != 2 || page.GetPosts()[0].GetText() != "третья" || page.GetNextCursor() == "" {
		t.Fatalf("first page: %v", page)
	}
	if page.GetPosts()[0].GetAuthorId() != ann.String() || page.GetPosts()[0].GetCreatedAt() == nil {
		t.Fatalf("post fields: %v", page.GetPosts()[0])
	}

	last, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{AuthorId: ann.String(), Limit: 2, Cursor: page.GetNextCursor()})
	if err != nil {
		t.Fatal(err)
	}
	if len(last.GetPosts()) != 1 || last.GetPosts()[0].GetText() != "первая" || last.GetNextCursor() != "" {
		t.Fatalf("last page: %v", last)
	}

	if _, err = s.client.CreatePost(s.as(t, bob), &postv1.CreatePostRequest{Text: "от боба"}); err != nil {
		t.Fatal(err)
	}

	feed, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{AuthorIds: []string{ann.String(), bob.String()}, Limit: 2})
	if err != nil {
		t.Fatalf("ListFeed: %v", err)
	}
	if len(feed.GetPosts()) != 2 || feed.GetPosts()[0].GetText() != "от боба" || feed.GetPosts()[1].GetText() != "третья" || feed.GetNextCursor() == "" {
		t.Fatalf("feed: %v", feed)
	}

	victim := page.GetPosts()[0].GetId()

	_, err = s.client.DeletePost(s.as(t, bob), &postv1.DeletePostRequest{Id: victim})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("stranger deletes: %v", err)
	}

	if _, err = s.client.DeletePost(s.as(t, ann), &postv1.DeletePostRequest{Id: victim}); err != nil {
		t.Fatalf("author deletes: %v", err)
	}

	_, err = s.client.DeletePost(s.as(t, ann), &postv1.DeletePostRequest{Id: victim})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("deleted twice: %v", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := s.as(t, uuid.New())

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"create without token", func() error {
			_, err := s.client.CreatePost(context.Background(), &postv1.CreatePostRequest{Text: "x"})
			return err
		}, codes.Unauthenticated},
		{"empty text", func() error {
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{Text: "  \n "})
			return err
		}, codes.InvalidArgument},
		{"too long text", func() error {
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{Text: strings.Repeat("я", 1001)})
			return err
		}, codes.InvalidArgument},
		{"bad author id", func() error {
			_, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{AuthorId: "ann"})
			return err
		}, codes.InvalidArgument},
		{"bad cursor", func() error {
			_, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{AuthorId: uuid.NewString(), Cursor: "x"})
			return err
		}, codes.InvalidArgument},
		{"bad feed author", func() error {
			_, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{AuthorIds: []string{"ann"}})
			return err
		}, codes.InvalidArgument},
		{"too many feed authors", func() error {
			ids := make([]string, 1002)
			for i := range ids {
				ids[i] = uuid.NewString()
			}
			_, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{AuthorIds: ids})
			return err
		}, codes.InvalidArgument},
		{"bad post id", func() error {
			_, err := s.client.DeletePost(ann, &postv1.DeletePostRequest{Id: "x"})
			return err
		}, codes.InvalidArgument},
		{"get a missing post", func() error {
			_, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"like without token", func() error {
			_, err := s.client.LikePost(context.Background(), &postv1.LikePostRequest{PostId: uuid.NewString()})
			return err
		}, codes.Unauthenticated},
		{"like a bad post id", func() error {
			_, err := s.client.LikePost(ann, &postv1.LikePostRequest{PostId: "x"})
			return err
		}, codes.InvalidArgument},
		{"like a missing post", func() error {
			_, err := s.client.LikePost(ann, &postv1.LikePostRequest{PostId: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"unlike without token", func() error {
			_, err := s.client.UnlikePost(context.Background(), &postv1.UnlikePostRequest{PostId: uuid.NewString()})
			return err
		}, codes.Unauthenticated},
		{"comment without token", func() error {
			_, err := s.client.CreateComment(context.Background(), &postv1.CreateCommentRequest{PostId: uuid.NewString(), Text: "x"})
			return err
		}, codes.Unauthenticated},
		{"empty comment", func() error {
			_, err := s.client.CreateComment(ann, &postv1.CreateCommentRequest{PostId: uuid.NewString(), Text: " "})
			return err
		}, codes.InvalidArgument},
		{"too long comment", func() error {
			_, err := s.client.CreateComment(ann, &postv1.CreateCommentRequest{PostId: uuid.NewString(), Text: strings.Repeat("я", 1001)})
			return err
		}, codes.InvalidArgument},
		{"comment a missing post", func() error {
			_, err := s.client.CreateComment(ann, &postv1.CreateCommentRequest{PostId: uuid.NewString(), Text: "x"})
			return err
		}, codes.NotFound},
		{"delete a missing comment", func() error {
			_, err := s.client.DeleteComment(ann, &postv1.DeleteCommentRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"delete a bad comment id", func() error {
			_, err := s.client.DeleteComment(ann, &postv1.DeleteCommentRequest{Id: "x"})
			return err
		}, codes.InvalidArgument},
		{"comments of a missing post", func() error {
			_, err := s.client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"comments with a bad cursor", func() error {
			_, err := s.client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: uuid.NewString(), Cursor: "x"})
			return err
		}, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := status.Code(tt.call()); got != tt.want {
				t.Fatalf("code = %v, want %v", got, tt.want)
			}
		})
	}
}

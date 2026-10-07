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
	"github.com/vexner67/freenet/post/internal/mediaclient"
	"github.com/vexner67/freenet/post/internal/notifyclient"
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
		jwtauth.UnaryServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer), jwtauth.Access{Public: postgrpc.PublicMethods()}),
	))
	store := inmem.NewStore()
	postv1.RegisterPostServiceServer(srv, postgrpc.NewPostHandler(
		service.NewPostService(service.PostDeps{Posts: store.Posts(), Likes: store.Likes(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}}),
		service.NewCommentService(store.Posts(), store.Comments(), inmem.TxManager{}, notifyclient.Nop{}),
		service.NewPurgeService(service.PurgeDeps{
			Posts: store.Posts(), Likes: store.Likes(), Comments: store.Comments(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{},
		}),
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

	page, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String(), Limit: 2})
	if err != nil {
		t.Fatalf("ListPosts without a token: %v", err)
	}
	if page.GetTotal() != 3 || len(page.GetPosts()) != 2 || page.GetPosts()[0].GetText() != "третья" || page.GetNextCursor() == "" {
		t.Fatalf("first page: %v", page)
	}
	if page.GetPosts()[0].GetAuthorId() != ann.String() || page.GetPosts()[0].GetCreatedAt() == nil {
		t.Fatalf("post fields: %v", page.GetPosts()[0])
	}

	last, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String(), Limit: 2, Cursor: page.GetNextCursor()})
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
		{"neither text nor photos", func() error {
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{Text: "  \n "})
			return err
		}, codes.InvalidArgument},
		{"eleven photos", func() error {
			ids := make([]string, 11)
			for i := range ids {
				ids[i] = uuid.NewString()
			}
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{PhotoIds: ids})
			return err
		}, codes.InvalidArgument},
		{"the same photo twice", func() error {
			id := uuid.NewString()
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{PhotoIds: []string{id, id}})
			return err
		}, codes.InvalidArgument},
		{"bad photo id", func() error {
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{PhotoIds: []string{"x"}})
			return err
		}, codes.InvalidArgument},
		{"too long text", func() error {
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{Text: strings.Repeat("я", 1001)})
			return err
		}, codes.InvalidArgument},
		{"bad author id", func() error {
			_, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: "ann"})
			return err
		}, codes.InvalidArgument},
		{"bad cursor", func() error {
			_, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: uuid.NewString(), Cursor: "x"})
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
		{"too many posts", func() error {
			_, err := s.client.GetPosts(context.Background(), &postv1.GetPostsRequest{Ids: make([]string, 51)})
			return err
		}, codes.InvalidArgument},
		{"bad id in GetComments", func() error {
			_, err := s.client.GetComments(context.Background(), &postv1.GetCommentsRequest{Ids: []string{"x"}})
			return err
		}, codes.InvalidArgument},
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

func TestPhotos(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()
	photos := []string{uuid.NewString(), uuid.NewString()}

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{PhotoIds: photos})
	if err != nil {
		t.Fatalf("a post of photos only: %v", err)
	}
	if p := created.GetPost(); p.GetText() != "" || len(p.GetPhotoIds()) != 2 || p.GetPhotoIds()[0] != photos[0] {
		t.Fatalf("created = %v; photos keep their order", p)
	}

	got, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: created.GetPost().GetId()})
	if err != nil || len(got.GetPost().GetPhotoIds()) != 2 || got.GetPost().GetPhotoIds()[1] != photos[1] {
		t.Fatalf("GetPost = %v, %v", got, err)
	}

	wall, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String()})
	if err != nil || len(wall.GetPosts()[0].GetPhotoIds()) != 2 {
		t.Fatalf("ListPosts = %v, %v", wall, err)
	}
}

func TestSomeoneElsesWall(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, bob), &postv1.CreatePostRequest{Text: "привет", WallId: ann.String()})
	if err != nil {
		t.Fatalf("CreatePost on ann's wall: %v", err)
	}
	if p := created.GetPost(); p.GetWallId() != ann.String() || p.GetAuthorId() != bob.String() {
		t.Fatalf("post = %v", p)
	}

	own, err := s.client.CreatePost(s.as(t, bob), &postv1.CreatePostRequest{Text: "своё"})
	if err != nil || own.GetPost().GetWallId() != bob.String() {
		t.Fatalf("without wall_id the post goes on the author's wall: %v, %v", own, err)
	}

	wall, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String()})
	if err != nil || wall.GetTotal() != 1 || wall.GetPosts()[0].GetAuthorId() != bob.String() {
		t.Fatalf("ann's wall = %v, %v", wall, err)
	}

	_, err = s.client.CreatePost(s.as(t, bob), &postv1.CreatePostRequest{Text: "x", WallId: "ann"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("bad wall id: code = %v", got)
	}

	_, err = s.client.DeletePost(s.as(t, cat), &postv1.DeletePostRequest{Id: created.GetPost().GetId()})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("a stranger deletes: code = %v", got)
	}
	if _, err = s.client.DeletePost(s.as(t, ann), &postv1.DeletePostRequest{Id: created.GetPost().GetId()}); err != nil {
		t.Fatalf("the owner of the wall deletes: %v", err)
	}
}

func TestRepostPost(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	original, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "оригинал"})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := s.client.RepostPost(s.as(t, bob), &postv1.RepostPostRequest{PostId: original.GetPost().GetId(), Text: " смотрите "})
	if err != nil {
		t.Fatalf("RepostPost: %v", err)
	}
	if p := resp.GetPost(); p.GetRepostOf() != original.GetPost().GetId() || p.GetWallId() != bob.String() || p.GetText() != "смотрите" {
		t.Fatalf("repost = %v", p)
	}

	got, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: original.GetPost().GetId()})
	if err != nil || got.GetPost().GetRepostCount() != 1 || got.GetPost().GetRepostOf() != "" {
		t.Fatalf("original = %v, %v", got.GetPost(), err)
	}

	tests := []struct {
		name string
		ctx  context.Context
		req  *postv1.RepostPostRequest
		code codes.Code
	}{
		{"without a token", context.Background(), &postv1.RepostPostRequest{PostId: original.GetPost().GetId()}, codes.Unauthenticated},
		{"bad id", s.as(t, bob), &postv1.RepostPostRequest{PostId: "x"}, codes.InvalidArgument},
		{"missing post", s.as(t, bob), &postv1.RepostPostRequest{PostId: uuid.NewString()}, codes.NotFound},
		{"text too long", s.as(t, bob), &postv1.RepostPostRequest{PostId: original.GetPost().GetId(), Text: strings.Repeat("я", 1001)}, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.client.RepostPost(tt.ctx, tt.req)
			if got := status.Code(err); got != tt.code {
				t.Fatalf("code = %v, want %v", got, tt.code)
			}
		})
	}
}

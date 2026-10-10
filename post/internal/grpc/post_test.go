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
	"github.com/vexner67/freenet/post/internal/clubclient"
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
	suggestions := inmem.NewSuggestionRepository()
	topics := inmem.NewTopicRepository()
	albums := inmem.NewAlbumRepository()
	albumService := service.NewAlbumService(service.AlbumDeps{
		Albums: albums, Photos: albums.Photos(), Tx: inmem.TxManager{}, Media: mediaclient.Nop{}, Moderators: clubclient.Nop{},
		Likes: albums.PhotoLikes(), Comments: albums.PhotoComments(),
	})
	postv1.RegisterPostServiceServer(srv, postgrpc.NewPostHandler(postgrpc.Services{
		Posts:    service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{}, AlbumPhotos: albums.Photos(), WallPhotos: albumService, Switches: store.Posts()}),
		Comments: service.NewCommentService(service.CommentDeps{Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{}}),
		Suggestions: service.NewSuggestionService(service.SuggestionDeps{
			Suggestions: suggestions, Posts: store.Posts(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{},
		}),
		Topics: service.NewTopicService(service.TopicDeps{
			Topics: topics, Messages: topics.Messages(), Likes: topics.Messages(), Tx: inmem.TxManager{},
			Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{},
		}),
		Albums: albumService,
		Purge: service.NewPurgeService(service.PurgeDeps{
			Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Comments: store.Comments(), Suggestions: suggestions,
			Topics: topics, Albums: albums, Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{},
		}),
	}, log))

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

	if n, err := s.client.CountPosts(context.Background(), &postv1.CountPostsRequest{}); err != nil || n.GetCount() < 1 {
		t.Fatalf("CountPosts = %v, %v", n, err)
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
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{Text: strings.Repeat("я", 60001)})
			return err
		}, codes.InvalidArgument},
		{"speaking for one's own wall", func() error {
			_, err := s.client.CreatePost(ann, &postv1.CreatePostRequest{Text: "я", FromWall: true})
			return err
		}, codes.InvalidArgument},
		{"a bad wall in the feed", func() error {
			_, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{WallIds: []string{"x"}})
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
			_, err := s.client.CreateComment(ann, &postv1.CreateCommentRequest{PostId: uuid.NewString(), Text: strings.Repeat("я", 60001)})
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
		{"text too long", s.as(t, bob), &postv1.RepostPostRequest{PostId: original.GetPost().GetId(), Text: strings.Repeat("я", 60001)}, codes.InvalidArgument},
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

func TestEditAndPin(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	// Bob writes on ann's wall.
	created, err := s.client.CreatePost(s.as(t, bob), &postv1.CreatePostRequest{Text: "привет", WallId: ann.String()})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetPost().GetId()
	if created.GetPost().GetEditedAt() != nil || created.GetPost().GetEditableUntil() == nil {
		t.Fatalf("new post: edited at %v, editable until %v", created.GetPost().GetEditedAt(), created.GetPost().GetEditableUntil())
	}

	edited, err := s.client.EditPost(s.as(t, bob), &postv1.EditPostRequest{Id: id, Text: "  привет, Аня \r\n"})
	if err != nil {
		t.Fatalf("EditPost: %v", err)
	}
	if p := edited.GetPost(); p.GetText() != "привет, Аня" || p.GetEditedAt() == nil {
		t.Fatalf("edited post = %v", p)
	}

	for _, tt := range []struct {
		name string
		call func() error
		code codes.Code
	}{
		{"the owner of the wall does not edit bob's words", func() error {
			_, err := s.client.EditPost(s.as(t, ann), &postv1.EditPostRequest{Id: id, Text: "иначе"})
			return err
		}, codes.PermissionDenied},
		{"nothing left", func() error {
			_, err := s.client.EditPost(s.as(t, bob), &postv1.EditPostRequest{Id: id, Text: " "})
			return err
		}, codes.InvalidArgument},
		{"a bad photo", func() error {
			_, err := s.client.EditPost(s.as(t, bob), &postv1.EditPostRequest{Id: id, PhotoIds: []string{"x"}})
			return err
		}, codes.InvalidArgument},
		{"edit without a token", func() error {
			_, err := s.client.EditPost(context.Background(), &postv1.EditPostRequest{Id: id, Text: "x"})
			return err
		}, codes.Unauthenticated},
		{"bob does not pin on ann's wall", func() error {
			_, err := s.client.PinPost(s.as(t, bob), &postv1.PinPostRequest{Id: id})
			return err
		}, codes.PermissionDenied},
		{"pin a missing post", func() error {
			_, err := s.client.PinPost(s.as(t, ann), &postv1.PinPostRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
	} {
		if err := tt.call(); status.Code(err) != tt.code {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.code)
		}
	}

	if _, err = s.client.PinPost(s.as(t, ann), &postv1.PinPostRequest{Id: id}); err != nil {
		t.Fatalf("PinPost: %v", err)
	}
	got, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: id})
	if err != nil || !got.GetPost().GetPinned() {
		t.Fatalf("GetPost of the pinned post = %v, %v", got, err)
	}
	if _, err = s.client.UnpinPost(s.as(t, ann), &postv1.UnpinPostRequest{Id: id}); err != nil {
		t.Fatalf("UnpinPost: %v", err)
	}
	if got, _ = s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: id}); got.GetPost().GetPinned() {
		t.Fatal("the post is still pinned")
	}
}

func TestClubWall(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	club, admin := uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, admin), &postv1.CreatePostRequest{Text: "новости", WallId: club.String(), ClubWall: true, FromWall: true, Signed: true})
	if err != nil || !created.GetPost().GetFromWall() || created.GetPost().GetWallId() != club.String() || !created.GetPost().GetMine() || !created.GetPost().GetSigned() {
		t.Fatalf("CreatePost = %v, %v", created, err)
	}

	// Only its author is told it is theirs.
	got, err := s.client.GetPost(s.as(t, admin), &postv1.GetPostRequest{Id: created.GetPost().GetId()})
	if err != nil || !got.GetPost().GetMine() {
		t.Fatalf("GetPost by the author = %v, %v", got, err)
	}
	for _, ctx := range []context.Context{s.as(t, uuid.New()), context.Background()} {
		if got, err = s.client.GetPost(ctx, &postv1.GetPostRequest{Id: created.GetPost().GetId()}); err != nil || got.GetPost().GetMine() {
			t.Fatalf("GetPost by someone else = %v, %v", got, err)
		}
	}

	feed, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{WallIds: []string{club.String()}})
	if err != nil || len(feed.GetPosts()) != 1 || !feed.GetPosts()[0].GetFromWall() {
		t.Fatalf("ListFeed of the club = %v, %v", feed, err)
	}

	byAuthor, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{AuthorIds: []string{admin.String()}})
	if err != nil || len(byAuthor.GetPosts()) != 0 {
		t.Fatalf("ListFeed of the admin = %v, %v; the post is the club's", byAuthor, err)
	}
}

func TestAvatarUpdate(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	club, ann, photo := uuid.New(), uuid.New(), uuid.New()

	own, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{PhotoIds: []string{photo.String()}, AvatarUpdate: true})
	if err != nil || !own.GetPost().GetAvatarUpdate() || own.GetPost().GetFromWall() || own.GetPost().GetWallId() != ann.String() {
		t.Fatalf("on ann's wall = %v, %v", own, err)
	}
	if !own.GetPost().GetEditableUntil().AsTime().Equal(own.GetPost().GetCreatedAt().AsTime()) {
		t.Fatalf("editable until %v", own.GetPost().GetEditableUntil())
	}
	_, err = s.client.EditPost(s.as(t, ann), &postv1.EditPostRequest{Id: own.GetPost().GetId(), Text: "подпись"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("EditPost = %v", err)
	}

	clubs, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{PhotoIds: []string{uuid.NewString()}, WallId: club.String(), ClubWall: true, FromWall: true, AvatarUpdate: true})
	if err != nil || !clubs.GetPost().GetAvatarUpdate() || !clubs.GetPost().GetFromWall() {
		t.Fatalf("for the club = %v, %v", clubs, err)
	}

	_, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "новое фото", PhotoIds: []string{uuid.NewString()}, AvatarUpdate: true})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("with words = %v", err)
	}
}

func TestSuggestions(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	club := uuid.NewString()

	suggested, err := s.client.SuggestPost(s.as(t, ann), &postv1.SuggestPostRequest{WallId: club, Text: " Котик нашёлся "})
	if err != nil || suggested.GetSuggestion().GetText() != "Котик нашёлся" || suggested.GetSuggestion().GetAuthorId() != ann.String() ||
		suggested.GetSuggestion().GetWallId() != club || suggested.GetSuggestion().GetCreatedAt() == nil {
		t.Fatalf("SuggestPost = %v, %v", suggested, err)
	}
	id := suggested.GetSuggestion().GetId()

	mine, err := s.client.ListSuggestions(s.as(t, ann), &postv1.ListSuggestionsRequest{WallId: club, Mine: true})
	if err != nil || len(mine.GetSuggestions()) != 1 || mine.GetTotal() != 1 || mine.GetNextCursor() != "" {
		t.Fatalf("ListSuggestions = %v, %v", mine, err)
	}

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"suggest without a token", func() error {
			_, err := s.client.SuggestPost(context.Background(), &postv1.SuggestPostRequest{WallId: club, Text: "x"})
			return err
		}, codes.Unauthenticated},
		{"suggest nothing", func() error {
			_, err := s.client.SuggestPost(s.as(t, ann), &postv1.SuggestPostRequest{WallId: club})
			return err
		}, codes.InvalidArgument},
		{"suggest for a bad wall", func() error {
			_, err := s.client.SuggestPost(s.as(t, ann), &postv1.SuggestPostRequest{WallId: "x", Text: "x"})
			return err
		}, codes.InvalidArgument},
		{"all of them, not an admin", func() error {
			_, err := s.client.ListSuggestions(s.as(t, ann), &postv1.ListSuggestionsRequest{WallId: club})
			return err
		}, codes.PermissionDenied},
		{"publish, not an admin", func() error {
			_, err := s.client.PublishSuggestion(s.as(t, ann), &postv1.PublishSuggestionRequest{Id: id, Text: "x"})
			return err
		}, codes.PermissionDenied},
		{"withdraw someone else's", func() error {
			_, err := s.client.DeleteSuggestion(s.as(t, bob), &postv1.DeleteSuggestionRequest{Id: id})
			return err
		}, codes.PermissionDenied},
		{"publish a missing one", func() error {
			_, err := s.client.PublishSuggestion(s.as(t, ann), &postv1.PublishSuggestionRequest{Id: uuid.NewString(), Text: "x"})
			return err
		}, codes.NotFound},
		{"a bad id", func() error {
			_, err := s.client.DeleteSuggestion(s.as(t, ann), &postv1.DeleteSuggestionRequest{Id: "x"})
			return err
		}, codes.InvalidArgument},
	}
	for _, tt := range tests {
		if got := status.Code(tt.call()); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}

	if _, err = s.client.DeleteSuggestion(s.as(t, ann), &postv1.DeleteSuggestionRequest{Id: id}); err != nil {
		t.Fatalf("the author withdraws: %v", err)
	}
}

func TestTopics(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateTopic(s.as(t, ann), &postv1.CreateTopicRequest{ClubId: club.String(), Title: "  Знакомство ", Text: "Привет!"})
	if err != nil || created.GetTopic().GetTitle() != "Знакомство" || created.GetTopic().GetMessageCount() != 1 || !created.GetTopic().GetMine() {
		t.Fatalf("CreateTopic = %v, %v", created, err)
	}
	id := created.GetTopic().GetId()

	if _, err = s.client.CreateTopic(s.as(t, ann), &postv1.CreateTopicRequest{ClubId: club.String(), Title: " "}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("no title: %v", err)
	}

	// Public, with the newest message and whose it is.
	list, err := s.client.ListTopics(context.Background(), &postv1.ListTopicsRequest{ClubId: club.String()})
	if err != nil || len(list.GetTopics()) != 1 || list.GetTotal() != 1 || list.GetTopics()[0].GetLastMessage().GetText() != "Привет!" || list.GetTopics()[0].GetMine() {
		t.Fatalf("ListTopics = %v, %v", list, err)
	}
	if _, err = s.client.ListTopics(context.Background(), &postv1.ListTopicsRequest{ClubId: club.String(), Cursor: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad cursor: %v", err)
	}

	msg, err := s.client.CreateTopicMessage(s.as(t, bob), &postv1.CreateTopicMessageRequest{TopicId: id, Text: "И тебе"})
	if err != nil || !msg.GetMessage().GetMine() {
		t.Fatalf("CreateTopicMessage = %v, %v", msg, err)
	}
	messages, err := s.client.ListTopicMessages(context.Background(), &postv1.ListTopicMessagesRequest{TopicId: id})
	if err != nil || len(messages.GetMessages()) != 2 || messages.GetTotal() != 2 || messages.GetMessages()[1].GetId() != msg.GetMessage().GetId() {
		t.Fatalf("ListTopicMessages = %v, %v", messages, err)
	}
	// A heart on the message, told to whoever put it.
	liked, err := s.client.LikeTopicMessage(s.as(t, ann), &postv1.LikeTopicMessageRequest{Id: msg.GetMessage().GetId()})
	if err != nil || liked.GetLikeCount() != 1 {
		t.Fatalf("LikeTopicMessage = %v, %v", liked, err)
	}
	if _, err = s.client.LikeTopicMessage(context.Background(), &postv1.LikeTopicMessageRequest{Id: msg.GetMessage().GetId()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("LikeTopicMessage without a token: %v", err)
	}
	if _, err = s.client.LikeTopicMessage(s.as(t, ann), &postv1.LikeTopicMessageRequest{Id: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("LikeTopicMessage of nothing: %v", err)
	}
	if messages, err = s.client.ListTopicMessages(s.as(t, ann), &postv1.ListTopicMessagesRequest{TopicId: id}); err != nil ||
		!messages.GetMessages()[1].GetLiked() || messages.GetMessages()[1].GetLikeCount() != 1 || messages.GetMessages()[0].GetLiked() {
		t.Fatalf("ListTopicMessages to ann = %v, %v", messages, err)
	}
	if unliked, err := s.client.UnlikeTopicMessage(s.as(t, ann), &postv1.UnlikeTopicMessageRequest{Id: msg.GetMessage().GetId()}); err != nil || unliked.GetLikeCount() != 0 {
		t.Fatalf("UnlikeTopicMessage = %v, %v", unliked, err)
	}

	if _, err = s.client.DeleteTopicMessage(s.as(t, ann), &postv1.DeleteTopicMessageRequest{Id: msg.GetMessage().GetId()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ann deletes bob's message: %v", err)
	}

	_, err = s.client.EditTopic(s.as(t, ann), &postv1.EditTopicRequest{Id: id, Title: "Знакомство", Restricted: true})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ann restricts: %v", err)
	}
	edited, err := s.client.EditTopic(s.as(t, ann), &postv1.EditTopicRequest{Id: id, Title: "Знакомимся", Closed: true})
	if err != nil || !edited.GetTopic().GetClosed() || edited.GetTopic().GetTitle() != "Знакомимся" {
		t.Fatalf("EditTopic = %v, %v", edited, err)
	}
	if _, err = s.client.CreateTopicMessage(s.as(t, bob), &postv1.CreateTopicMessageRequest{TopicId: id, Text: "ещё"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("writes in a closed topic: %v", err)
	}

	if _, err = s.client.DeleteTopic(s.as(t, bob), &postv1.DeleteTopicRequest{Id: id}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob deletes: %v", err)
	}
	if _, err = s.client.DeleteTopic(s.as(t, ann), &postv1.DeleteTopicRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetTopic(context.Background(), &postv1.GetTopicRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetTopic after delete: %v", err)
	}
}

func TestAlbums(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateAlbum(s.as(t, ann), &postv1.CreateAlbumRequest{Name: "  Лето ", Description: "На даче"})
	if err != nil || created.GetAlbum().GetName() != "Лето" || created.GetAlbum().GetOwnerId() != ann.String() || created.GetAlbum().GetCover() != nil {
		t.Fatalf("CreateAlbum = %v, %v", created, err)
	}
	id := created.GetAlbum().GetId()

	for name, req := range map[string]*postv1.CreateAlbumRequest{
		"no name":    {Name: " "},
		"long name":  {Name: strings.Repeat("я", 37)},
		"bad club":   {ClubId: "x", Name: "Кошки"},
		"long words": {Name: "Лето", Description: strings.Repeat("я", 256)},
	} {
		if _, err = s.client.CreateAlbum(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("CreateAlbum with %s: %v", name, err)
		}
	}
	// Nobody runs the club here (clubclient.Nop).
	if _, err = s.client.CreateAlbum(s.as(t, ann), &postv1.CreateAlbumRequest{ClubId: club.String(), Name: "Кошки"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("CreateAlbum for a club: %v", err)
	}
	if _, err = s.client.CreateAlbum(context.Background(), &postv1.CreateAlbumRequest{Name: "Лето"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateAlbum without a token: %v", err)
	}

	first, second := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	added, err := s.client.AddAlbumPhotos(s.as(t, ann), &postv1.AddAlbumPhotosRequest{AlbumId: id, Photos: []*postv1.NewAlbumPhoto{
		{PhotoId: first, Description: "Море"}, {PhotoId: second},
	}})
	if err != nil || len(added.GetPhotos()) != 2 || !added.GetPhotos()[0].GetMine() || added.GetPhotos()[0].GetDescription() != "Море" {
		t.Fatalf("AddAlbumPhotos = %v, %v", added, err)
	}
	if _, err = s.client.AddAlbumPhotos(s.as(t, bob), &postv1.AddAlbumPhotosRequest{AlbumId: id, Photos: []*postv1.NewAlbumPhoto{{PhotoId: uuid.NewString()}}}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AddAlbumPhotos by a stranger: %v", err)
	}
	if _, err = s.client.AddAlbumPhotos(s.as(t, ann), &postv1.AddAlbumPhotosRequest{AlbumId: id, Photos: []*postv1.NewAlbumPhoto{{PhotoId: first}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AddAlbumPhotos again: %v", err)
	}
	if _, err = s.client.AddAlbumPhotos(s.as(t, ann), &postv1.AddAlbumPhotosRequest{AlbumId: id}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AddAlbumPhotos without photos: %v", err)
	}

	// Public: the albums, an album, its photos and a photo.
	list, err := s.client.ListAlbums(context.Background(), &postv1.ListAlbumsRequest{OwnerId: ann.String()})
	if err != nil || list.GetTotal() != 1 || list.GetAlbums()[0].GetPhotoCount() != 2 || list.GetAlbums()[0].GetCover().GetId() != first {
		t.Fatalf("ListAlbums = %v, %v", list, err)
	}
	album, err := s.client.GetAlbum(context.Background(), &postv1.GetAlbumRequest{Id: id})
	if err != nil || album.GetAlbum().GetCover().GetId() != first || album.GetAlbum().GetCover().GetMine() {
		t.Fatalf("GetAlbum = %v, %v", album, err)
	}
	photos, err := s.client.ListAlbumPhotos(context.Background(), &postv1.ListAlbumPhotosRequest{AlbumId: id, Limit: 1})
	if err != nil || photos.GetTotal() != 2 || len(photos.GetPhotos()) != 1 || photos.GetNextCursor() != first {
		t.Fatalf("ListAlbumPhotos = %v, %v", photos, err)
	}
	photo, err := s.client.GetAlbumPhoto(context.Background(), &postv1.GetAlbumPhotoRequest{Id: second})
	if err != nil || photo.GetAlbum().GetName() != "Лето" || photo.GetPhoto().GetAuthorId() != ann.String() {
		t.Fatalf("GetAlbumPhoto = %v, %v", photo, err)
	}

	edited, err := s.client.EditAlbumPhoto(s.as(t, ann), &postv1.EditAlbumPhotoRequest{Id: second, Description: "Закат"})
	if err != nil || edited.GetPhoto().GetDescription() != "Закат" {
		t.Fatalf("EditAlbumPhoto = %v, %v", edited, err)
	}
	if _, err = s.client.EditAlbumPhoto(s.as(t, bob), &postv1.EditAlbumPhotoRequest{Id: second}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("EditAlbumPhoto by a stranger: %v", err)
	}
	if _, err = s.client.DeleteAlbumPhoto(s.as(t, bob), &postv1.DeleteAlbumPhotoRequest{Id: second}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DeleteAlbumPhoto by a stranger: %v", err)
	}
	if _, err = s.client.DeleteAlbumPhoto(s.as(t, ann), &postv1.DeleteAlbumPhotoRequest{Id: second}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetAlbumPhoto(context.Background(), &postv1.GetAlbumPhotoRequest{Id: second}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetAlbumPhoto after delete: %v", err)
	}

	renamed, err := s.client.EditAlbum(s.as(t, ann), &postv1.EditAlbumRequest{Id: id, Name: "Осень"})
	if err != nil || renamed.GetAlbum().GetName() != "Осень" || renamed.GetAlbum().GetDescription() != "" {
		t.Fatalf("EditAlbum = %v, %v", renamed, err)
	}
	if _, err = s.client.DeleteAlbum(s.as(t, bob), &postv1.DeleteAlbumRequest{Id: id}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DeleteAlbum by a stranger: %v", err)
	}
	if _, err = s.client.DeleteAlbum(s.as(t, ann), &postv1.DeleteAlbumRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetAlbum(context.Background(), &postv1.GetAlbumRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetAlbum after delete: %v", err)
	}
	if _, err = s.client.GetAlbumPhoto(context.Background(), &postv1.GetAlbumPhotoRequest{Id: first}); status.Code(err) != codes.NotFound {
		t.Fatalf("a photo of a deleted album: %v", err)
	}
}

func TestAvatarAlbum(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()
	photo := uuid.Must(uuid.NewV7())

	added, err := s.client.AddAvatarPhoto(s.as(t, ann), &postv1.AddAvatarPhotoRequest{PhotoId: photo.String()})
	if err != nil || added.GetPhoto().GetId() != photo.String() || added.GetPhoto().GetAuthorId() != ann.String() {
		t.Fatalf("AddAvatarPhoto = %v, %v", added, err)
	}

	got, err := s.client.GetAlbum(context.Background(), &postv1.GetAlbumRequest{Id: added.GetPhoto().GetAlbumId()})
	if a := got.GetAlbum(); err != nil || a.GetKind() != postv1.AlbumKind_ALBUM_KIND_AVATARS || a.GetName() != "Фотографии со страницы" || a.GetPhotoCount() != 1 {
		t.Fatalf("GetAlbum = %v, %v", got, err)
	}

	_, err = s.client.EditAlbum(s.as(t, ann), &postv1.EditAlbumRequest{Id: added.GetPhoto().GetAlbumId(), Name: "Другое"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("renaming it: %v", err)
	}

	for name, req := range map[string]*postv1.AddAvatarPhotoRequest{
		"a bad photo": {PhotoId: "x"},
		"a bad club":  {ClubId: "x", PhotoId: photo.String()},
	} {
		if _, err = s.client.AddAvatarPhoto(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err = s.client.AddAvatarPhoto(context.Background(), &postv1.AddAvatarPhotoRequest{PhotoId: photo.String()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
}

func TestArchiveSearchAndClosedComments(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	write := func(text string) string {
		t.Helper()

		created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: text})
		if err != nil {
			t.Fatal(err)
		}

		return created.GetPost().GetId()
	}
	sea, mountains := write("Еду на море"), write("горы")

	// "Поиск" of the wall.
	found, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String(), Query: "МОРЕ"})
	if err != nil || found.GetTotal() != 1 || found.GetPosts()[0].GetId() != sea {
		t.Fatalf("search = %v, %v", found, err)
	}
	if _, err = s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String(), Query: strings.Repeat("я", 101)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a long query: %v", err)
	}

	// "Архив записей": ann's only.
	if _, err = s.client.ArchivePost(s.as(t, bob), &postv1.ArchivePostRequest{Id: sea}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob archives: %v", err)
	}
	if _, err = s.client.ArchivePost(context.Background(), &postv1.ArchivePostRequest{Id: sea}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
	if _, err = s.client.ArchivePost(s.as(t, ann), &postv1.ArchivePostRequest{Id: sea}); err != nil {
		t.Fatalf("ArchivePost: %v", err)
	}

	wall, err := s.client.ListPosts(s.as(t, bob), &postv1.ListPostsRequest{WallId: ann.String()})
	if err != nil || wall.GetTotal() != 1 || wall.GetPosts()[0].GetId() != mountains {
		t.Fatalf("the wall = %v, %v", wall, err)
	}
	if _, err = s.client.GetPost(s.as(t, bob), &postv1.GetPostRequest{Id: sea}); status.Code(err) != codes.NotFound {
		t.Fatalf("bob gets the archived post: %v", err)
	}
	archive := &postv1.ListPostsRequest{WallId: ann.String(), Filter: postv1.WallFilter_WALL_FILTER_ARCHIVE}
	if _, err = s.client.ListPosts(s.as(t, bob), archive); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob reads the archive: %v", err)
	}
	if _, err = s.client.ListPosts(context.Background(), archive); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("an anonymous viewer reads the archive: %v", err)
	}
	own, err := s.client.ListPosts(s.as(t, ann), archive)
	if err != nil || own.GetTotal() != 1 || own.GetPosts()[0].GetId() != sea || !own.GetPosts()[0].GetArchived() {
		t.Fatalf("the archive = %v, %v", own, err)
	}
	if _, err = s.client.UnarchivePost(s.as(t, ann), &postv1.UnarchivePostRequest{Id: sea}); err != nil {
		t.Fatalf("UnarchivePost: %v", err)
	}
	if got, err := s.client.GetPost(s.as(t, bob), &postv1.GetPostRequest{Id: sea}); err != nil || got.GetPost().GetArchived() {
		t.Fatalf("the returned post = %v, %v", got, err)
	}

	// Closed comments.
	if _, err = s.client.CloseComments(s.as(t, bob), &postv1.CloseCommentsRequest{PostId: sea}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob closes the comments: %v", err)
	}
	if _, err = s.client.CloseComments(s.as(t, ann), &postv1.CloseCommentsRequest{PostId: sea}); err != nil {
		t.Fatalf("CloseComments: %v", err)
	}
	if got, _ := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: sea}); !got.GetPost().GetCommentsClosed() {
		t.Fatal("the post does not say its comments are closed")
	}
	if _, err = s.client.CreateComment(s.as(t, bob), &postv1.CreateCommentRequest{PostId: sea, Text: "ау"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a comment on a closed post: %v", err)
	}
	if _, err = s.client.OpenComments(s.as(t, ann), &postv1.OpenCommentsRequest{PostId: sea}); err != nil {
		t.Fatalf("OpenComments: %v", err)
	}
	if _, err = s.client.CreateComment(s.as(t, bob), &postv1.CreateCommentRequest{PostId: sea, Text: "ау"}); err != nil {
		t.Fatalf("a comment after they are opened: %v", err)
	}
	if _, err = s.client.CloseComments(s.as(t, ann), &postv1.CloseCommentsRequest{PostId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad id: %v", err)
	}
}

func TestAlbumCoverMoveAndWall(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	album := func(name string) string {
		t.Helper()

		created, err := s.client.CreateAlbum(s.as(t, ann), &postv1.CreateAlbumRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}

		return created.GetAlbum().GetId()
	}
	summer, winter := album("Лето"), album("Зима")

	first, second := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	if _, err := s.client.AddAlbumPhotos(s.as(t, ann), &postv1.AddAlbumPhotosRequest{
		AlbumId: summer, Photos: []*postv1.NewAlbumPhoto{{PhotoId: first}, {PhotoId: second}},
	}); err != nil {
		t.Fatal(err)
	}

	// "Обложка альбома": the chosen photo instead of the first.
	if _, err := s.client.SetAlbumCover(s.as(t, ann), &postv1.SetAlbumCoverRequest{AlbumId: summer, PhotoId: second}); err != nil {
		t.Fatalf("SetAlbumCover: %v", err)
	}
	if got, err := s.client.GetAlbum(context.Background(), &postv1.GetAlbumRequest{Id: summer}); err != nil || got.GetAlbum().GetCover().GetId() != second {
		t.Fatalf("the cover = %v, %v", got.GetAlbum().GetCover(), err)
	}
	for name, tt := range map[string]struct {
		ctx  context.Context
		req  *postv1.SetAlbumCoverRequest
		code codes.Code
	}{
		"without a token":          {context.Background(), &postv1.SetAlbumCoverRequest{AlbumId: summer, PhotoId: first}, codes.Unauthenticated},
		"a stranger":               {s.as(t, bob), &postv1.SetAlbumCoverRequest{AlbumId: summer, PhotoId: first}, codes.PermissionDenied},
		"a photo of another album": {s.as(t, ann), &postv1.SetAlbumCoverRequest{AlbumId: winter, PhotoId: first}, codes.NotFound},
		"a bad photo":              {s.as(t, ann), &postv1.SetAlbumCoverRequest{AlbumId: summer, PhotoId: "x"}, codes.InvalidArgument},
		"no album":                 {s.as(t, ann), &postv1.SetAlbumCoverRequest{AlbumId: uuid.NewString(), PhotoId: first}, codes.NotFound},
	} {
		if _, err := s.client.SetAlbumCover(tt.ctx, tt.req); status.Code(err) != tt.code {
			t.Errorf("SetAlbumCover, %s: %v, want %v", name, err, tt.code)
		}
	}

	// The photo moves to another album of ann's.
	if _, err := s.client.MoveAlbumPhoto(s.as(t, bob), &postv1.MoveAlbumPhotoRequest{PhotoId: second, AlbumId: winter}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("a stranger moves it: %v", err)
	}
	if _, err := s.client.MoveAlbumPhoto(s.as(t, ann), &postv1.MoveAlbumPhotoRequest{PhotoId: second, AlbumId: summer}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("to the album it is in: %v", err)
	}
	if _, err := s.client.MoveAlbumPhoto(s.as(t, ann), &postv1.MoveAlbumPhotoRequest{PhotoId: second, AlbumId: winter}); err != nil {
		t.Fatalf("MoveAlbumPhoto: %v", err)
	}
	if got, err := s.client.GetAlbumPhoto(context.Background(), &postv1.GetAlbumPhotoRequest{Id: second}); err != nil || got.GetPhoto().GetAlbumId() != winter {
		t.Fatalf("the moved photo = %v, %v", got, err)
	}
	if got, _ := s.client.GetAlbum(context.Background(), &postv1.GetAlbumRequest{Id: summer}); got.GetAlbum().GetPhotoCount() != 1 || got.GetAlbum().GetCover().GetId() != first {
		t.Fatalf("the album it left = %v", got.GetAlbum())
	}

	// "Фотографии со стены": the photos of a post on one's own wall.
	if _, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "море", PhotoIds: []string{uuid.NewString()}}); err != nil {
		t.Fatal(err)
	}
	list, err := s.client.ListAlbums(context.Background(), &postv1.ListAlbumsRequest{OwnerId: ann.String()})
	if err != nil || list.GetTotal() != 3 {
		t.Fatalf("ListAlbums = %v, %v", list, err)
	}
	wall := list.GetAlbums()[0]
	if wall.GetKind() != postv1.AlbumKind_ALBUM_KIND_WALL || wall.GetName() != "Фотографии со стены" || wall.GetPhotoCount() != 1 {
		t.Fatalf("the wall album = %v", wall)
	}
	if _, err = s.client.MoveAlbumPhoto(s.as(t, ann), &postv1.MoveAlbumPhotoRequest{PhotoId: first, AlbumId: wall.GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("into the album the site keeps: %v", err)
	}
}

func TestPhotoReactions(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	created, err := s.client.CreateAlbum(s.as(t, ann), &postv1.CreateAlbumRequest{Name: "Лето"})
	if err != nil {
		t.Fatal(err)
	}
	photo := uuid.Must(uuid.NewV7()).String()
	if _, err = s.client.AddAlbumPhotos(s.as(t, ann), &postv1.AddAlbumPhotosRequest{
		AlbumId: created.GetAlbum().GetId(), Photos: []*postv1.NewAlbumPhoto{{PhotoId: photo}},
	}); err != nil {
		t.Fatal(err)
	}

	// The heart under the photo, and who like it.
	liked, err := s.client.LikeAlbumPhoto(s.as(t, bob), &postv1.LikeAlbumPhotoRequest{PhotoId: photo})
	if err != nil || liked.GetLikeCount() != 1 {
		t.Fatalf("LikeAlbumPhoto = %v, %v", liked, err)
	}
	got, err := s.client.GetAlbumPhoto(s.as(t, bob), &postv1.GetAlbumPhotoRequest{Id: photo})
	if err != nil || !got.GetPhoto().GetLiked() || got.GetPhoto().GetLikeCount() != 1 {
		t.Fatalf("GetAlbumPhoto = %v, %v", got, err)
	}
	likers, err := s.client.ListAlbumPhotoLikers(context.Background(), &postv1.ListAlbumPhotoLikersRequest{PhotoId: photo})
	if err != nil || likers.GetTotal() != 1 || likers.GetUserIds()[0] != bob.String() {
		t.Fatalf("ListAlbumPhotoLikers = %v, %v", likers, err)
	}
	if _, err = s.client.LikeAlbumPhoto(s.as(t, bob), &postv1.LikeAlbumPhotoRequest{PhotoId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("liking no photo: %v", err)
	}

	// Comments: bob's, which ann, who uploaded the photo, may delete.
	comment, err := s.client.CreatePhotoComment(s.as(t, bob), &postv1.CreatePhotoCommentRequest{PhotoId: photo, Text: " красиво "})
	if err != nil || comment.GetComment().GetText() != "красиво" || !comment.GetComment().GetMine() {
		t.Fatalf("CreatePhotoComment = %v, %v", comment, err)
	}
	if _, err = s.client.CreatePhotoComment(s.as(t, bob), &postv1.CreatePhotoCommentRequest{PhotoId: photo, Text: " "}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an empty comment: %v", err)
	}
	comments, err := s.client.ListPhotoComments(context.Background(), &postv1.ListPhotoCommentsRequest{PhotoId: photo})
	if err != nil || comments.GetTotal() != 1 || len(comments.GetComments()) != 1 {
		t.Fatalf("ListPhotoComments = %v, %v", comments, err)
	}
	stranger := uuid.New()
	if _, err = s.client.DeletePhotoComment(s.as(t, stranger), &postv1.DeletePhotoCommentRequest{Id: comment.GetComment().GetId()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("a stranger deletes: %v", err)
	}
	if _, err = s.client.DeletePhotoComment(s.as(t, ann), &postv1.DeletePhotoCommentRequest{Id: comment.GetComment().GetId()}); err != nil {
		t.Fatalf("who uploaded the photo deletes: %v", err)
	}

	// "Поделиться": a post of bob's sharing it.
	shared, err := s.client.RepostPost(s.as(t, bob), &postv1.RepostPostRequest{PhotoId: photo, Text: "смотрите"})
	if err != nil || shared.GetPost().GetSharedPhotoId() != photo || shared.GetPost().GetText() != "смотрите" || shared.GetPost().GetWallId() != bob.String() {
		t.Fatalf("sharing a photo = %v, %v", shared, err)
	}
	if _, err = s.client.RepostPost(s.as(t, bob), &postv1.RepostPostRequest{PhotoId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("sharing no photo: %v", err)
	}
}

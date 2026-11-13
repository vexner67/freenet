package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"slices"
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
	documents := inmem.NewDocumentRepository()
	music := inmem.NewMusic()
	audio := service.NewAudioService(music.Audios, music.Lists, music.Profiles, inmem.TxManager{}, mediaclient.Nop{})
	posts := service.NewPostService(service.PostDeps{Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{}, Switches: store.Posts(), Polls: store.Polls(), Walls: store.Sources()})
	postponed := inmem.NewPostponedRepository()
	postv1.RegisterPostServiceServer(srv, postgrpc.NewPostHandler(postgrpc.Services{
		Posts: posts,
		Postponed: service.NewPostponedService(service.PostponedDeps{
			Postponed: postponed, Poster: posts, Tx: inmem.TxManager{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{},
		}),
		Comments: service.NewCommentService(service.CommentDeps{Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{}, Walls: store.Sources()}),
		Suggestions: service.NewSuggestionService(service.SuggestionDeps{
			Suggestions: suggestions, Posts: store.Posts(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{},
		}),
		Topics: service.NewTopicService(service.TopicDeps{
			Topics: topics, Messages: topics.Messages(), Likes: topics.Messages(), Tx: inmem.TxManager{},
			Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{}, Moderators: clubclient.Nop{},
		}),
		Polls: service.NewPollService(service.PollDeps{Polls: store.Polls(), Posts: store.Posts(), Moderators: clubclient.Nop{}, Tx: inmem.TxManager{}, Walls: store.Sources()}),
		Faves: service.NewFaveService(service.FaveDeps{
			PostLikes: store.Likes(), CommentLikes: store.CommentLikes(),
			Posts: store.Posts(), Comments: store.Comments(), Walls: store.Sources(),
		}),
		Documents: service.NewDocumentService(documents, inmem.TxManager{}, mediaclient.Nop{}),
		Audio:     audio,
		Playlists: service.NewPlaylistService(music.Playlists, music.Lists, audio),
		Articles: service.NewArticleService(service.ArticleDeps{
			Articles: inmem.NewArticleRepository(), Tx: inmem.TxManager{}, Photos: mediaclient.Nop{}, Posts: store.Posts(), Eraser: posts,
		}),
		Stories: service.NewStoryService(service.StoryDeps{
			Stories: store.Stories(), Views: store.StoryViews(), Likes: store.StoryLikes(), Comments: store.StoryComments(),
			Tx: inmem.TxManager{}, Photos: mediaclient.Nop{}, Notifier: notifyclient.Nop{},
		}),
		Purge: service.NewPurgeService(service.PurgeDeps{
			Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Votes: store.Polls(), Comments: store.Comments(), Suggestions: suggestions,
			Topics: topics, Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Photos: mediaclient.Nop{},
			Held: []service.ReactionHolder{store.Holds(), topics}, Sources: store.Sources(),
			Postponed: postponed, Documents: documents, Audio: music.Audios,
		}),
		ClubPurge: service.NewClubPurgeService(service.ClubPurgeDeps{
			Posts: store.Posts(), Comments: store.Comments(), Suggestions: suggestions, Postponed: postponed,
			Topics: topics, Sources: store.Sources(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Media: mediaclient.Nop{},
		}),
		Holds:   service.NewHoldService(inmem.TxManager{}, store.Holds(), topics),
		Sources: service.NewSourceService(store.Sources()),
		Games:   service.NewGameService(inmem.NewGameRepository(), documents),
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

// reads tells the server, as the user and club services do, that the
// reader reads people and, after them, clubs.
func (s testServer) reads(t *testing.T, reader uuid.UUID, people []uuid.UUID, clubs ...uuid.UUID) context.Context {
	t.Helper()

	for i, source := range append(people, clubs...) {
		req := &postv1.FollowSourceRequest{ReaderId: reader.String(), SourceId: source.String(), Club: i >= len(people)}
		if _, err := s.client.FollowSource(context.Background(), req); err != nil {
			t.Fatalf("FollowSource: %v", err)
		}
	}

	return s.as(t, reader)
}

func TestFeedSources(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	if _, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "анна"}); err != nil {
		t.Fatal(err)
	}

	feed := func(ctx context.Context) int {
		t.Helper()

		page, err := s.client.ListFeed(ctx, &postv1.ListFeedRequest{})
		if err != nil {
			t.Fatalf("ListFeed: %v", err)
		}

		return len(page.GetPosts())
	}

	// Without a token there is no reader and no news.
	if got := feed(context.Background()); got != 0 {
		t.Fatalf("the feed of nobody shows %d posts", got)
	}
	if got := feed(s.as(t, bob)); got != 0 {
		t.Fatalf("bob reads %d posts before following", got)
	}
	if got := feed(s.reads(t, bob, []uuid.UUID{ann})); got != 1 {
		t.Fatalf("bob reads %d posts, want ann's", got)
	}

	unfollow := &postv1.UnfollowSourceRequest{ReaderId: bob.String(), SourceId: ann.String()}
	for range 2 {
		if _, err := s.client.UnfollowSource(context.Background(), unfollow); err != nil {
			t.Fatalf("UnfollowSource: %v", err)
		}
	}
	if got := feed(s.as(t, bob)); got != 0 {
		t.Fatalf("bob reads %d posts after unfollowing", got)
	}

	// The whole of what bob follows, told anew, brings the lost follow
	// back, and takes it away once it says nobody.
	stretch := &postv1.SyncSourcesRequest{OwnerId: bob.String(), Ids: []string{ann.String()}, Last: true}
	if _, err := s.client.SyncSources(context.Background(), stretch); err != nil {
		t.Fatalf("SyncSources: %v", err)
	}
	if got := feed(s.as(t, bob)); got != 1 {
		t.Fatalf("bob reads %d posts after the stretch, want ann's", got)
	}
	if _, err := s.client.SyncSources(context.Background(), &postv1.SyncSourcesRequest{OwnerId: bob.String(), Last: true}); err != nil {
		t.Fatalf("SyncSources: %v", err)
	}
	if got := feed(s.as(t, bob)); got != 0 {
		t.Fatalf("bob reads %d posts after a stretch of nobody", got)
	}

	tests := []struct {
		name string
		call func() error
	}{
		{"a bad reader", func() error {
			_, err := s.client.FollowSource(context.Background(), &postv1.FollowSourceRequest{ReaderId: "x", SourceId: ann.String()})
			return err
		}},
		{"a bad source", func() error {
			_, err := s.client.FollowSource(context.Background(), &postv1.FollowSourceRequest{ReaderId: bob.String(), SourceId: "x"})
			return err
		}},
		{"oneself", func() error {
			_, err := s.client.FollowSource(context.Background(), &postv1.FollowSourceRequest{ReaderId: bob.String(), SourceId: bob.String()})
			return err
		}},
		{"a stretch of a bad owner", func() error {
			_, err := s.client.SyncSources(context.Background(), &postv1.SyncSourcesRequest{OwnerId: "x", Last: true})
			return err
		}},
		{"a stretch from a bad start", func() error {
			_, err := s.client.SyncSources(context.Background(), &postv1.SyncSourcesRequest{OwnerId: bob.String(), AfterId: "x", Last: true})
			return err
		}},
		{"a stretch with a bad id", func() error {
			_, err := s.client.SyncSources(context.Background(), &postv1.SyncSourcesRequest{OwnerId: bob.String(), Ids: []string{"x"}, Last: true})
			return err
		}},
		{"a stretch with the owner in it", func() error {
			_, err := s.client.SyncSources(context.Background(), &postv1.SyncSourcesRequest{OwnerId: bob.String(), Ids: []string{bob.String()}, Last: true})
			return err
		}},
		{"unfollowing a bad source", func() error {
			_, err := s.client.UnfollowSource(context.Background(), &postv1.UnfollowSourceRequest{ReaderId: bob.String(), SourceId: "x"})
			return err
		}},
	}

	for _, tt := range tests {
		if err := tt.call(); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v, want InvalidArgument", tt.name, err)
		}
	}
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

	feed, err := s.client.ListFeed(s.reads(t, uuid.New(), []uuid.UUID{ann, bob}), &postv1.ListFeedRequest{Limit: 2})
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
		{"bad author id", func() error {
			_, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: "ann"})
			return err
		}, codes.InvalidArgument},
		{"bad cursor", func() error {
			_, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: uuid.NewString(), Cursor: "x"})
			return err
		}, codes.InvalidArgument},
		{"too many ignored walls", func() error {
			ids := make([]string, 51)
			for i := range ids {
				ids[i] = uuid.NewString()
			}
			_, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{Everyone: true, IgnoredWallIds: ids})
			return err
		}, codes.InvalidArgument},
		{"a bad ignored wall", func() error {
			_, err := s.client.ListFeed(context.Background(), &postv1.ListFeedRequest{Everyone: true, IgnoredWallIds: []string{"x"}})
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

	feed, err := s.client.ListFeed(s.reads(t, uuid.New(), nil, club), &postv1.ListFeedRequest{})
	if err != nil || len(feed.GetPosts()) != 1 || !feed.GetPosts()[0].GetFromWall() {
		t.Fatalf("ListFeed of the club = %v, %v", feed, err)
	}

	byAuthor, err := s.client.ListFeed(s.reads(t, uuid.New(), []uuid.UUID{admin}), &postv1.ListFeedRequest{})
	if err != nil || len(byAuthor.GetPosts()) != 0 {
		t.Fatalf("ListFeed of the admin = %v, %v; the post is the club's", byAuthor, err)
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

	// What bob does not see he neither likes nor shares.
	if _, err = s.client.LikePost(s.as(t, bob), &postv1.LikePostRequest{PostId: sea}); status.Code(err) != codes.NotFound {
		t.Fatalf("bob likes the archived post: %v", err)
	}
	if _, err = s.client.RepostPost(s.as(t, bob), &postv1.RepostPostRequest{PostId: sea}); status.Code(err) != codes.NotFound {
		t.Fatalf("bob shares the archived post: %v", err)
	}

	// The years of the archive and the whole wall put away at once.
	year := int32(time.Now().UTC().Year())
	years, err := s.client.ListWallYears(s.as(t, ann), &postv1.ListWallYearsRequest{WallId: ann.String()})
	if err != nil || !slices.Equal(years.GetYears(), []int32{year}) {
		t.Fatalf("ListWallYears = %v, %v", years, err)
	}
	if _, err = s.client.ListWallYears(s.as(t, bob), &postv1.ListWallYearsRequest{WallId: ann.String()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob reads the years: %v", err)
	}
	if _, err = s.client.ListWallYears(context.Background(), &postv1.ListWallYearsRequest{WallId: ann.String()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("the years without a token: %v", err)
	}

	archive.Year = year - 1
	if none, err := s.client.ListPosts(s.as(t, ann), archive); err != nil || none.GetTotal() != 0 {
		t.Fatalf("the archive of last year = %v, %v", none, err)
	}
	archive.Year = 1
	if _, err = s.client.ListPosts(s.as(t, ann), archive); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("the archive of year 1: %v", err)
	}

	all := &postv1.ArchiveWallRequest{WallId: ann.String(), Year: year, Archived: true}
	if _, err = s.client.ArchiveWall(s.as(t, bob), all); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob archives the wall: %v", err)
	}
	if _, err = s.client.ArchiveWall(context.Background(), all); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("archiving without a token: %v", err)
	}
	if _, err = s.client.ArchiveWall(s.as(t, ann), &postv1.ArchiveWallRequest{WallId: ann.String(), Year: 1, Archived: true}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("archiving year 1: %v", err)
	}
	if done, err := s.client.ArchiveWall(s.as(t, ann), all); err != nil || done.GetChanged() != 1 {
		t.Fatalf("ArchiveWall = %v, %v", done, err)
	}
	all.Archived, all.Year = false, 0
	if done, err := s.client.ArchiveWall(s.as(t, ann), all); err != nil || done.GetChanged() != 2 {
		t.Fatalf("restoring the wall = %v, %v", done, err)
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

func TestNSFW(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "для взрослых", Nsfw: true})
	if err != nil || !created.GetPost().GetNsfw() {
		t.Fatalf("CreatePost = %v, %v", created, err)
	}
	id := created.GetPost().GetId()

	if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "котики"}); err != nil {
		t.Fatal(err)
	}

	feed, err := s.client.ListFeed(s.as(t, ann), &postv1.ListFeedRequest{})
	if err != nil || len(feed.GetPosts()) != 2 || !feed.GetPosts()[1].GetNsfw() {
		t.Fatalf("ListFeed = %v, %v", feed, err)
	}
	feed, err = s.client.ListFeed(s.as(t, ann), &postv1.ListFeedRequest{HideNsfw: true})
	if err != nil || len(feed.GetPosts()) != 1 || feed.GetPosts()[0].GetText() != "котики" {
		t.Fatalf("ListFeed without adult posts = %v, %v", feed, err)
	}

	// An edit keeps the mark unless it names it.
	edited, err := s.client.EditPost(s.as(t, ann), &postv1.EditPostRequest{Id: id, Text: "всё ещё"})
	if err != nil || !edited.GetPost().GetNsfw() {
		t.Fatalf("EditPost = %v, %v", edited, err)
	}

	off := false
	if edited, err = s.client.EditPost(s.as(t, ann), &postv1.EditPostRequest{Id: id, Text: "для всех", Nsfw: &off}); err != nil || edited.GetPost().GetNsfw() {
		t.Fatalf("EditPost taking the mark off = %v, %v", edited, err)
	}
}

// Lists ask for the beginnings of texts; a post alone is whole.
func TestTextLimit(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "длинная запись"})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	id := created.GetPost().GetId()

	if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "да"}); err != nil {
		t.Fatal(err)
	}

	comment, err := s.client.CreateComment(s.as(t, ann), &postv1.CreateCommentRequest{PostId: id, Text: "длинный ответ"})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	wall, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String(), TextLimit: 7})
	if err != nil || len(wall.GetPosts()) != 2 {
		t.Fatalf("ListPosts = %v, %v", wall, err)
	}
	if short, long := wall.GetPosts()[0], wall.GetPosts()[1]; short.GetText() != "да" || short.GetTextCut() ||
		long.GetText() != "длинная" || !long.GetTextCut() {
		t.Fatalf("wall with a limit = %v", wall.GetPosts())
	}

	whole, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String()})
	if err != nil || whole.GetPosts()[1].GetText() != "длинная запись" || whole.GetPosts()[1].GetTextCut() {
		t.Fatalf("wall without a limit = %v, %v", whole, err)
	}

	feed, err := s.client.ListFeed(s.as(t, ann), &postv1.ListFeedRequest{TextLimit: 7})
	if err != nil || len(feed.GetPosts()) != 2 || feed.GetPosts()[1].GetText() != "длинная" || !feed.GetPosts()[1].GetTextCut() {
		t.Fatalf("ListFeed = %v, %v", feed, err)
	}

	many, err := s.client.GetPosts(context.Background(), &postv1.GetPostsRequest{Ids: []string{id}, TextLimit: 7})
	if err != nil || len(many.GetPosts()) != 1 || many.GetPosts()[0].GetText() != "длинная" || !many.GetPosts()[0].GetTextCut() {
		t.Fatalf("GetPosts = %v, %v", many, err)
	}

	one, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: id})
	if err != nil || one.GetPost().GetText() != "длинная запись" || one.GetPost().GetTextCut() {
		t.Fatalf("GetPost = %v, %v", one, err)
	}

	comments, err := s.client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: id, TextLimit: 7})
	if err != nil || len(comments.GetComments()) != 1 || comments.GetComments()[0].GetText() != "длинный" || !comments.GetComments()[0].GetTextCut() {
		t.Fatalf("ListComments = %v, %v", comments, err)
	}

	last, err := s.client.ListLastComments(context.Background(), &postv1.ListLastCommentsRequest{PostIds: []string{id}, TextLimit: 7})
	if err != nil || len(last.GetComments()) != 1 || last.GetComments()[0].GetText() != "длинный" || !last.GetComments()[0].GetTextCut() {
		t.Fatalf("ListLastComments = %v, %v", last, err)
	}

	byID := &postv1.GetCommentsRequest{Ids: []string{comment.GetComment().GetId()}, TextLimit: 7}
	if got, err := s.client.GetComments(context.Background(), byID); err != nil || len(got.GetComments()) != 1 ||
		got.GetComments()[0].GetText() != "длинный" || !got.GetComments()[0].GetTextCut() {
		t.Fatalf("GetComments = %v, %v", got, err)
	}

	byID.TextLimit = 0
	if got, err := s.client.GetComments(context.Background(), byID); err != nil || got.GetComments()[0].GetText() != "длинный ответ" || got.GetComments()[0].GetTextCut() {
		t.Fatalf("GetComments whole = %v, %v", got, err)
	}
}

// The lists of topics, their messages, suggestions and photo comments ask
// for the beginnings of texts; one of them alone is whole.
func TestTextLimit_OtherLists(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	club := uuid.New()

	topic, err := s.client.CreateTopic(s.as(t, ann), &postv1.CreateTopicRequest{ClubId: club.String(), Title: "Тема", Text: "длинное начало"})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}

	messages, err := s.client.ListTopicMessages(context.Background(), &postv1.ListTopicMessagesRequest{TopicId: topic.GetTopic().GetId(), TextLimit: 7})
	if err != nil || len(messages.GetMessages()) != 1 || messages.GetMessages()[0].GetText() != "длинное" || !messages.GetMessages()[0].GetTextCut() {
		t.Fatalf("ListTopicMessages = %v, %v", messages, err)
	}

	topics, err := s.client.ListTopics(context.Background(), &postv1.ListTopicsRequest{ClubId: club.String(), TextLimit: 7})
	if err != nil || len(topics.GetTopics()) != 1 || topics.GetTopics()[0].GetLastMessage().GetText() != "длинное" || !topics.GetTopics()[0].GetLastMessage().GetTextCut() {
		t.Fatalf("ListTopics = %v, %v", topics, err)
	}

	message, err := s.client.GetTopicMessage(context.Background(), &postv1.GetTopicMessageRequest{Id: messages.GetMessages()[0].GetId()})
	if err != nil || message.GetMessage().GetText() != "длинное начало" || message.GetMessage().GetTextCut() {
		t.Fatalf("GetTopicMessage = %v, %v", message, err)
	}
	if _, err = s.client.GetTopicMessage(context.Background(), &postv1.GetTopicMessageRequest{Id: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("a missing message: %v", err)
	}

	suggested, err := s.client.SuggestPost(s.as(t, ann), &postv1.SuggestPostRequest{WallId: club.String(), Text: "длинная новость"})
	if err != nil {
		t.Fatalf("SuggestPost: %v", err)
	}

	mine, err := s.client.ListSuggestions(s.as(t, ann), &postv1.ListSuggestionsRequest{WallId: club.String(), Mine: true, TextLimit: 7})
	if err != nil || len(mine.GetSuggestions()) != 1 || mine.GetSuggestions()[0].GetText() != "длинная" || !mine.GetSuggestions()[0].GetTextCut() {
		t.Fatalf("ListSuggestions = %v, %v", mine, err)
	}

	id := suggested.GetSuggestion().GetId()
	if got, err := s.client.GetSuggestion(s.as(t, ann), &postv1.GetSuggestionRequest{Id: id}); err != nil || got.GetSuggestion().GetText() != "длинная новость" {
		t.Fatalf("GetSuggestion = %v, %v", got, err)
	}
	// Somebody else is not told it exists.
	if _, err = s.client.GetSuggestion(s.as(t, bob), &postv1.GetSuggestionRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("a stranger's GetSuggestion: %v", err)
	}
	if _, err = s.client.GetSuggestion(context.Background(), &postv1.GetSuggestionRequest{Id: id}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetSuggestion without a token: %v", err)
	}
}

func TestTopicMessageLikersAndQuotes(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	club := uuid.New()

	topic, err := s.client.CreateTopic(s.as(t, ann), &postv1.CreateTopicRequest{ClubId: club.String(), Title: "Тема", Text: "длинное начало"})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}

	messages, err := s.client.ListTopicMessages(context.Background(), &postv1.ListTopicMessagesRequest{TopicId: topic.GetTopic().GetId()})
	if err != nil || len(messages.GetMessages()) != 1 {
		t.Fatalf("ListTopicMessages = %v, %v", messages, err)
	}
	id := messages.GetMessages()[0].GetId()

	if _, err = s.client.LikeTopicMessage(s.as(t, bob), &postv1.LikeTopicMessageRequest{Id: id}); err != nil {
		t.Fatalf("LikeTopicMessage: %v", err)
	}

	likers, err := s.client.ListTopicMessageLikers(context.Background(), &postv1.ListTopicMessageLikersRequest{Id: id})
	if err != nil || likers.GetTotal() != 1 || len(likers.GetUserIds()) != 1 || likers.GetUserIds()[0] != bob.String() {
		t.Fatalf("ListTopicMessageLikers = %v, %v", likers, err)
	}
	if _, err = s.client.ListTopicMessageLikers(context.Background(), &postv1.ListTopicMessageLikersRequest{Id: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("likers of nothing: %v", err)
	}
	if _, err = s.client.ListTopicMessageLikers(context.Background(), &postv1.ListTopicMessageLikersRequest{Id: id, Cursor: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad cursor: %v", err)
	}

	quoted, err := s.client.GetTopicMessages(context.Background(), &postv1.GetTopicMessagesRequest{Ids: []string{id, uuid.NewString()}, TextLimit: 7})
	if err != nil || len(quoted.GetMessages()) != 1 || quoted.GetMessages()[0].GetText() != "длинное" || !quoted.GetMessages()[0].GetTextCut() {
		t.Fatalf("GetTopicMessages = %v, %v", quoted, err)
	}
}

func TestPolls(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	anyone := context.Background()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{
		Poll: &postv1.PollDraft{Title: " Кошки или  собаки? ", Options: []string{"кошки", "собаки"}, Days: 2},
	})
	if err != nil {
		t.Fatalf("CreatePost with a poll and no words: %v", err)
	}

	id := created.GetPost().GetId()
	if !created.GetPost().GetPolled() {
		t.Fatal("the post does not say it asks a poll")
	}

	got, err := s.client.GetPolls(anyone, &postv1.GetPollsRequest{PostIds: []string{id, uuid.NewString()}})
	if err != nil || len(got.GetPolls()) != 1 {
		t.Fatalf("GetPolls without a token = %v, %v", got, err)
	}

	poll := got.GetPolls()[0]
	if poll.GetPostId() != id || poll.GetTitle() != "Кошки или собаки?" || len(poll.GetOptions()) != 2 ||
		poll.GetOptions()[1].GetName() != "собаки" || poll.GetEndsAt() == nil || poll.GetEnded() || len(poll.GetChosen()) != 0 {
		t.Fatalf("poll = %v", poll)
	}

	if _, err = s.client.VotePoll(anyone, &postv1.VotePollRequest{PostId: id, Options: []int32{0}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("a vote without a token: got %v", err)
	}

	voted, err := s.client.VotePoll(s.as(t, bob), &postv1.VotePollRequest{PostId: id, Options: []int32{1}})
	if err != nil {
		t.Fatalf("VotePoll: %v", err)
	}
	if p := voted.GetPoll(); p.GetVoterCount() != 1 || p.GetOptions()[1].GetVoteCount() != 1 || len(p.GetChosen()) != 1 || p.GetChosen()[0] != 1 {
		t.Fatalf("after the vote: %v", p)
	}

	for name, tt := range map[string]struct {
		req  *postv1.VotePollRequest
		code codes.Code
	}{
		"twice":           {&postv1.VotePollRequest{PostId: id, Options: []int32{0}}, codes.FailedPrecondition},
		"no post":         {&postv1.VotePollRequest{PostId: uuid.NewString(), Options: []int32{0}}, codes.NotFound},
		"a malformed id":  {&postv1.VotePollRequest{PostId: "x", Options: []int32{0}}, codes.InvalidArgument},
		"nothing chosen":  {&postv1.VotePollRequest{PostId: id}, codes.InvalidArgument},
		"no such option":  {&postv1.VotePollRequest{PostId: id, Options: []int32{5}}, codes.InvalidArgument},
		"several options": {&postv1.VotePollRequest{PostId: id, Options: []int32{0, 1}}, codes.InvalidArgument},
	} {
		// Ann has not voted, so only "twice" is Bob's.
		ctx := s.as(t, ann)
		if name == "twice" {
			ctx = s.as(t, bob)
		}

		if _, err = s.client.VotePoll(ctx, tt.req); status.Code(err) != tt.code {
			t.Fatalf("%s: got %v, want %v", name, err, tt.code)
		}
	}

	// Bob sees his choice in the poll of the page.
	got, err = s.client.GetPolls(s.as(t, bob), &postv1.GetPollsRequest{PostIds: []string{id}})
	if err != nil || len(got.GetPolls()) != 1 || len(got.GetPolls()[0].GetChosen()) != 1 {
		t.Fatalf("GetPolls for a voter = %v, %v", got, err)
	}

	voters, err := s.client.ListPollVoters(anyone, &postv1.ListPollVotersRequest{PostId: id, Option: 1})
	if err != nil || voters.GetTotal() != 1 || len(voters.GetUserIds()) != 1 || voters.GetUserIds()[0] != bob.String() {
		t.Fatalf("ListPollVoters = %v, %v", voters, err)
	}
	if _, err = s.client.ListPollVoters(anyone, &postv1.ListPollVotersRequest{PostId: id, Option: 9}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("voters of no option: got %v", err)
	}

	back, err := s.client.RetractPollVote(s.as(t, bob), &postv1.RetractPollVoteRequest{PostId: id})
	if err != nil || back.GetPoll().GetVoterCount() != 0 || len(back.GetPoll().GetChosen()) != 0 {
		t.Fatalf("RetractPollVote = %v, %v", back, err)
	}
	if _, err = s.client.RetractPollVote(s.as(t, bob), &postv1.RetractPollVoteRequest{PostId: id}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("retract twice: got %v", err)
	}

	// The poll stays through an edit, and says what an edit cannot change.
	edited, err := s.client.EditPost(s.as(t, ann), &postv1.EditPostRequest{Id: id, Text: "А вы как думаете?"})
	if err != nil || !edited.GetPost().GetPolled() {
		t.Fatalf("EditPost = %v, %v", edited, err)
	}

	locked, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{
		Text: "тайно",
		Poll: &postv1.PollDraft{Title: "Да?", Options: []string{"да"}, Anonymous: true, Locked: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	lockedID := locked.GetPost().GetId()
	if _, err = s.client.VotePoll(s.as(t, bob), &postv1.VotePollRequest{PostId: lockedID, Options: []int32{0}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RetractPollVote(s.as(t, bob), &postv1.RetractPollVoteRequest{PostId: lockedID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("retract in a locked poll: got %v", err)
	}
	if _, err = s.client.ListPollVoters(anyone, &postv1.ListPollVotersRequest{PostId: lockedID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("voters of an anonymous poll: got %v", err)
	}

	for name, d := range map[string]*postv1.PollDraft{
		"no title":         {Options: []string{"да"}},
		"no options":       {Title: "Да?"},
		"an empty option":  {Title: "Да?", Options: []string{"да", " "}},
		"eleven options":   {Title: "Да?", Options: make([]string, 11)},
		"forty days":       {Title: "Да?", Options: []string{"да"}, Days: 40},
		"a negative term":  {Title: "Да?", Options: []string{"да"}, Days: -1},
		"a very long line": {Title: strings.Repeat("я", 129), Options: []string{"да"}},
	} {
		if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "слова", Poll: d}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%s: got %v, want InvalidArgument", name, err)
		}
	}

	// A post without a poll still needs something to say.
	if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an empty post: got %v", err)
	}

	if _, err = s.client.GetPolls(anyone, &postv1.GetPollsRequest{PostIds: make([]string, 1)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("GetPolls of a malformed id: got %v", err)
	}
}

func TestSearchPosts(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	for author, text := range map[uuid.UUID]string{ann: "Идём в #поход", bob: "поход без тега"} {
		if _, err := s.client.CreatePost(s.as(t, author), &postv1.CreatePostRequest{Text: text, WallId: author.String()}); err != nil {
			t.Fatal(err)
		}
	}

	found, err := s.client.SearchPosts(s.as(t, bob), &postv1.SearchPostsRequest{Query: "#Поход"})
	if err != nil || found.GetTotal() != 1 || !slices.Equal(postTexts(found.GetPosts()), []string{"Идём в #поход"}) {
		t.Fatalf("the tag finds %v, %v", found, err)
	}

	found, err = s.client.SearchPosts(s.as(t, ann), &postv1.SearchPostsRequest{Query: "поход", HiddenAuthorIds: []string{bob.String()}})
	if err != nil || found.GetTotal() != 1 || !found.GetPosts()[0].GetMine() {
		t.Fatalf("without the blacklisted the text finds %v, %v", found, err)
	}

	if _, err = s.client.SearchPosts(s.as(t, ann), &postv1.SearchPostsRequest{Query: strings.Repeat("я", 101)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a long query: %v", err)
	}
	if _, err = s.client.SearchPosts(context.Background(), &postv1.SearchPostsRequest{Query: "поход"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: %v", err)
	}
}

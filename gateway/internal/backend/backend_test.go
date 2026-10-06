package backend

import (
	"context"
	"errors"
	"testing"
	"time"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFromStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		kind    failure.Kind
		message string
	}{
		{"service message passes", status.Error(codes.AlreadyExists, "nickname is already taken"), failure.Conflict, "nickname is already taken"},
		{"rate limit", status.Error(codes.ResourceExhausted, "too many requests, try again later"), failure.TooManyRequests, "too many requests, try again later"},
		{"transport errors carry addresses", status.Error(codes.Unavailable, "connection error: desc = dial tcp 10.0.0.5:50051"), failure.Unavailable, "service temporarily unavailable"},
		{"internal stays internal", status.Error(codes.Internal, "internal error"), failure.Internal, "internal error"},
		{"not a status", errors.New("boom"), failure.Internal, "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := failure.From(fromStatus(tt.err))
			if got.Kind != tt.kind || got.Message != tt.message {
				t.Fatalf("got %v %q, want %v %q", got.Kind, got.Message, tt.kind, tt.message)
			}
		})
	}
}

func TestEnums(t *testing.T) {
	t.Parallel()

	if got := enumName(userv1.Relationship_RELATIONSHIP_IN_LOVE.String(), relationshipPrefix); got != "in_love" {
		t.Fatalf("enumName = %q", got)
	}
	if got := enumName(userv1.Gender_GENDER_NONE.String(), genderPrefix); got != "" {
		t.Fatalf("not set must be empty, got %q", got)
	}

	req, err := toUpdateProfileRequest(model.Profile{
		Name: "Ann", Nickname: "ann", Gender: "female", PoliticalViews: "secular_humanism",
	})
	if err == nil {
		t.Fatalf("secular_humanism is a worldview, not political views: %v", req)
	}

	req, err = toUpdateProfileRequest(model.Profile{
		Name: "Ann", Nickname: "ann", Gender: "female", Worldview: "secular_humanism",
		Birthday: time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.GetGender() != userv1.Gender_GENDER_FEMALE || req.GetWorldview() != userv1.Worldview_WORLDVIEW_SECULAR_HUMANISM ||
		req.GetRelationship() != userv1.Relationship_RELATIONSHIP_NONE || req.GetBirthday().AsTime().Year() != 1990 {
		t.Fatalf("request: %v", req)
	}

	for _, bad := range []string{"Female", "none", "NONE", "robot"} {
		_, err = toUpdateProfileRequest(model.Profile{Gender: bad})
		if failure.From(err).Kind != failure.InvalidArgument {
			t.Fatalf("gender %q: got %v, want InvalidArgument", bad, err)
		}
	}
}

type fakeAuthAPI struct {
	authv1.AuthServiceClient
	md metadata.MD
}

func (f *fakeAuthAPI) RequestCode(ctx context.Context, _ *authv1.RequestCodeRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &emptypb.Empty{}, nil
}

type fakeUserAPI struct {
	userv1.UserServiceClient
	md metadata.MD
}

func (f *fakeUserAPI) GetFollowStats(ctx context.Context, _ *userv1.GetFollowStatsRequest, _ ...grpc.CallOption) (*userv1.GetFollowStatsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.GetFollowStatsResponse{FollowersCount: 2, FollowingCount: 1, IsFollowing: len(f.md.Get("authorization")) == 1}, nil
}

func (f *fakeUserAPI) GetMe(ctx context.Context, _ *userv1.GetMeRequest, _ ...grpc.CallOption) (*userv1.GetMeResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.GetMeResponse{User: &userv1.User{
		Id: "u1", Nickname: "ann", Gender: userv1.Gender_GENDER_FEMALE,
		Birthday: timestamppb.New(time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)),
		Privacy:  &userv1.PrivacySettings{ShowBirthday: true},
	}}, nil
}

func TestMetadata(t *testing.T) {
	t.Parallel()

	authAPI := &fakeAuthAPI{}
	if err := NewAuth(authAPI).RequestCode(context.Background(), "ann@example.com", "198.51.100.7"); err != nil {
		t.Fatal(err)
	}
	if got := authAPI.md.Get(authv1.ClientIPMetadata); len(got) != 1 || got[0] != "198.51.100.7" {
		t.Fatalf("client ip metadata = %v", got)
	}

	userAPI := &fakeUserAPI{}
	u, err := NewUsers(userAPI).GetMe(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got := userAPI.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}
	if u.Profile.Gender != "female" || u.Profile.Birthday.Year() != 1990 || !u.Privacy.ShowBirthday {
		t.Fatalf("user: %+v", u)
	}
}

type fakePostAPI struct {
	postv1.PostServiceClient
	md  metadata.MD
	req *postv1.ListPostsRequest
}

func (f *fakePostAPI) CreatePost(ctx context.Context, req *postv1.CreatePostRequest, _ ...grpc.CallOption) (*postv1.CreatePostResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &postv1.CreatePostResponse{Post: &postv1.Post{
		Id: "p1", AuthorId: "u1", Text: req.GetText(),
		CreatedAt: timestamppb.New(time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)),
	}}, nil
}

func (f *fakePostAPI) ListPosts(ctx context.Context, req *postv1.ListPostsRequest, _ ...grpc.CallOption) (*postv1.ListPostsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.req = req
	return &postv1.ListPostsResponse{
		Posts: []*postv1.Post{{
			Id: "p1", AuthorId: "u1", Text: "привет", CreatedAt: timestamppb.Now(),
			LikeCount: 4, CommentCount: 2, Liked: len(f.md.Get("authorization")) == 1,
		}},
		NextCursor: "p1",
		Total:      3,
	}, nil
}

func (f *fakePostAPI) LikePost(ctx context.Context, req *postv1.LikePostRequest, _ ...grpc.CallOption) (*postv1.LikePostResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if req.GetPostId() != "p1" {
		return nil, status.Error(codes.NotFound, "post not found")
	}
	return &postv1.LikePostResponse{LikeCount: 5}, nil
}

func (f *fakePostAPI) ListComments(_ context.Context, req *postv1.ListCommentsRequest, _ ...grpc.CallOption) (*postv1.ListCommentsResponse, error) {
	return &postv1.ListCommentsResponse{
		Comments: []*postv1.Comment{{
			Id: "c1", PostId: req.GetPostId(), AuthorId: "u2", Text: "согласен",
			CreatedAt: timestamppb.New(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)),
		}},
		NextCursor: "c1",
		Total:      7,
	}, nil
}

func TestPosts(t *testing.T) {
	t.Parallel()

	api := &fakePostAPI{}
	posts := NewPosts(api)

	p, err := posts.Create(context.Background(), "tok", "привет")
	if err != nil {
		t.Fatal(err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}
	if p.ID != "p1" || p.Author.ID != "u1" || p.Text != "привет" || p.CreatedAt.Hour() != 15 {
		t.Fatalf("post = %+v", p)
	}

	page, err := posts.List(context.Background(), "", "u1", "p9", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if api.req.GetAuthorId() != "u1" || api.req.GetCursor() != "p9" || api.req.GetLimit() != 1<<10 {
		t.Fatalf("request = %v", api.req)
	}
	if len(api.md.Get("authorization")) != 0 {
		t.Fatalf("anonymous wall sent a token: %v", api.md)
	}
	if page.Total != 3 || page.NextCursor != "p1" || len(page.Posts) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if got := page.Posts[0]; got.Likes != 4 || got.Comments != 2 || got.Liked {
		t.Fatalf("anonymous post = %+v", got)
	}

	page, err = posts.List(context.Background(), "tok", "u1", "", 0)
	if err != nil || !page.Posts[0].Liked {
		t.Fatalf("signed-in wall = %+v, %v", page, err)
	}

	likes, err := posts.Like(context.Background(), "tok", "p1")
	if err != nil || likes != 5 {
		t.Fatalf("Like = %d, %v", likes, err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("Like metadata = %v", got)
	}

	if _, err = posts.Like(context.Background(), "tok", "p2"); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("Like a missing post: %v", err)
	}

	comments, err := posts.Comments(context.Background(), "p1", "", 0)
	if err != nil || comments.Total != 7 || comments.NextCursor != "c1" || len(comments.Comments) != 1 {
		t.Fatalf("Comments = %+v, %v", comments, err)
	}
	if c := comments.Comments[0]; c.ID != "c1" || c.PostID != "p1" || c.Author.ID != "u2" || c.Text != "согласен" || c.CreatedAt.Hour() != 12 {
		t.Fatalf("comment = %+v", c)
	}
}

func TestFollowStatsToken(t *testing.T) {
	t.Parallel()

	api := &fakeUserAPI{}
	users := NewUsers(api)

	stats, err := users.FollowStats(context.Background(), "", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(api.md.Get("authorization")) != 0 || stats.IsFollowing || stats.Followers != 2 {
		t.Fatalf("anonymous: md %v, stats %+v", api.md, stats)
	}

	stats, err = users.FollowStats(context.Background(), "tok", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" || !stats.IsFollowing {
		t.Fatalf("signed in: md %v, stats %+v", got, stats)
	}
}

type fakeNotificationAPI struct {
	notificationv1.NotificationServiceClient
	md  metadata.MD
	req *notificationv1.ListNotificationsRequest
}

func (f *fakeNotificationAPI) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest, _ ...grpc.CallOption) (*notificationv1.ListNotificationsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.req = req
	return &notificationv1.ListNotificationsResponse{
		Notifications: []*notificationv1.Notification{
			{Id: "n2", Kind: notificationv1.Kind_KIND_COMMENT, ActorId: "u2", PostId: "p1", CommentId: "c1", CreatedAt: timestamppb.Now()},
			{Id: "n1", Kind: notificationv1.Kind_KIND_FOLLOW, ActorId: "u3", CreatedAt: timestamppb.Now()},
		},
		NextCursor: "n1",
	}, nil
}

func (f *fakeNotificationAPI) MarkRead(_ context.Context, req *notificationv1.MarkReadRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if req.GetNotificationId() != "n2" {
		return nil, status.Error(codes.NotFound, "notification not found")
	}
	return &emptypb.Empty{}, nil
}

func TestNotifications(t *testing.T) {
	t.Parallel()

	api := &fakeNotificationAPI{}
	notifications := NewNotifications(api)

	page, err := notifications.List(context.Background(), "tok", false, "n9", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if api.req.GetBox() != notificationv1.Box_BOX_ARCHIVE || api.req.GetCursor() != "n9" || api.req.GetLimit() != 1<<10 {
		t.Fatalf("request = %v", api.req)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}

	comment, follow := page.Notifications[0], page.Notifications[1]
	if comment.Kind != "comment" || comment.PostID != "p1" || comment.CommentID != "c1" || follow.Kind != "follow" || page.NextCursor != "n1" {
		t.Fatalf("page = %+v", page)
	}

	if _, err = notifications.List(context.Background(), "tok", true, "", 0); err != nil || api.req.GetBox() != notificationv1.Box_BOX_UNREAD {
		t.Fatalf("unread box: %v, %v", api.req, err)
	}

	if err = notifications.MarkRead(context.Background(), "tok", "n0"); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("MarkRead of someone else's: %v", err)
	}
}

type fakeMessageAPI struct {
	messagev1.MessageServiceClient
	md  metadata.MD
	req *messagev1.ListMessagesRequest
}

func (f *fakeMessageAPI) ListMessages(ctx context.Context, req *messagev1.ListMessagesRequest, _ ...grpc.CallOption) (*messagev1.ListMessagesResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.req = req
	return &messagev1.ListMessagesResponse{
		Messages:     []*messagev1.Message{{Id: "m1", SenderId: "u1", RecipientId: "u2", Text: "привет", CreatedAt: timestamppb.Now(), Read: true}},
		NextCursor:   "m1",
		PeerReadUpTo: "m1",
	}, nil
}

func (f *fakeMessageAPI) ListDialogs(_ context.Context, _ *messagev1.ListDialogsRequest, _ ...grpc.CallOption) (*messagev1.ListDialogsResponse, error) {
	return &messagev1.ListDialogsResponse{Dialogs: []*messagev1.Dialog{
		{PeerId: "u2", LastMessage: &messagev1.Message{Id: "m3", SenderId: "u2", Text: "ау"}, Unread: true},
	}}, nil
}

func (f *fakeMessageAPI) SendMessage(_ context.Context, _ *messagev1.SendMessageRequest, _ ...grpc.CallOption) (*messagev1.SendMessageResponse, error) {
	return nil, status.Error(codes.InvalidArgument, "you cannot write to yourself")
}

func TestMessages(t *testing.T) {
	t.Parallel()

	api := &fakeMessageAPI{}
	messages := NewMessages(api)

	page, err := messages.History(context.Background(), "tok", "u2", "", "m0", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if api.req.GetPeerId() != "u2" || api.req.GetAfter() != "m0" || api.req.GetLimit() != 1<<10 {
		t.Fatalf("request = %v", api.req)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}
	if m := page.Messages[0]; m.ID != "m1" || m.SenderID != "u1" || !m.Read || page.PeerReadUpTo != "m1" || page.NextCursor != "m1" {
		t.Fatalf("page = %+v", page)
	}

	dialogs, err := messages.Dialogs(context.Background(), "tok", "", 0)
	if err != nil || len(dialogs.Dialogs) != 1 || !dialogs.Dialogs[0].Unread || dialogs.Dialogs[0].LastMessage.Text != "ау" {
		t.Fatalf("dialogs = %+v, %v", dialogs, err)
	}

	_, err = messages.Send(context.Background(), "tok", "u1", "x")
	if f := failure.From(err); f.Kind != failure.InvalidArgument || f.Message != "you cannot write to yourself" {
		t.Fatalf("Send to yourself: %v", err)
	}
}

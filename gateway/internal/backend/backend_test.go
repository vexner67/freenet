package backend

import (
	"context"
	"errors"
	"testing"
	"time"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
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
	}, "")
	if err == nil {
		t.Fatalf("secular_humanism is a worldview, not political views: %v", req)
	}

	req, err = toUpdateProfileRequest(model.Profile{
		Name: "Ann", Nickname: "ann", Gender: "female", Worldview: "secular_humanism",
		Birthday: time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC), Hometown: "Тверь", Relationship: "civil_union",
	}, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if req.GetGender() != userv1.Gender_GENDER_FEMALE || req.GetWorldview() != userv1.Worldview_WORLDVIEW_SECULAR_HUMANISM ||
		req.GetRelationship() != userv1.Relationship_RELATIONSHIP_CIVIL_UNION || req.GetBirthday().AsTime().Year() != 1990 ||
		req.GetHometown() != "Тверь" || req.GetRelationshipPartner() != "bob" {
		t.Fatalf("request: %v", req)
	}

	for _, bad := range []string{"Female", "none", "NONE", "robot"} {
		_, err = toUpdateProfileRequest(model.Profile{Gender: bad}, "")
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
	md      metadata.MD
	privacy *userv1.PrivacySettings
}

func (f *fakeUserAPI) UpdatePrivacy(_ context.Context, req *userv1.UpdatePrivacyRequest, _ ...grpc.CallOption) (*userv1.UpdatePrivacyResponse, error) {
	f.privacy = req.GetPrivacy()
	return &userv1.UpdatePrivacyResponse{User: &userv1.User{Id: "u1", Privacy: req.GetPrivacy()}}, nil
}

// GetAccess shows the photos only, and GetProfile hides the page; both
// keep the caller's token.
func (f *fakeUserAPI) GetAccess(ctx context.Context, req *userv1.GetAccessRequest, _ ...grpc.CallOption) (*userv1.GetAccessResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.GetAccessResponse{Page: req.GetUserId() == "u2", Photos: true}, nil
}

func (f *fakeUserAPI) GetProfile(ctx context.Context, req *userv1.GetProfileRequest, _ ...grpc.CallOption) (*userv1.GetProfileResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.GetProfileResponse{Profile: &userv1.PublicProfile{Id: "u2", Nickname: req.GetNickname(), PageHidden: true, InfoHidden: true}}, nil
}

func (f *fakeUserAPI) CanMessage(ctx context.Context, req *userv1.CanMessageRequest, _ ...grpc.CallOption) (*userv1.CanMessageResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.CanMessageResponse{Allowed: req.GetWroteFirst()}, nil
}

func (f *fakeUserAPI) GetRelation(context.Context, *userv1.GetRelationRequest, ...grpc.CallOption) (*userv1.GetRelationResponse, error) {
	return &userv1.GetRelationResponse{BlockedMe: true}, nil
}

func (f *fakeUserAPI) ListBlocked(_ context.Context, req *userv1.ListBlockedRequest, _ ...grpc.CallOption) (*userv1.ListBlockedResponse, error) {
	return &userv1.ListBlockedResponse{
		Users:      []*userv1.PublicProfile{{Id: "u2", Nickname: "bob"}},
		NextCursor: req.GetCursor() + "+",
		Total:      1,
	}, nil
}

func (f *fakeUserAPI) Block(_ context.Context, req *userv1.BlockRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if req.GetUserId() == "self" {
		return nil, status.Error(codes.InvalidArgument, "you cannot blacklist yourself")
	}
	return &emptypb.Empty{}, nil
}

func (f *fakeUserAPI) CanPostOnWall(ctx context.Context, req *userv1.CanPostOnWallRequest, _ ...grpc.CallOption) (*userv1.CanPostOnWallResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.CanPostOnWallResponse{Allowed: req.GetUserId() == "u2"}, nil
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
		Privacy:  &userv1.PrivacySettings{ShowBirthday: true, WallWrite: userv1.WallWrite_WALL_WRITE_ONLY_ME},
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
	if u.Profile.Gender != "female" || u.Profile.Birthday.Year() != 1990 || !u.Privacy.ShowBirthday || u.Privacy.WallWrite != "only_me" {
		t.Fatalf("user: %+v", u)
	}
}

func TestWallPrivacy(t *testing.T) {
	t.Parallel()

	api := &fakeUserAPI{}
	users := NewUsers(api)

	u, err := users.UpdatePrivacy(context.Background(), "tok", model.Privacy{
		ShowWorldview: true, WallWrite: "everyone", MessagesWrite: "nobody", ClubsRead: "only_me",
		PageRead: "users", InfoRead: "following", PhotosRead: "only_me", FriendsRead: "everyone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if api.privacy.GetWallWrite() != userv1.WallWrite_WALL_WRITE_EVERYONE || u.Privacy.WallWrite != "everyone" {
		t.Fatalf("sent %v, got %+v", api.privacy, u.Privacy)
	}
	if api.privacy.GetMessagesWrite() != userv1.MessagesWrite_MESSAGES_WRITE_NOBODY || u.Privacy.MessagesWrite != "nobody" {
		t.Fatalf("sent %v, got %+v", api.privacy, u.Privacy)
	}
	if api.privacy.GetClubsRead() != userv1.ClubsRead_CLUBS_READ_ONLY_ME || u.Privacy.ClubsRead != "only_me" {
		t.Fatalf("sent %v, got %+v", api.privacy, u.Privacy)
	}
	if api.privacy.GetPageRead() != userv1.Audience_AUDIENCE_USERS || api.privacy.GetInfoRead() != userv1.Audience_AUDIENCE_FOLLOWING ||
		api.privacy.GetPhotosRead() != userv1.Audience_AUDIENCE_ONLY_ME || api.privacy.GetFriendsRead() != userv1.Audience_AUDIENCE_EVERYONE ||
		u.Privacy.PageRead != "users" || u.Privacy.InfoRead != "following" || u.Privacy.PhotosRead != "only_me" || u.Privacy.FriendsRead != "everyone" {
		t.Fatalf("sent %v, got %+v", api.privacy, u.Privacy)
	}

	for _, bad := range []model.Privacy{{WallWrite: "friends", MessagesWrite: "everyone"}, {WallWrite: "everyone", MessagesWrite: "only_me"}, {WallWrite: "everyone", MessagesWrite: "everyone", ClubsRead: "nobody"},
		{WallWrite: "everyone", MessagesWrite: "everyone", PhotosRead: "friends"},
	} {
		if _, err = users.UpdatePrivacy(context.Background(), "tok", bad); failure.From(err).Kind != failure.InvalidArgument {
			t.Fatalf("privacy %+v: %v", bad, err)
		}
	}

	access, err := users.Access(context.Background(), "tok", "u2")
	if err != nil || access != (model.Access{Page: true, Photos: true}) || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("Access = %+v, %v", access, err)
	}

	profile, err := users.GetProfile(context.Background(), "tok", "bob")
	if err != nil || !profile.PageHidden || !profile.InfoHidden || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("GetProfile = %+v, %v", profile, err)
	}

	allowed, err := users.CanPostOnWall(context.Background(), "tok", "u2")
	if err != nil || !allowed {
		t.Fatalf("CanPostOnWall = %v, %v", allowed, err)
	}

	if allowed, err = users.CanMessage(context.Background(), "tok", "u2", true); err != nil || !allowed {
		t.Fatalf("CanMessage passes wroteFirst on: %v, %v", allowed, err)
	}

	rel, err := users.Relation(context.Background(), "tok", "u2")
	if err != nil || !rel.BlockedMe || rel.BlockedByMe || !rel.Blocked() {
		t.Fatalf("Relation = %+v, %v", rel, err)
	}

	page, err := users.Blacklist(context.Background(), "tok", "c1", 10)
	if err != nil || page.Total != 1 || page.Users[0].Profile.Nickname != "bob" || page.NextCursor != "c1+" {
		t.Fatalf("Blacklist = %+v, %v", page, err)
	}

	if err = users.Block(context.Background(), "tok", "self"); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("Block yourself: %v", err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
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
		Id: "p1", AuthorId: "u1", WallId: req.GetWallId(), Text: req.GetText(), PhotoIds: req.GetPhotoIds(),
		CreatedAt: timestamppb.New(time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)),
	}}, nil
}

func (f *fakePostAPI) ListPosts(ctx context.Context, req *postv1.ListPostsRequest, _ ...grpc.CallOption) (*postv1.ListPostsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.req = req
	return &postv1.ListPostsResponse{
		Posts: []*postv1.Post{{
			Id: "p1", AuthorId: "u2", WallId: "u1", Text: "привет", CreatedAt: timestamppb.Now(),
			LikeCount: 4, CommentCount: 2, RepostCount: 5, Liked: len(f.md.Get("authorization")) == 1,
		}},
		NextCursor: "p1",
		Total:      3,
	}, nil
}

func (f *fakePostAPI) RepostPost(ctx context.Context, req *postv1.RepostPostRequest, _ ...grpc.CallOption) (*postv1.RepostPostResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if req.GetPostId() != "p1" {
		return nil, status.Error(codes.NotFound, "post not found")
	}
	return &postv1.RepostPostResponse{Post: &postv1.Post{
		Id: "p2", AuthorId: "u2", WallId: "u2", Text: req.GetText(), RepostOf: "p1", CreatedAt: timestamppb.Now(),
	}}, nil
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

	p, err := posts.Create(context.Background(), "tok", model.NewPost{WallID: "u2", Text: "привет", PhotoIDs: []string{"ph1", "ph2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.PhotoIDs) != 2 || p.PhotoIDs[1] != "ph2" {
		t.Fatalf("photos = %v", p.PhotoIDs)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}
	if p.ID != "p1" || p.Author.ID != "u1" || p.Wall.ID != "u2" || p.Text != "привет" || p.CreatedAt.Hour() != 15 {
		t.Fatalf("post = %+v", p)
	}

	page, err := posts.List(context.Background(), "", "u1", "p9", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if api.req.GetWallId() != "u1" || api.req.GetCursor() != "p9" || api.req.GetLimit() != 1<<10 {
		t.Fatalf("request = %v", api.req)
	}
	if len(api.md.Get("authorization")) != 0 {
		t.Fatalf("anonymous wall sent a token: %v", api.md)
	}
	if page.Total != 3 || page.NextCursor != "p1" || len(page.Posts) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if got := page.Posts[0]; got.Likes != 4 || got.Comments != 2 || got.Reposts != 5 || got.Liked || got.Author.ID != "u2" || got.Wall.ID != "u1" {
		t.Fatalf("anonymous post = %+v", got)
	}

	page, err = posts.List(context.Background(), "tok", "u1", "", 0)
	if err != nil || !page.Posts[0].Liked {
		t.Fatalf("signed-in wall = %+v, %v", page, err)
	}

	repost, err := posts.Repost(context.Background(), "tok", "p1", "смотрите")
	if err != nil || repost.RepostOf != "p1" || repost.Text != "смотрите" || repost.Wall.ID != "u2" {
		t.Fatalf("Repost = %+v, %v", repost, err)
	}
	if _, err = posts.Repost(context.Background(), "tok", "p2", ""); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("Repost a missing post: %v", err)
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

	comments, err := posts.Comments(context.Background(), "", "p1", "", 0)
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

func (f *fakeMessageAPI) HasMessagesFrom(ctx context.Context, req *messagev1.HasMessagesFromRequest, _ ...grpc.CallOption) (*messagev1.HasMessagesFromResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &messagev1.HasMessagesFromResponse{HasMessages: req.GetPeerId() == "u2"}, nil
}

func (f *fakeMessageAPI) ListMessages(ctx context.Context, req *messagev1.ListMessagesRequest, _ ...grpc.CallOption) (*messagev1.ListMessagesResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.req = req
	return &messagev1.ListMessagesResponse{
		Messages: []*messagev1.Message{{
			Id: "m1", SenderId: "u1", RecipientId: "u2", Text: "привет", CreatedAt: timestamppb.Now(), Read: true,
			DeletableUntil: timestamppb.New(time.Date(2026, 10, 6, 22, 0, 0, 0, time.UTC)),
		}},
		NextCursor:   "m1",
		PeerReadUpTo: "m1",
	}, nil
}

func (f *fakeMessageAPI) ListDialogs(_ context.Context, _ *messagev1.ListDialogsRequest, _ ...grpc.CallOption) (*messagev1.ListDialogsResponse, error) {
	return &messagev1.ListDialogsResponse{Dialogs: []*messagev1.Dialog{
		{PeerId: "u2", LastMessage: &messagev1.Message{Id: "m3", SenderId: "u2", Text: "ау"}, Unread: true},
	}}, nil
}

func (f *fakeMessageAPI) DeleteMessage(_ context.Context, req *messagev1.DeleteMessageRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if req.GetId() != "m1" {
		return nil, status.Error(codes.PermissionDenied, "only the sender can delete the message")
	}
	return &emptypb.Empty{}, nil
}

func (f *fakeMessageAPI) SendMessage(_ context.Context, _ *messagev1.SendMessageRequest, _ ...grpc.CallOption) (*messagev1.SendMessageResponse, error) {
	return nil, status.Error(codes.InvalidArgument, "you cannot write to yourself")
}

func TestMessages(t *testing.T) {
	t.Parallel()

	api := &fakeMessageAPI{}
	messages := NewMessages(api)

	if wrote, err := messages.HasMessagesFrom(context.Background(), "tok", "u2"); err != nil || !wrote || len(api.md.Get("authorization")) != 1 {
		t.Fatalf("HasMessagesFrom = %v, %v (metadata %v)", wrote, err, api.md)
	}

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
	if m := page.Messages[0]; m.ID != "m1" || m.SenderID != "u1" || !m.Read || page.PeerReadUpTo != "m1" || page.NextCursor != "m1" ||
		!m.DeletableUntil.Equal(time.Date(2026, 10, 6, 22, 0, 0, 0, time.UTC)) {
		t.Fatalf("page = %+v", page)
	}

	dialogs0, err := messages.Dialogs(context.Background(), "tok", "", 0)
	if err != nil || !dialogs0.Dialogs[0].LastMessage.DeletableUntil.IsZero() {
		t.Fatalf("a message without a deadline = %+v, %v; want a zero time", dialogs0, err)
	}

	if err = messages.Delete(context.Background(), "tok", "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if f := failure.From(messages.Delete(context.Background(), "tok", "m2")); f.Kind != failure.PermissionDenied {
		t.Fatalf("Delete of someone else's: %+v", f)
	}

	dialogs, err := messages.Dialogs(context.Background(), "tok", "", 0)
	if err != nil || len(dialogs.Dialogs) != 1 || !dialogs.Dialogs[0].Unread || dialogs.Dialogs[0].LastMessage.Text != "ау" {
		t.Fatalf("dialogs = %+v, %v", dialogs, err)
	}

	_, err = messages.Send(context.Background(), "tok", "u1", "x", nil)
	if f := failure.From(err); f.Kind != failure.InvalidArgument || f.Message != "you cannot write to yourself" {
		t.Fatalf("Send to yourself: %v", err)
	}
}

type fakeMediaAPI struct {
	mediav1.MediaServiceClient
	md   metadata.MD
	size mediav1.PhotoSize
}

func (f *fakeMediaAPI) UploadPhoto(ctx context.Context, _ *mediav1.UploadPhotoRequest, _ ...grpc.CallOption) (*mediav1.UploadPhotoResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &mediav1.UploadPhotoResponse{PhotoId: "ph1"}, nil
}

func (f *fakeMediaAPI) GetPhoto(_ context.Context, req *mediav1.GetPhotoRequest, _ ...grpc.CallOption) (*mediav1.GetPhotoResponse, error) {
	f.size = req.GetSize()
	return &mediav1.GetPhotoResponse{Image: []byte("jpeg")}, nil
}

func (f *fakeMediaAPI) ListOwnPhotos(_ context.Context, req *mediav1.ListOwnPhotosRequest, _ ...grpc.CallOption) (*mediav1.ListOwnPhotosResponse, error) {
	return &mediav1.ListOwnPhotosResponse{PhotoIds: req.GetPhotoIds()[:1]}, nil
}

func TestMediaPhotos(t *testing.T) {
	t.Parallel()

	api := &fakeMediaAPI{}
	media := NewMedia(api)

	id, err := media.UploadPhoto(context.Background(), "tok", []byte("png"))
	if err != nil || id != "ph1" {
		t.Fatalf("UploadPhoto = %q, %v", id, err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}

	if _, err = media.Photo(context.Background(), "u1", "ph1", true); err != nil || api.size != mediav1.PhotoSize_PHOTO_SIZE_SMALL {
		t.Fatalf("small photo: size %v, %v", api.size, err)
	}
	if _, err = media.Photo(context.Background(), "u1", "ph1", false); err != nil || api.size != mediav1.PhotoSize_PHOTO_SIZE_LARGE {
		t.Fatalf("large photo: size %v, %v", api.size, err)
	}

	own, err := media.OwnPhotos(context.Background(), "tok", []string{"ph1", "ph2"})
	if err != nil || len(own) != 1 || own[0] != "ph1" {
		t.Fatalf("OwnPhotos = %v, %v", own, err)
	}
}

// fakeStream yields responses, then err.
type fakeStream[T any] struct {
	grpc.ServerStreamingClient[T]
	responses []*T
	err       error
}

func (s *fakeStream[T]) Recv() (*T, error) {
	if len(s.responses) == 0 {
		return nil, s.err
	}

	r := s.responses[0]
	s.responses = s.responses[1:]

	return r, nil
}

func (f *fakeMessageAPI) Subscribe(ctx context.Context, _ *messagev1.SubscribeRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[messagev1.SubscribeResponse], error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &fakeStream[messagev1.SubscribeResponse]{
		responses: []*messagev1.SubscribeResponse{
			{Event: &messagev1.SubscribeResponse_Message{Message: &messagev1.Message{Id: "m1", SenderId: "u2", RecipientId: "u1", Text: "привет"}}},
			{Event: &messagev1.SubscribeResponse_Read{Read: &messagev1.ReadMark{ReaderId: "u2", PeerId: "u1", UpToId: "m1"}}},
		},
		err: status.Error(codes.Unauthenticated, "access token expired"),
	}, nil
}

func TestMessages_Subscribe(t *testing.T) {
	t.Parallel()

	api := &fakeMessageAPI{}
	next, err := NewMessages(api).Subscribe(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}

	e, err := next()
	if err != nil || e.Message == nil || e.Message.ID != "m1" || e.Message.Text != "привет" || e.Read != nil {
		t.Fatalf("first event = %+v, %v", e, err)
	}

	e, err = next()
	if err != nil || e.Read == nil || *e.Read != (model.ReadMark{ReaderID: "u2", PeerID: "u1", UpTo: "m1"}) {
		t.Fatalf("second event = %+v, %v", e, err)
	}

	if _, err = next(); failure.From(err).Kind != failure.Unauthenticated {
		t.Fatalf("the end of the stream: %v, want Unauthenticated", err)
	}
}

func (f *fakeNotificationAPI) Subscribe(context.Context, *notificationv1.SubscribeRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[notificationv1.SubscribeResponse], error) {
	return &fakeStream[notificationv1.SubscribeResponse]{
		responses: []*notificationv1.SubscribeResponse{{Change: notificationv1.Change_CHANGE_NEW}, {Change: notificationv1.Change_CHANGE_READ}},
		err:       status.Error(codes.ResourceExhausted, "fell behind, subscribe again"),
	}, nil
}

func TestNotifications_Subscribe(t *testing.T) {
	t.Parallel()

	next, err := NewNotifications(&fakeNotificationAPI{}).Subscribe(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"new", "read"} {
		if got, err := next(); err != nil || got != want {
			t.Fatalf("change = %q, %v; want %q", got, err, want)
		}
	}

	if _, err = next(); failure.From(err).Kind != failure.TooManyRequests {
		t.Fatalf("the end of the stream: %v, want TooManyRequests", err)
	}
}

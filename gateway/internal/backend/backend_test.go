package backend

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
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

func (f *fakeAuthAPI) VerifyCode(ctx context.Context, _ *authv1.VerifyCodeRequest, _ ...grpc.CallOption) (*authv1.TokenPair, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &authv1.TokenPair{AccessToken: "a", RefreshToken: "r"}, nil
}

func (f *fakeAuthAPI) ListSessions(ctx context.Context, _ *authv1.ListSessionsRequest, _ ...grpc.CallOption) (*authv1.ListSessionsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	used := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	return &authv1.ListSessionsResponse{Sessions: []*authv1.Session{{
		Id: "s1", CreatedAt: timestamppb.New(used.Add(-time.Hour)), LastUsedAt: timestamppb.New(used),
		Ip: "203.0.113.7", UserAgent: "Mozilla/5.0", Current: true,
	}}}, nil
}

func (f *fakeAuthAPI) EndSession(_ context.Context, req *authv1.EndSessionRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if req.GetSessionId() != "s1" {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	return &emptypb.Empty{}, nil
}

func (f *fakeAuthAPI) EndOtherSessions(context.Context, *authv1.EndOtherSessionsRequest, ...grpc.CallOption) (*authv1.EndOtherSessionsResponse, error) {
	return &authv1.EndOtherSessionsResponse{Ended: 3}, nil
}

func (f *fakeAuthAPI) RequestCode(ctx context.Context, _ *authv1.RequestCodeRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &emptypb.Empty{}, nil
}

type fakeUserAPI struct {
	userv1.UserServiceClient
	md      metadata.MD
	privacy *userv1.PrivacySettings
	// online: ListFollowing was asked for the people online.
	online bool
}

func (f *fakeUserAPI) UpdatePrivacy(_ context.Context, req *userv1.UpdatePrivacyRequest, _ ...grpc.CallOption) (*userv1.UpdatePrivacyResponse, error) {
	f.privacy = req.GetPrivacy()
	return &userv1.UpdatePrivacyResponse{User: &userv1.User{Id: "u1", Privacy: req.GetPrivacy()}}, nil
}

// GetAccess shows the photos only, and GetProfile hides the page; both
// keep the caller's token.
func (f *fakeUserAPI) GetAccess(ctx context.Context, req *userv1.GetAccessRequest, _ ...grpc.CallOption) (*userv1.GetAccessResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.GetAccessResponse{Page: req.GetUserId() == "u2", Photos: true, Notes: true}, nil
}

func (f *fakeUserAPI) GetProfile(ctx context.Context, req *userv1.GetProfileRequest, _ ...grpc.CallOption) (*userv1.GetProfileResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &userv1.GetProfileResponse{Profile: &userv1.PublicProfile{
		Id: "u2", Nickname: req.GetNickname(), PageHidden: true, InfoHidden: true,
		LastSeenAt: timestamppb.New(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)), Online: true,
	}}, nil
}

func (f *fakeUserAPI) MarkOnline(ctx context.Context, _ *userv1.MarkOnlineRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return &emptypb.Empty{}, nil
}

func (f *fakeUserAPI) ListFollowing(_ context.Context, req *userv1.ListFollowsRequest, _ ...grpc.CallOption) (*userv1.ListFollowsResponse, error) {
	f.online = req.GetOnline()
	return &userv1.ListFollowsResponse{Total: 1}, nil
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
		PageRead: "users", InfoRead: "following", PhotosRead: "only_me", FriendsRead: "everyone", OnlineRead: "following", NotesRead: "only_me",
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
		u.Privacy.PageRead != "users" || u.Privacy.InfoRead != "following" || u.Privacy.PhotosRead != "only_me" || u.Privacy.FriendsRead != "everyone" ||
		api.privacy.GetOnlineRead() != userv1.Audience_AUDIENCE_FOLLOWING || u.Privacy.OnlineRead != "following" ||
		api.privacy.GetNotesRead() != userv1.Audience_AUDIENCE_ONLY_ME || u.Privacy.NotesRead != "only_me" {
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
	if err != nil || access != (model.Access{Page: true, Photos: true, Notes: true}) || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("Access = %+v, %v", access, err)
	}

	profile, err := users.GetProfile(context.Background(), "tok", "bob")
	if err != nil || !profile.PageHidden || !profile.InfoHidden || api.md.Get("authorization")[0] != "Bearer tok" ||
		!profile.Online || profile.LastSeen.Hour() != 12 {
		t.Fatalf("GetProfile = %+v, %v", profile, err)
	}

	api.md = nil
	if err = users.MarkOnline(context.Background(), "tok"); err != nil || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("MarkOnline: %v", err)
	}
	if page, err := users.ListOnlineFollowing(context.Background(), "u1", "", 6); err != nil || !api.online || page.Total != 1 {
		t.Fatalf("ListOnlineFollowing = %+v, %v (online %v)", page, err, api.online)
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
			Id: "p1", AuthorId: "u2", WallId: "u1", Text: "привет", TextCut: true, CreatedAt: timestamppb.Now(),
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

	page, err := posts.List(context.Background(), "", "u1", model.WallPick{Filter: "others", Query: "море"}, "p9", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = posts.List(context.Background(), "", "u1", model.WallPick{Filter: "mine"}, "", 0); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("a bad wall type: %v", err)
	}
	if api.req.GetTextLimit() != model.ExcerptLength || !page.Posts[0].TextCut {
		t.Fatalf("a wall asks for the beginnings of texts: limit = %d, post = %+v", api.req.GetTextLimit(), page.Posts[0])
	}
	if api.req.GetWallId() != "u1" || api.req.GetCursor() != "p9" || api.req.GetLimit() != 1<<10 || api.req.GetFilter() != postv1.WallFilter_WALL_FILTER_OTHERS || api.req.GetQuery() != "море" {
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

	page, err = posts.List(context.Background(), "tok", "u1", model.WallPick{}, "", 0)
	if err != nil || !page.Posts[0].Liked {
		t.Fatalf("signed-in wall = %+v, %v", page, err)
	}

	repost, err := posts.Repost(context.Background(), "tok", "p1", model.NewRepost{Text: "смотрите"})
	if err != nil || repost.RepostOf != "p1" || repost.Text != "смотрите" || repost.Wall.ID != "u2" {
		t.Fatalf("Repost = %+v, %v", repost, err)
	}
	if _, err = posts.Repost(context.Background(), "tok", "p2", model.NewRepost{}); failure.From(err).Kind != failure.NotFound {
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

func (f *fakePostAPI) GetPolls(ctx context.Context, req *postv1.GetPollsRequest, _ ...grpc.CallOption) (*postv1.GetPollsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)

	polls := make([]*postv1.Poll, 0, len(req.GetPostIds()))
	for _, id := range req.GetPostIds() {
		polls = append(polls, &postv1.Poll{
			PostId: id, Title: "Да?", VoterCount: 3, Multiple: true, Locked: true,
			Options: []*postv1.PollOption{{Name: "да", VoteCount: 3}, {Name: "нет", VoteCount: 1}},
			EndsAt:  timestamppb.New(time.Date(2026, 10, 6, 15, 30, 0, 0, time.UTC)),
			Chosen:  []int32{0, 1},
		})
	}

	return &postv1.GetPollsResponse{Polls: polls}, nil
}

func (f *fakePostAPI) VotePoll(ctx context.Context, req *postv1.VotePollRequest, _ ...grpc.CallOption) (*postv1.VotePollResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if req.GetPostId() != "p1" {
		return nil, status.Error(codes.FailedPrecondition, "the poll has ended")
	}

	return &postv1.VotePollResponse{Poll: &postv1.Poll{PostId: "p1", Chosen: req.GetOptions()}}, nil
}

func (f *fakePostAPI) RetractPollVote(_ context.Context, req *postv1.RetractPollVoteRequest, _ ...grpc.CallOption) (*postv1.RetractPollVoteResponse, error) {
	return &postv1.RetractPollVoteResponse{Poll: &postv1.Poll{PostId: req.GetPostId(), Anonymous: true}}, nil
}

func (f *fakePostAPI) ListPollVoters(ctx context.Context, req *postv1.ListPollVotersRequest, _ ...grpc.CallOption) (*postv1.ListPollVotersResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if req.GetOption() != 1 || req.GetCursor() != "v1" || req.GetLimit() != 6 {
		return nil, status.Error(codes.InvalidArgument, "unexpected request")
	}

	return &postv1.ListPollVotersResponse{UserIds: []string{"u2"}, NextCursor: "v2", Total: 4}, nil
}

func TestPolls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakePostAPI{}
	posts := NewPosts(api)

	polls, err := posts.Polls(ctx, "tok", []string{"p1", "p2"})
	if err != nil || len(polls) != 2 {
		t.Fatalf("Polls = %+v, %v", polls, err)
	}

	want := model.Poll{
		PostID: "p2", Title: "Да?", Voters: 3, Multiple: true, Locked: true,
		Options: []model.PollOption{{Name: "да", Votes: 3}, {Name: "нет", Votes: 1}},
		EndsAt:  time.Date(2026, 10, 6, 15, 30, 0, 0, time.UTC),
		Chosen:  []int{0, 1},
	}
	if !reflect.DeepEqual(polls[1], want) {
		t.Fatalf("poll = %+v, want %+v", polls[1], want)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("Polls metadata = %v", got)
	}

	voted, err := posts.Vote(ctx, "tok", "p1", []int{1})
	if err != nil || !reflect.DeepEqual(voted.Chosen, []int{1}) {
		t.Fatalf("Vote = %+v, %v", voted, err)
	}

	// What the poll refuses is told to the client as a conflict, in words.
	if _, err = posts.Vote(ctx, "tok", "p2", []int{1}); failure.From(err).Kind != failure.Conflict || failure.From(err).Message != "the poll has ended" {
		t.Fatalf("Vote in an ended poll: %v", err)
	}
	if _, err = posts.Vote(ctx, "tok", "p1", []int{-1}); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("Vote for a negative option: %v", err)
	}

	back, err := posts.RetractVote(ctx, "tok", "p1")
	if err != nil || back.PostID != "p1" || !back.Anonymous || !back.EndsAt.IsZero() || len(back.Chosen) != 0 {
		t.Fatalf("RetractVote = %+v, %v", back, err)
	}

	voters, err := posts.PollVoters(ctx, "p1", 1, "v1", 6)
	if err != nil || len(voters.UserIDs) != 1 || voters.NextCursor != "v2" || voters.Total != 4 {
		t.Fatalf("PollVoters = %+v, %v", voters, err)
	}
	if len(api.md.Get("authorization")) != 0 {
		t.Fatalf("voters are public, a token was sent: %v", api.md)
	}

	// A new post carries its poll.
	if got := toProtoPollDraft(&model.NewPoll{Title: "Да?", Options: []string{"да"}, Locked: true, Days: 1 << 40}); got.GetTitle() != "Да?" ||
		len(got.GetOptions()) != 1 || !got.GetLocked() || got.GetDays() != math.MaxInt32 {
		t.Fatalf("draft = %v", got)
	}
	if toProtoPollDraft(nil) != nil {
		t.Fatal("a post without a poll sent one")
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
	// edit and removal are the last EditMessage and DeleteMessages.
	edit    *messagev1.EditMessageRequest
	removal *messagev1.DeleteMessagesRequest
}

func (f *fakeMessageAPI) EditMessage(_ context.Context, req *messagev1.EditMessageRequest, _ ...grpc.CallOption) (*messagev1.EditMessageResponse, error) {
	f.edit = req
	return &messagev1.EditMessageResponse{Message: &messagev1.Message{Id: req.GetId(), Text: req.GetText(), PhotoIds: req.GetPhotoIds()}}, nil
}

func (f *fakeMessageAPI) DeleteMessages(_ context.Context, req *messagev1.DeleteMessagesRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.removal = req
	if len(req.GetIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no messages")
	}

	return &emptypb.Empty{}, nil
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
			Forwarded: []*messagev1.ForwardedMessage{{
				SenderId: "u3", Text: "смотри", PhotoIds: []string{"ph1"}, PostId: "p1", CreatedAt: timestamppb.New(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)),
			}},
		}, {
			Id: "m2", SenderId: "u2", RecipientId: "u1", CreatedAt: timestamppb.Now(),
			DeletedAt: timestamppb.New(time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)),
		}},
		NextCursor:   "m1",
		PeerReadUpTo: "m1",
	}, nil
}

func (f *fakeMessageAPI) ListDialogs(_ context.Context, req *messagev1.ListDialogsRequest, _ ...grpc.CallOption) (*messagev1.ListDialogsResponse, error) {
	return &messagev1.ListDialogsResponse{Dialogs: []*messagev1.Dialog{
		{PeerId: "u2", LastMessage: &messagev1.Message{Id: "m3", SenderId: "u2", Text: "ау", TextCut: req.GetTextLimit() == model.PreviewLength}, Unread: true},
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

	page, err := messages.History(context.Background(), "tok", "u2", model.MessagesAt{After: "m0"}, 1<<20)
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

	if f := page.Messages[0].Forwarded; len(f) != 1 || f[0].SenderID != "u3" || f[0].Text != "смотри" || f[0].PhotoIDs[0] != "ph1" || f[0].PostID != "p1" ||
		!f[0].CreatedAt.Equal(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("forwarded = %+v", f)
	}
	if gone := page.Messages[1]; !gone.DeletedAt.Equal(time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)) || !page.Messages[0].DeletedAt.IsZero() {
		t.Fatalf("messages = %+v; want the second taken back", page.Messages)
	}

	if _, err = messages.History(context.Background(), "tok", "u2", model.MessagesAt{Around: "m5"}, 0); err != nil || api.req.GetAround() != "m5" || api.req.GetAfter() != "" {
		t.Fatalf("around: request = %v, %v", api.req, err)
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

	if err = messages.DeleteMany(context.Background(), "tok", []string{"m1", "m2"}, true); err != nil || !api.removal.GetForEveryone() || len(api.removal.GetIds()) != 2 {
		t.Fatalf("DeleteMany = %v, asked %v", err, api.removal)
	}
	if f := failure.From(messages.DeleteMany(context.Background(), "tok", nil, false)); f.Kind != failure.InvalidArgument || api.removal.GetForEveryone() {
		t.Fatalf("DeleteMany of none: %+v", f)
	}

	// An edit replaces the photos only when it says so.
	if _, err = messages.Edit(context.Background(), "tok", "m1", model.MessageEdit{Text: "x"}); err != nil || api.edit.GetReplacePhotos() || api.edit.GetText() != "x" {
		t.Fatalf("Edit of the words = %v, asked %v", err, api.edit)
	}
	edited, err := messages.Edit(context.Background(), "tok", "m1", model.MessageEdit{PhotoIDs: []string{"ph1"}, ReplacePhotos: true})
	if err != nil || !api.edit.GetReplacePhotos() || len(edited.PhotoIDs) != 1 || edited.PhotoIDs[0] != "ph1" {
		t.Fatalf("Edit of the photos = %+v, %v", edited, err)
	}

	dialogs, err := messages.Dialogs(context.Background(), "tok", "", 0)
	if err != nil || len(dialogs.Dialogs) != 1 || !dialogs.Dialogs[0].Unread || dialogs.Dialogs[0].LastMessage.Text != "ау" ||
		!dialogs.Dialogs[0].LastMessage.TextCut {
		t.Fatalf("dialogs = %+v, %v", dialogs, err)
	}

	_, err = messages.Send(context.Background(), "tok", "u1", model.MessageDraft{Text: "x"})
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

func TestAuthSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeAuthAPI{}
	auth := NewAuth(api)

	// What the browser calls itself is the client's word: gRPC metadata
	// takes only printable ASCII of it, and auth keeps 255 bytes.
	agent := "Mozilla/5.0 (тест)\r\n " + strings.Repeat("x", 300)
	if _, err := auth.VerifyCode(ctx, "ann@example.com", "123456", model.Client{IP: "198.51.100.7", UserAgent: agent}); err != nil {
		t.Fatal(err)
	}
	if got := api.md.Get(authv1.ClientIPMetadata); len(got) != 1 || got[0] != "198.51.100.7" {
		t.Fatalf("client ip metadata = %v", got)
	}
	got := api.md.Get(authv1.UserAgentMetadata)
	if len(got) != 1 || len(got[0]) != 255 || !strings.HasPrefix(got[0], "Mozilla/5.0 () xxx") {
		t.Fatalf("user agent metadata = %q", got)
	}

	if _, err := auth.VerifyCode(ctx, "ann@example.com", "123456", model.Client{}); err != nil {
		t.Fatal(err)
	}
	if len(api.md.Get(authv1.ClientIPMetadata))+len(api.md.Get(authv1.UserAgentMetadata)) != 0 {
		t.Fatalf("an unknown client is stated: %v", api.md)
	}

	sessions, err := auth.ListSessions(ctx, "tok")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("ListSessions = %v, %v", sessions, err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}
	used := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	want := model.Session{ID: "s1", CreatedAt: used.Add(-time.Hour), LastUsedAt: used, IP: "203.0.113.7", UserAgent: "Mozilla/5.0", Current: true}
	if sessions[0] != want {
		t.Fatalf("session = %+v", sessions[0])
	}

	if err = auth.EndSession(ctx, "tok", "s1"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if err = auth.EndSession(ctx, "tok", "s2"); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("EndSession of someone else's: %v", err)
	}

	if n, err := auth.EndOtherSessions(ctx, "tok"); err != nil || n != 3 {
		t.Fatalf("EndOtherSessions = %d, %v", n, err)
	}
}

// fakeNoteAPI is the notes side of the post service.
type fakeNoteAPI struct {
	postv1.PostServiceClient
	md        metadata.MD
	textLimit int32
}

func (f *fakeNoteAPI) ListNotes(ctx context.Context, req *postv1.ListNotesRequest, _ ...grpc.CallOption) (*postv1.ListNotesResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.textLimit = req.GetTextLimit()

	at := timestamppb.New(time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC))

	return &postv1.ListNotesResponse{
		Notes: []*postv1.Note{
			{Id: "n1", AuthorId: req.GetAuthorId(), Title: "Про котов", Text: "Коты", TextCut: true, CommentCount: 2, CreatedAt: at, EditedAt: at, Mine: true},
			{Id: "n0", AuthorId: req.GetAuthorId(), Title: "Старая", Text: "Текст", CreatedAt: at},
		},
		NextCursor: "n0", Total: 5,
	}, nil
}

func (f *fakeNoteAPI) GetNote(context.Context, *postv1.GetNoteRequest, ...grpc.CallOption) (*postv1.GetNoteResponse, error) {
	return nil, status.Error(codes.NotFound, "note not found")
}

func TestNotes(t *testing.T) {
	t.Parallel()

	api := &fakeNoteAPI{}
	notes := NewNotes(api)

	page, err := notes.List(context.Background(), "tok", "u1", "", 2)
	if err != nil || len(page.Notes) != 2 || page.NextCursor != "n0" || page.Total != 5 {
		t.Fatalf("List = %+v, %v", page, err)
	}
	if api.md.Get("authorization")[0] != "Bearer tok" || api.textLimit != model.ExcerptLength {
		t.Fatalf("token %v, text limit %d", api.md, api.textLimit)
	}
	if n := page.Notes[0]; n.Title != "Про котов" || !n.TextCut || n.Comments != 2 || !n.Mine || n.EditedAt.IsZero() || n.AuthorID != "u1" {
		t.Fatalf("note = %+v", n)
	}
	if !page.Notes[1].EditedAt.IsZero() {
		t.Fatalf("a note never changed is edited at %v", page.Notes[1].EditedAt)
	}

	if _, err = notes.Get(context.Background(), "", "nope"); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("Get of no note: %v", err)
	}
}

// fakeFaveAPI is the faves side of the post service.
type fakeFaveAPI struct {
	postv1.PostServiceClient
	md        metadata.MD
	cursor    string
	limit     int32
	textLimit int32
}

func (f *fakeFaveAPI) ask(ctx context.Context, cursor string, limit, textLimit int32) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.cursor, f.limit, f.textLimit = cursor, limit, textLimit
}

func (f *fakeFaveAPI) ListLikedPosts(ctx context.Context, req *postv1.ListLikedPostsRequest, _ ...grpc.CallOption) (*postv1.ListLikedPostsResponse, error) {
	f.ask(ctx, req.GetCursor(), req.GetLimit(), req.GetTextLimit())

	return &postv1.ListLikedPostsResponse{Posts: []*postv1.Post{{Id: "p1", AuthorId: "u2", WallId: "u2", Text: "раз", Liked: true}}, NextCursor: "1_p1", Total: 4}, nil
}

func (f *fakeFaveAPI) ListOwnPosts(ctx context.Context, req *postv1.ListOwnPostsRequest, _ ...grpc.CallOption) (*postv1.ListOwnPostsResponse, error) {
	f.ask(ctx, req.GetCursor(), req.GetLimit(), req.GetTextLimit())

	return &postv1.ListOwnPostsResponse{Posts: []*postv1.Post{{Id: "p2", AuthorId: "u1", WallId: "u2", Mine: true}}, Total: 1}, nil
}

func (f *fakeFaveAPI) ListLikedComments(ctx context.Context, req *postv1.ListLikedCommentsRequest, _ ...grpc.CallOption) (*postv1.ListLikedCommentsResponse, error) {
	f.ask(ctx, req.GetCursor(), req.GetLimit(), req.GetTextLimit())

	return &postv1.ListLikedCommentsResponse{Comments: []*postv1.Comment{{Id: "c1", PostId: "p1", AuthorId: "u2", Text: "да", TextCut: true, Liked: true}}, Total: 1}, nil
}

func (f *fakeFaveAPI) ListLikedAlbumPhotos(ctx context.Context, req *postv1.ListLikedAlbumPhotosRequest, _ ...grpc.CallOption) (*postv1.ListLikedAlbumPhotosResponse, error) {
	if req.GetCursor() == "bad" {
		return nil, status.Error(codes.InvalidArgument, "invalid cursor")
	}

	f.ask(ctx, req.GetCursor(), req.GetLimit(), 0)

	return &postv1.ListLikedAlbumPhotosResponse{
		Photos:   []*postv1.AlbumPhoto{{Id: "f1", AlbumId: "a1", AuthorId: "u2", Liked: true}, {Id: "f2", AlbumId: "a2", AuthorId: "u3", Liked: true}},
		OwnerIds: []string{"u2", "club1"}, NextCursor: "1_f2", Total: 9,
	}, nil
}

func TestFaves(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeFaveAPI{}
	faves := NewFaves(api)

	posts, err := faves.Posts(ctx, "tok", "1_p9", 5)
	if err != nil || len(posts.Posts) != 1 || posts.NextCursor != "1_p1" || posts.Total != 4 || !posts.Posts[0].Liked {
		t.Fatalf("Posts = %+v, %v", posts, err)
	}
	if api.md.Get("authorization")[0] != "Bearer tok" || api.cursor != "1_p9" || api.limit != 5 || api.textLimit != model.ExcerptLength {
		t.Fatalf("request = %+v", api)
	}

	own, err := faves.OwnPosts(ctx, "tok", "", 0)
	if err != nil || len(own.Posts) != 1 || own.Total != 1 || !own.Posts[0].Mine || own.Posts[0].Wall.ID != "u2" || api.textLimit != model.ExcerptLength {
		t.Fatalf("OwnPosts = %+v, %v", own, err)
	}

	comments, err := faves.Comments(ctx, "tok", "", 0)
	if err != nil || len(comments.Comments) != 1 || comments.Total != 1 || api.textLimit != model.ExcerptLength {
		t.Fatalf("Comments = %+v, %v", comments, err)
	}
	if c := comments.Comments[0]; c.ID != "c1" || c.PostID != "p1" || !c.TextCut || !c.Liked {
		t.Fatalf("comment = %+v", c)
	}

	photos, err := faves.Photos(ctx, "tok", "", 0)
	if err != nil || len(photos.Photos) != 2 || photos.NextCursor != "1_f2" || photos.Total != 9 {
		t.Fatalf("Photos = %+v, %v", photos, err)
	}
	if p := photos.Photos[1]; p.ID != "f2" || p.AuthorID != "u3" || p.OwnerID != "club1" || !p.Liked {
		t.Fatalf("photo = %+v", p)
	}

	if _, err = faves.Photos(ctx, "tok", "bad", 0); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("a bad cursor: %v", err)
	}
}

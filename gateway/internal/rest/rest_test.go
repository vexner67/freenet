package rest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
	"github.com/vexner67/freenet/pkg/reqid"
)

type fakeAuth struct {
	clientIP  string
	requestID string
	err       error
	// client is where the last sign-in or refresh came from.
	client model.Client
	// ended are the sessions EndSession was asked to end.
	ended []string
}

func (f *fakeAuth) RequestCode(ctx context.Context, _, clientIP string) error {
	f.clientIP = clientIP
	f.requestID = reqid.FromContext(ctx)
	return f.err
}

func (f *fakeAuth) VerifyCode(_ context.Context, email, code string, client model.Client) (model.Tokens, error) {
	f.client = client
	if code != "123456" {
		return model.Tokens{}, failure.New(failure.Unauthenticated, "invalid or expired code")
	}
	return model.Tokens{AccessToken: "access-" + email, RefreshToken: "refresh"}, nil
}

func (f *fakeAuth) RefreshToken(_ context.Context, _ string, client model.Client) (model.Tokens, error) {
	f.client = client
	return model.Tokens{AccessToken: "a2", RefreshToken: "r2"}, f.err
}

// ListSessions: ann is signed in on a laptop, which asks, and on a phone.
func (f *fakeAuth) ListSessions(context.Context, string) ([]model.Session, error) {
	signedIn := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	return []model.Session{
		{ID: "s-laptop", CreatedAt: signedIn, LastUsedAt: signedIn.Add(time.Hour), IP: "203.0.113.9", UserAgent: "Mozilla/5.0 (laptop)", Current: true},
		{ID: "s-phone", CreatedAt: signedIn.Add(-time.Hour), LastUsedAt: signedIn},
	}, f.err
}

func (f *fakeAuth) EndSession(_ context.Context, _, sessionID string) error {
	if sessionID != "s-laptop" && sessionID != "s-phone" {
		return failure.New(failure.NotFound, "session not found")
	}

	f.ended = append(f.ended, sessionID)
	return nil
}

func (f *fakeAuth) EndOtherSessions(context.Context, string) (int, error) {
	return 1, f.err
}

func (f *fakeAuth) Logout(context.Context, string) error { return f.err }

type fakeUsers struct {
	// named are the nicknames the gateway asked ResolveMentions about.
	named []string
	// uninvitable is the nickname of who lets nobody add them to chats.
	uninvitable string
	// blockedIDs are whom the caller blacklisted, as BlockedIDs tells.
	blockedIDs []string
	// ignoredIDs are the walls the caller's news leave out, as IgnoredIDs
	// tells; ignored and unignored are what the gateway asked to change.
	ignoredIDs []string
	ignored    string
	unignored  [][]string
	// ui is how the site looks to the caller, as GetMe tells.
	ui                                              model.Interface
	token, statsToken, followed, unfollowed, listed string
	profileIDs                                      []string
	profileCalls                                    int
	avatarID, query, cursor                         string
	profile                                         model.Profile
	wallAsked                                       string
	closedWall                                      bool
	// Privacy: the parts of every page hidden from the caller; accessAsked
	// is whose page the gateway asked about, profileToken the token
	// GetProfile got.
	hiddenPage, hiddenInfo, hiddenPhotos, hiddenFriends, hiddenClubs, hiddenNotes, hiddenGifts bool
	// closedPage: the hidden page is a closed one ("Закрытая страница").
	closedPage                bool
	accessAsked, profileToken string
	// Messages: closed unless the peer wrote first; wroteFirst is what the
	// gateway passed on.
	closedMessages, wroteFirst bool
	// Blacklists: relation is the same with everyone.
	relation                         model.Relation
	blocked, unblocked, relationWith string
	// deactivated: the caller's page is deleted, with this reason.
	deactivated bool
	reason      string
	// partner is the nickname UpdateProfile got; about the rest of the
	// page, and search the last query.
	partner  string
	about    model.About
	search   model.UserQuery
	backdrop model.Backdrop
	// onlineTokens are the tokens MarkOnline got.
	onlineMu     sync.Mutex
	onlineTokens []string
	err          error
}

func (f *fakeUsers) user() model.User {
	u := model.User{ID: "u1", Email: "ann@example.com", Profile: f.profile, ProfileCompleted: f.profile.Nickname != "", About: f.about, Interface: f.ui}
	if f.deactivated {
		u.Deactivated, u.PurgeAt = true, time.Date(2027, 5, 7, 12, 0, 0, 0, time.UTC)
	}
	return u
}

func (f *fakeUsers) Deactivate(_ context.Context, token, reason string) (model.User, error) {
	f.token, f.reason, f.deactivated = token, reason, f.err == nil
	return f.user(), f.err
}

func (f *fakeUsers) Reactivate(_ context.Context, token string) (model.User, error) {
	f.token, f.deactivated = token, false
	return f.user(), f.err
}

func (f *fakeUsers) GetMe(_ context.Context, token string) (model.User, error) {
	f.token = token
	return f.user(), f.err
}

func (f *fakeUsers) UpdateProfile(_ context.Context, token string, p model.Profile, partner string) (model.User, error) {
	f.token, f.profile, f.partner = token, p, partner
	if partner == "bob" {
		f.profile.PartnerID = "u2"
	}
	return f.user(), f.err
}

func (f *fakeUsers) UpdateContacts(_ context.Context, token string, c model.Contacts) (model.User, error) {
	f.token, f.about.Contacts = token, c
	return f.user(), f.err
}

func (f *fakeUsers) UpdateInterests(_ context.Context, token string, in model.Interests) (model.User, error) {
	f.token, f.about.Interests = token, in
	return f.user(), f.err
}

func (f *fakeUsers) UpdateExtraFields(_ context.Context, token string, fields []model.ExtraField) (model.User, error) {
	f.token, f.about.ExtraFields = token, fields
	return f.user(), f.err
}

func (f *fakeUsers) UpdatePrivacy(_ context.Context, token string, p model.Privacy) (model.User, error) {
	f.token = token
	u := f.user()
	u.Privacy = p
	return u, f.err
}

func (f *fakeUsers) GetProfile(_ context.Context, token, nickname string) (model.PublicProfile, error) {
	f.profileToken = token
	p, err := f.profileOf(nickname)
	if err == nil && f.hiddenPage {
		return model.PublicProfile{ID: p.ID, Profile: model.Profile{Name: p.Profile.Name, Nickname: p.Profile.Nickname}, PageHidden: true, Closed: f.closedPage}, nil
	}
	if err == nil && f.hiddenInfo {
		p.InfoHidden, p.About = true, model.About{}
	}
	return p, err
}

func (f *fakeUsers) profileOf(nickname string) (model.PublicProfile, error) {
	switch nickname {
	case "ann":
		return model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann", Worldview: "buddhism"}}, nil
	case "bob":
		return model.PublicProfile{
			ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob"},
			LastSeen: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC), Online: true,
			Backdrop: model.Backdrop{Right: "pb2"},
		}, nil
	case "cat":
		return model.PublicProfile{ID: "u3", Profile: model.Profile{Name: "Cat", Nickname: "cat"}, Deactivated: true}, nil
	case "dina":
		return model.PublicProfile{
			ID: "u4", Profile: model.Profile{Name: "Dina", Nickname: "dina", Birthday: time.Date(2000, 5, 17, 0, 0, 0, 0, time.UTC), PartnerID: "u2"},
			BirthYearHidden: true,
			About: model.About{
				Contacts: model.Contacts{City: "Москва"}, RegisteredAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
				ExtraFields: []model.ExtraField{{Name: "Цвет", Text: "синий", Place: "interests"}},
			},
		}, nil
	default:
		return model.PublicProfile{}, failure.New(failure.NotFound, "user not found")
	}
}

// GetProfiles knows u1, u2, u3 (a deleted page) and anyone whose ID starts
// with "x", and takes at most 100 IDs, as the user service.
func (f *fakeUsers) GetProfiles(_ context.Context, ids []string) ([]model.PublicProfile, error) {
	f.profileIDs = ids
	f.profileCalls++
	if len(ids) > 100 {
		return nil, failure.New(failure.InvalidArgument, "too many user ids")
	}
	var found []model.PublicProfile
	for _, id := range ids {
		if strings.HasPrefix(id, "x") {
			found = append(found, model.PublicProfile{ID: id, Profile: model.Profile{Name: id, Nickname: id}})
		}
		if id == "u2" {
			found = append(found, model.PublicProfile{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob", Gender: "male"}})
		}
		if id == "u1" {
			found = append(found, model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}})
		}
		if id == "u3" {
			found = append(found, model.PublicProfile{ID: "u3", Profile: model.Profile{Name: "Cat", Nickname: "cat"}, Deactivated: true})
		}
	}
	return found, nil
}

// CanPostOnWall: every wall is open but when closedWall.
func (f *fakeUsers) CanPostOnWall(_ context.Context, token, userID string) (bool, error) {
	f.token, f.wallAsked = token, userID
	return !f.closedWall, f.err
}

func (f *fakeUsers) Access(_ context.Context, token, userID string) (model.Access, error) {
	f.token, f.accessAsked = token, userID
	if f.hiddenPage {
		return model.Access{}, f.err
	}
	return model.Access{
		Page: !f.hiddenPage, Info: !f.hiddenInfo, Photos: !f.hiddenPhotos, Friends: !f.hiddenFriends, Clubs: !f.hiddenClubs, Notes: !f.hiddenNotes, Gifts: !f.hiddenGifts,
	}, f.err
}

func (f *fakeUsers) CanMessage(_ context.Context, token, _ string, wroteFirst bool) (bool, error) {
	f.token, f.wroteFirst = token, wroteFirst
	return !f.relation.Blocked() && (!f.closedMessages || wroteFirst), f.err
}

// KnownNicknames knows ann and bob.
func (f *fakeUsers) KnownNicknames(_ context.Context, nicknames []string) ([]string, error) {
	var known []string
	for _, n := range nicknames {
		if l := strings.ToLower(n); l == "ann" || l == "bob" {
			known = append(known, l)
		}
	}

	return known, f.err
}

// ResolveInvites knows bob (u2) and cat (u3), who let the caller add them
// unless uninvitable.
func (f *fakeUsers) ResolveInvites(_ context.Context, _ string, nicknames []string) ([]model.Invitee, error) {
	var invitees []model.Invitee
	for _, n := range nicknames {
		n = strings.ToLower(n)
		if id, ok := map[string]string{"bob": "u2", "cat": "u3"}[n]; ok {
			invitees = append(invitees, model.Invitee{Nickname: n, UserID: id, Allowed: n != f.uninvitable})
		}
	}

	return invitees, f.err
}

// ResolveMentions knows ann (u1) and bob (u2), unless they are blocked.
func (f *fakeUsers) ResolveMentions(_ context.Context, _ string, nicknames []string) ([]string, error) {
	f.named = nicknames

	var ids []string
	for _, n := range nicknames {
		if id, ok := map[string]string{"ann": "u1", "bob": "u2"}[n]; ok && !f.relation.Blocked() {
			ids = append(ids, id)
		}
	}

	return ids, f.err
}

func (f *fakeUsers) Block(_ context.Context, token, userID string) error {
	f.token, f.blocked = token, userID
	return f.err
}

func (f *fakeUsers) Unblock(_ context.Context, token, userID string) error {
	f.token, f.unblocked = token, userID
	return f.err
}

func (f *fakeUsers) Blacklist(_ context.Context, token, cursor string, _ int) (model.UserPage, error) {
	f.token, f.cursor = token, cursor
	return model.UserPage{Users: []model.PublicProfile{{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob"}}}, Total: 1}, f.err
}

func (f *fakeUsers) Relation(_ context.Context, token, userID string) (model.Relation, error) {
	f.token, f.relationWith = token, userID
	return f.relation, f.err
}

func (f *fakeUsers) Follow(_ context.Context, token, userID string) error {
	f.token, f.followed = token, userID
	return f.err
}

func (f *fakeUsers) Unfollow(_ context.Context, token, userID string) error {
	f.token, f.unfollowed = token, userID
	return f.err
}

func (f *fakeUsers) SetAvatar(_ context.Context, token, avatarID string) (model.User, error) {
	f.token, f.avatarID = token, avatarID
	u := f.user()
	u.AvatarID = avatarID
	return u, f.err
}

func (f *fakeUsers) Search(_ context.Context, q model.UserQuery, cursor string, _ int) (model.UserPage, error) {
	f.query, f.cursor, f.search = q.Text, cursor, q
	if q == (model.UserQuery{}) {
		return model.UserPage{}, failure.New(failure.InvalidArgument, "search query is empty")
	}
	return model.UserPage{
		Users:      []model.PublicProfile{{ID: "u1", AvatarID: "a1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}}},
		NextCursor: "u1",
	}, nil
}

// BlockedIDs is the blacklist the news leave out, set by a test.
func (f *fakeUsers) BlockedIDs(context.Context, string) ([]string, error) {
	return f.blockedIDs, f.err
}

func (f *fakeUsers) IgnoredIDs(context.Context, string) ([]string, error) {
	return f.ignoredIDs, f.err
}

func (f *fakeUsers) Ignore(_ context.Context, token, sourceID string) error {
	f.token, f.ignored = token, sourceID
	return f.err
}

func (f *fakeUsers) Unignore(_ context.Context, token string, sourceIDs []string) error {
	f.token, f.unignored = token, append(f.unignored, sourceIDs)
	return f.err
}

func (f *fakeUsers) FollowStats(_ context.Context, token, _ string) (model.FollowStats, error) {
	f.statsToken = token
	return model.FollowStats{Followers: 3, Following: 1, IsFollowing: token != ""}, nil
}

func (f *fakeUsers) ListFollowers(_ context.Context, userID, cursor string, _ int) (model.UserPage, error) {
	f.listed = "followers:" + userID + ":" + cursor
	return model.UserPage{Users: []model.PublicProfile{{ID: "u2", Profile: model.Profile{Nickname: "bob"}}}, Total: 1}, nil
}

func (f *fakeUsers) ListFollowing(_ context.Context, userID, cursor string, _ int) (model.UserPage, error) {
	f.listed = "following:" + userID + ":" + cursor
	return model.UserPage{NextCursor: "c2", Total: 5}, nil
}

func (f *fakeUsers) ListCommonFollowing(_ context.Context, token, userID, cursor string, _ int) (model.UserPage, error) {
	f.token, f.listed = token, "common:"+userID+":"+cursor
	return model.UserPage{Users: []model.PublicProfile{{ID: "u3", Profile: model.Profile{Nickname: "cat"}}}, Total: 1}, nil
}

// UpdateStatus keeps the status in the caller's profile.
func (f *fakeUsers) UpdateInterface(_ context.Context, token string, ui model.Interface) (model.User, error) {
	f.token = token
	if f.err != nil {
		return model.User{}, f.err
	}
	u := f.user()
	u.Interface = ui

	return u, nil
}

func (f *fakeUsers) UpdateNickname(_ context.Context, token, nickname string) (model.User, error) {
	f.token = token
	if f.err != nil {
		return model.User{}, f.err
	}
	f.profile.Nickname = nickname

	return f.user(), nil
}

func (f *fakeUsers) UpdateStatus(_ context.Context, token, status string) (model.User, error) {
	f.token = token
	if f.err != nil {
		return model.User{}, f.err
	}
	f.profile.Status = status
	u := f.user()
	u.Completeness, u.Unfilled = 85, []string{"telegram"}
	return u, nil
}

// SetBackdrop keeps the caller's backdrop in backdrop.
func (f *fakeUsers) SetBackdrop(_ context.Context, token, side, photoID string) (model.User, string, error) {
	f.token = token
	if f.err != nil {
		return model.User{}, "", f.err
	}

	previous := &f.backdrop.Left
	if side == "right" {
		previous = &f.backdrop.Right
	}
	old := *previous
	*previous = photoID

	u := f.user()
	u.Backdrop = f.backdrop
	return u, old, nil
}

func (f *fakeUsers) ListOnlineFollowing(_ context.Context, userID, cursor string, _ int) (model.UserPage, error) {
	f.listed = "online:" + userID + ":" + cursor
	return model.UserPage{Users: []model.PublicProfile{{ID: "u2", Profile: model.Profile{Nickname: "bob"}}}, Total: 1}, nil
}

func (f *fakeUsers) Statistics(context.Context) (int, int, error) {
	return 12, 3, f.err
}

// MarkOnline counts the visits the event stream tells, from its goroutine.
func (f *fakeUsers) MarkOnline(_ context.Context, token string) error {
	f.onlineMu.Lock()
	defer f.onlineMu.Unlock()
	f.onlineTokens = append(f.onlineTokens, token)
	return nil
}

func (f *fakeUsers) markedOnline() []string {
	f.onlineMu.Lock()
	defer f.onlineMu.Unlock()
	return slices.Clone(f.onlineTokens)
}

type fakePosts struct {
	// manyToken is the token of the last GetMany.
	manyToken string
	lastAsked []string
	feedMore  []model.Post
	hideNSFW  bool
	// hidden are the authors the last feed was asked to leave out.
	hidden   []string
	everyone bool
	// ownOnly and ignored: the feed settings the gateway passed on.
	ownOnly                bool
	ignored                []string
	newPost                model.NewPost
	token, cursor, deleted string
	limit                  int
	wallPosts              []model.Post
	// signedForClub: the post for the club k1 names its author.
	signedForClub         bool
	created, wall, shared string
	// mentioned are the people the last edit named.
	mentioned      []string
	edited, pinned string
	// archived and closed are the post switched last, "-" before its ID
	// when switched back; wallQuery what the wall was searched for.
	archived, closed, wallQuery string
	// archivedWall is the wall, the year and the way of the last
	// ArchiveWall; yearsOf the wall whose years were asked for.
	archivedWall, yearsOf string
	wallYear              int
	replyTo, likedComment string
	photos                []string
	liked                 map[string]bool
	newComment            model.NewComment
	// Suggestions: suggested is the last one, mine whether only the
	// caller's were asked for; published and withdrawn are IDs.
	suggested            model.Suggestion
	mine                 bool
	published, withdrawn string
	signed               bool
	err                  error
	// wallFilter is the tab of the wall List got.
	wallFilter string
	// Postponed posts: what Postpone and EditPostponed got, whose wall
	// was listed, and the IDs published and deleted.
	postponed                           model.NewPostponed
	postponedEdit                       model.PostponedEdit
	postponedOf, publishedPP, deletedPP string
	// likersOf is whose likers were asked for.
	likersOf string
	// repost is how Repost was asked to share.
	repost model.NewRepost
	// polled: p1 asks a poll; pollsAsked are the posts whose polls were
	// asked for, a request after another; voted what Vote chose of
	// votedIn ("-" before the post of a vote taken back).
	polled     bool
	pollsAsked [][]string
	voted      []int
	votedIn    string
}

// Polls: every post asked about has a poll, in which the caller chose the
// second option.
func (f *fakePosts) Polls(_ context.Context, token string, postIDs []string) ([]model.Poll, error) {
	f.pollsAsked = append(f.pollsAsked, postIDs)

	polls := make([]model.Poll, 0, len(postIDs))
	for _, id := range postIDs {
		polls = append(polls, fakePoll(id, token != ""))
	}

	return polls, f.err
}

func fakePoll(postID string, voted bool) model.Poll {
	p := model.Poll{
		PostID:  postID,
		Title:   "Кошки или собаки?",
		Options: []model.PollOption{{Name: "кошки", Votes: 2}, {Name: "собаки", Votes: 1}},
		Voters:  3,
		Locked:  true,
		EndsAt:  time.Date(2026, 10, 6, 15, 30, 0, 0, time.UTC),
	}
	if voted {
		p.Chosen = []int{1}
	}

	return p
}

func (f *fakePosts) Vote(_ context.Context, token, postID string, options []int) (model.Poll, error) {
	f.token, f.votedIn, f.voted = token, postID, options
	return fakePoll(postID, true), f.err
}

func (f *fakePosts) RetractVote(_ context.Context, token, postID string) (model.Poll, error) {
	f.token, f.votedIn = token, "-"+postID
	return fakePoll(postID, false), f.err
}

// PollVoters: u2 and u3 (a deleted page) chose every option.
func (f *fakePosts) PollVoters(_ context.Context, postID string, option int, cursor string, limit int) (model.LikerPage, error) {
	f.cursor, f.limit, f.likersOf = cursor, limit, "poll:"+postID+":"+strconv.Itoa(option)
	return model.LikerPage{UserIDs: []string{"u3", "u2"}, NextCursor: "v2", Total: 2}, f.err
}

// Suggest keeps one suggestion by u1.
func (f *fakePosts) Suggest(_ context.Context, token, wallID, text string, photoIDs []string) (model.Suggestion, error) {
	f.token = token
	f.suggested = model.Suggestion{ID: "s1", WallID: wallID, AuthorID: "u1", Text: text, PhotoIDs: photoIDs, CreatedAt: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	return f.suggested, f.err
}

// Suggestions are one by Bob and one by Cat, whose page is deleted.
func (f *fakePosts) Suggestions(_ context.Context, token, wallID string, mine bool, cursor string, limit int) (model.SuggestionPage, error) {
	f.token, f.mine = token, mine
	return model.SuggestionPage{
		Suggestions: []model.Suggestion{
			{ID: "s2", WallID: wallID, AuthorID: "u2", Text: "от Петра", PhotoIDs: []string{"ph2"}},
			{ID: "s3", WallID: wallID, AuthorID: "u3", Text: "от Кати"},
		},
		NextCursor: "s3",
		Total:      2,
	}, f.err
}

// PublishSuggestion makes a post by Bob for the club k1.
func (f *fakePosts) PublishSuggestion(_ context.Context, token, id, text string, signed bool) (model.Post, error) {
	f.token, f.published, f.signed = token, id, signed
	return model.Post{ID: "p8", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "k1"}, Text: text, FromWall: true, Signed: signed}, f.err
}

func (f *fakePosts) DeleteSuggestion(_ context.Context, token, id string) error {
	f.token, f.withdrawn = token, id
	return f.err
}

// Postpone keeps one postponed post by u1.
func (f *fakePosts) Postpone(_ context.Context, token string, in model.NewPostponed) (model.Postponed, error) {
	f.token, f.postponed = token, in
	wallID := in.WallID
	if wallID == "" {
		wallID = "u1"
	}

	return model.Postponed{
		ID: "pp1", WallID: wallID, AuthorID: "u1", Text: in.Text, PhotoIDs: in.PhotoIDs, FromWall: in.FromWall, Signed: in.Signed,
		NSFW: in.NSFW, PublishAt: in.PublishAt, CreatedAt: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
	}, f.err
}

// PostponedPosts are one by Bob and one by Cat, whose page is deleted.
func (f *fakePosts) PostponedPosts(_ context.Context, token, wallID string) ([]model.Postponed, error) {
	f.token, f.postponedOf = token, wallID
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

	return []model.Postponed{
		{ID: "pp2", WallID: wallID, AuthorID: "u2", Text: "от Петра", TextCut: true, PublishAt: at},
		{ID: "pp3", WallID: wallID, AuthorID: "u3", Text: "от Кати", PublishAt: at},
	}, f.err
}

// PostponedPost knows pp2, by Bob.
func (f *fakePosts) PostponedPost(_ context.Context, token, id string) (model.Postponed, error) {
	f.token = token
	if id != "pp2" {
		return model.Postponed{}, failure.New(failure.NotFound, "postponed post not found")
	}

	return model.Postponed{ID: id, WallID: "u2", AuthorID: "u2", Text: "от Петра целиком", PublishAt: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakePosts) EditPostponed(ctx context.Context, token, id string, edit model.PostponedEdit) (model.Postponed, error) {
	p, err := f.PostponedPost(ctx, token, id)
	if err != nil {
		return model.Postponed{}, err
	}

	f.postponedEdit = edit
	p.Text, p.NSFW, p.PublishAt = edit.Text, edit.NSFW, edit.PublishAt

	return p, f.err
}

// PublishPostponed makes a post by Bob on his wall.
func (f *fakePosts) PublishPostponed(_ context.Context, token, id string) (model.Post, error) {
	f.token, f.publishedPP = token, id
	return model.Post{ID: "p8", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u2"}, Text: "от Петра", Mine: true}, f.err
}

func (f *fakePosts) DeletePostponed(_ context.Context, token, id string) error {
	f.token, f.deletedPP = token, id
	return f.err
}

func (f *fakePosts) Create(_ context.Context, token string, np model.NewPost) (model.Post, error) {
	f.token, f.wall, f.created, f.photos, f.newPost = token, np.WallID, np.Text, np.PhotoIDs, np
	return model.Post{ID: "p9", Text: np.Text, PhotoIDs: np.PhotoIDs, FromWall: np.FromWall, Signed: np.Signed, NSFW: np.NSFW, Mine: true, Polled: np.Poll != nil, DocumentIDs: np.DocumentIDs, CreatedAt: time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)}, f.err
}

// Edit edits the posts Get knows.
func (f *fakePosts) Edit(ctx context.Context, token, id string, edit model.PostEdit) (model.Post, error) {
	text, photoIDs := edit.Text, edit.PhotoIDs

	p, err := f.Get(ctx, token, id)
	if err != nil {
		return model.Post{}, err
	}

	f.edited, f.photos, f.mentioned = text, photoIDs, edit.Mentions
	p.Text, p.PhotoIDs, p.EditedAt = text, photoIDs, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

	return p, f.err
}

// Pin pins the posts Get knows; Unpin is "-" before the ID.
func (f *fakePosts) Pin(ctx context.Context, token, id string) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.pinned = id
	return f.err
}

func (f *fakePosts) Unpin(ctx context.Context, token, id string) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.pinned = "-" + id
	return f.err
}

// Repost shares p1 only, as u1's repost p7.
func (f *fakePosts) Repost(_ context.Context, token, postID string, nr model.NewRepost) (model.Post, error) {
	f.token, f.shared, f.created, f.repost = token, postID, nr.Text, nr
	if postID != "p1" && nr.PhotoID == "" {
		return model.Post{}, failure.New(failure.NotFound, "post not found")
	}
	ann, wall := model.Author{ID: "u1"}, model.Author{ID: "u1"}
	if nr.WallID != "" {
		wall = model.Author{ID: nr.WallID}
	}
	return model.Post{
		ID: "p7", Author: ann, Wall: wall, Text: nr.Text, PhotoIDs: nr.PhotoIDs, RepostOf: "p1", FromWall: nr.FromWall,
		CreatedAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
	}, f.err
}

// Reposts of p1: bob's, one of a vanished page and one for the club k1.
func (f *fakePosts) Reposts(_ context.Context, token, postID, cursor string, _ int) (model.PostPage, error) {
	f.token, f.cursor = token, cursor
	if postID != "p1" {
		return model.PostPage{}, failure.New(failure.NotFound, "post not found")
	}

	return model.PostPage{
		Posts: []model.Post{
			{ID: "r3", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "k1"}, RepostOf: "p1", FromWall: true},
			{ID: "r2", Author: model.Author{ID: "gone"}, Wall: model.Author{ID: "gone"}, RepostOf: "p1"},
			{ID: "r1", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u2"}, Text: "смотрите", RepostOf: "p1"},
		},
		NextCursor: "r1",
		Total:      3,
	}, f.err
}

// Delete knows the posts Get knows.
func (f *fakePosts) Delete(ctx context.Context, token, id string) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.token, f.deleted = token, id
	return f.err
}

// Get knows p1 by u1, p2 by an author who deleted the account, p4 by u2
// on u1's wall and p8, u2's repost of a deleted post.
func (f *fakePosts) Get(_ context.Context, token, id string) (model.Post, error) {
	f.token = token
	switch id {
	case "p1":
		return model.Post{ID: "p1", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u1"}, Text: "первая", PhotoIDs: []string{"ph1"}, Likes: 3, Comments: 2, Liked: token != "", Polled: f.polled}, nil
	case "p2":
		return model.Post{ID: "p2", Author: model.Author{ID: "gone"}, Wall: model.Author{ID: "gone"}, Text: "сирота"}, nil
	case "p4":
		return model.Post{ID: "p4", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u1"}, Text: "в гостях"}, nil
	case "p8":
		return model.Post{ID: "p8", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u2"}, RepostOf: "p-deleted"}, nil
	case "p5":
		return model.Post{ID: "p5", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "k1"}, Text: "в группе"}, nil
	default:
		return model.Post{}, failure.New(failure.NotFound, "post not found")
	}
}

// GetMany knows p1 only.
func (f *fakePosts) Count(context.Context) (int, error) {
	return 40, f.err
}

func (f *fakePosts) GetMany(_ context.Context, token string, ids []string) ([]model.Post, error) {
	f.manyToken = token
	var found []model.Post
	for _, id := range ids {
		if id == "p1" {
			found = append(found, model.Post{ID: "p1", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u1"}, Text: "первая", PhotoIDs: []string{"ph1"}, Reposts: 1, Polled: f.polled, CreatedAt: time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)})
		}
	}
	return found, nil
}

// Comment knows c1, a comment of p1, and c9, of a post nobody is shown.
func (f *fakePosts) Comment(_ context.Context, id string) (model.Comment, error) {
	switch id {
	case "c1":
		return model.Comment{ID: "c1", PostID: "p1", Author: model.Author{ID: "u2"}, Text: "согласен, и вот почему"}, nil
	case "c9":
		return model.Comment{ID: "c9", PostID: "p404", Author: model.Author{ID: "u2"}, Text: "в архиве"}, nil
	default:
		return model.Comment{}, failure.New(failure.NotFound, "comment not found")
	}
}

// CommentsByID knows c1 only.
func (f *fakePosts) CommentsByID(_ context.Context, ids []string) ([]model.Comment, error) {
	var found []model.Comment
	for _, id := range ids {
		if id == "c1" {
			found = append(found, model.Comment{ID: "c1", PostID: "p1", Author: model.Author{ID: "u2"}, Text: "согласен"})
		}
	}
	return found, nil
}

func (f *fakePosts) Feed(_ context.Context, token string, sources model.FeedSources, cursor string, _ int) (model.PostPage, error) {
	f.token, f.cursor, f.hideNSFW = token, cursor, sources.HideNSFW
	f.everyone, f.hidden = sources.Everyone, sources.Hidden
	f.ownOnly, f.ignored = sources.OwnOnly, sources.Ignored
	return model.PostPage{
		Posts: append([]model.Post{
			{ID: "p3", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u2"}, Text: "от боба", Likes: 1, Liked: true},
			{ID: "p2", Author: model.Author{ID: "gone"}, Wall: model.Author{ID: "gone"}, Text: "автор удалился"},
			{ID: "p5", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "gone"}, Text: "на исчезнувшей стене"},
			{ID: "p1", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u1"}, Text: "моя"},
		}, f.feedMore...),
		NextCursor: "p1",
	}, nil
}

// List is u1's wall: bob's post p4 on it and u1's own p2 and p1.
// PostLikers: x1 and u3 (a deleted page) like every post, the newest
// first.
func (f *fakePosts) PostLikers(_ context.Context, postID, cursor string, limit int) (model.LikerPage, error) {
	f.cursor, f.limit, f.likersOf = cursor, limit, postID
	return model.LikerPage{UserIDs: []string{"x1", "u3", "u2"}, NextCursor: "l2", Total: 4}, f.err
}

func (f *fakePosts) CommentLikers(_ context.Context, commentID, cursor string, limit int) (model.LikerPage, error) {
	f.cursor, f.limit, f.likersOf = cursor, limit, "comment:"+commentID
	return model.LikerPage{UserIDs: []string{"u2"}, Total: 1}, f.err
}

func (f *fakePosts) Archive(ctx context.Context, token, id string, archived bool) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.archived = map[bool]string{true: id, false: "-" + id}[archived]
	return f.err
}

func (f *fakePosts) ArchiveWall(_ context.Context, token, wallID string, year int, archived bool) (int, error) {
	f.token = token
	f.archivedWall = wallID + " " + strconv.Itoa(year) + " " + strconv.FormatBool(archived)

	return 3, f.err
}

func (f *fakePosts) WallYears(_ context.Context, _, wallID string) ([]int, error) {
	f.yearsOf = wallID

	return []int{2026, 2024}, f.err
}

func (f *fakePosts) CloseComments(ctx context.Context, token, id string, closed bool) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.closed = map[bool]string{true: id, false: "-" + id}[closed]
	return f.err
}

func (f *fakePosts) List(_ context.Context, token, wallID string, pick model.WallPick, cursor string, limit int) (model.PostPage, error) {
	filter := pick.Filter
	f.token, f.cursor, f.limit, f.wallFilter, f.wallQuery, f.wallYear = token, cursor, limit, filter, pick.Query, pick.Year
	if wallID == "k1" {
		ann, club := model.Author{ID: "u1"}, model.Author{ID: "k1"}
		return model.PostPage{Posts: []model.Post{
			{ID: "p8", Author: ann, Wall: club, Text: "новости", FromWall: true, Signed: f.signedForClub, PhotoIDs: []string{"ph1"}},
			{ID: "p7", Author: model.Author{ID: "u2"}, Wall: club, Text: "от боба"},
		}, Total: 2}, f.err
	}
	if wallID != "u1" {
		return model.PostPage{}, errors.New("wrong wall id")
	}
	if f.wallPosts != nil {
		return model.PostPage{Posts: f.wallPosts}, f.err
	}
	ann := model.Author{ID: "u1"}
	return model.PostPage{
		Posts: []model.Post{
			{ID: "p4", Author: model.Author{ID: "u2"}, Wall: ann, Text: "в гостях"},
			{ID: "p2", Author: ann, Wall: ann, Text: "вторая", Comments: 4},
			{ID: "p1", Author: ann, Wall: ann, Text: "первая", Likes: 2, Liked: true},
		},
		NextCursor: "p1",
		Total:      7,
	}, f.err
}

func (f *fakePosts) Like(_ context.Context, token, postID string) (int, error) {
	return f.setLike(token, postID, true)
}

func (f *fakePosts) Unlike(_ context.Context, token, postID string) (int, error) {
	return f.setLike(token, postID, false)
}

func (f *fakePosts) setLike(token, postID string, liked bool) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.liked == nil {
		f.liked = make(map[string]bool)
	}
	f.token, f.liked[postID] = token, liked

	likes := 0
	for _, l := range f.liked {
		if l {
			likes++
		}
	}
	return likes, nil
}

func (f *fakePosts) CreateComment(_ context.Context, token, postID string, c model.NewComment) (model.Comment, error) {
	f.token, f.created, f.photos, f.replyTo, f.newComment = token, c.Text, c.PhotoIDs, c.ReplyTo, c
	return model.Comment{
		ID: "c9", PostID: postID, Text: c.Text, PhotoIDs: c.PhotoIDs, ReplyTo: c.ReplyTo, CreatedAt: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC),
		FromWall: c.FromWall, Mine: true,
	}, f.err
}

// LikeComment knows c1, liked once before; Unlike takes the like back.
func (f *fakePosts) LikeComment(_ context.Context, token, id string) (int, error) {
	f.token, f.likedComment = token, id
	if id != "c1" {
		return 0, failure.New(failure.NotFound, "comment not found")
	}
	return 2, f.err
}

func (f *fakePosts) UnlikeComment(_ context.Context, token, id string) (int, error) {
	f.token, f.likedComment = token, "-"+id
	if id != "c1" {
		return 0, failure.New(failure.NotFound, "comment not found")
	}
	return 1, f.err
}

func (f *fakePosts) DeleteComment(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

// LastComments: p2 has two, one by an author who is gone; p8, a post of
// the club k1, one that u1 wrote for it.
func (f *fakePosts) LastComments(_ context.Context, token string, postIDs []string) ([]model.Comment, error) {
	f.lastAsked = postIDs

	var found []model.Comment
	for _, id := range postIDs {
		switch id {
		case "p2":
			found = append(found,
				model.Comment{ID: "c7", PostID: "p2", Author: model.Author{ID: "gone"}, Text: "автор удалился"},
				model.Comment{ID: "c8", PostID: "p2", Author: model.Author{ID: "u2"}, Text: "последний", Liked: token != ""})
		case "p8":
			found = append(found, model.Comment{ID: "c9", PostID: "p8", Author: model.Author{ID: "u1"}, Text: "от группы", FromWall: true})
		}
	}

	return found, f.err
}

func (f *fakePosts) Comments(_ context.Context, token, postID, cursor string, limit int) (model.CommentPage, error) {
	f.token, f.cursor, f.limit = token, cursor, limit
	if postID == "p5" {
		// u1 answers for the club k1, with a photo of theirs.
		return model.CommentPage{Comments: []model.Comment{
			{ID: "c5", PostID: "p5", Author: model.Author{ID: "u1"}, Text: "от группы", PhotoIDs: []string{"ph1"}, FromWall: true, Mine: token != ""},
		}, Total: 1}, nil
	}
	if postID != "p1" {
		return model.CommentPage{}, failure.New(failure.NotFound, "post not found")
	}
	return model.CommentPage{
		Comments: []model.Comment{
			{ID: "c1", PostID: "p1", Author: model.Author{ID: "u2"}, Text: "первый"},
			{ID: "c2", PostID: "p1", Author: model.Author{ID: "gone"}, Text: "автор удалился"},
			{ID: "c3", PostID: "p1", Author: model.Author{ID: "u1"}, Text: "ответ", ReplyTo: "c1", Likes: 2, Liked: token != ""},
		},
		NextCursor: "c3",
		Total:      5,
	}, nil
}

// fakeMedia: u1 has the photos ph1 and ph2.
type fakeMedia struct {
	uploaded []byte
	kept     string
	deleted  bool
	// clubAvatarOf is the club whose photo was uploaded or cleaned up.
	clubAvatarOf string
	// keptPhotos are the club's photos kept by the last clean-up.
	keptPhotos []string
	// published and unpublished are the photos kept for good and deleted.
	published, unpublished []string
	// taken are uploads something shows already; claimed and released
	// are the photos of each claim and release, "id,id".
	taken             []string
	claimed, released []string
	// releaseFails is how many releases fail before one goes through.
	releaseFails int
	// droppedDocuments are the documents deleted, "owner/id,id".
	droppedDocuments []string
	// chatPhotos are the uploads and clean-ups of chat photos, "upload
	// chat" and "keep chat photo".
	chatPhotos []string
	err        error
}

func (f *fakeMedia) UploadDocument(_ context.Context, _ string, data []byte) (string, error) {
	f.uploaded = data
	return "d9", f.err
}

func (f *fakeMedia) Document(_ context.Context, ownerID, documentID string) ([]byte, error) {
	if ownerID != "u1" || documentID != "d1" {
		return nil, failure.New(failure.NotFound, "document not found")
	}
	return []byte("%PDF-1.7"), nil
}

func (f *fakeMedia) DeleteDocuments(_ context.Context, ownerID string, ids []string) error {
	f.droppedDocuments = append(f.droppedDocuments, ownerID+"/"+strings.Join(ids, ","))
	return nil
}

func (f *fakeMedia) PublishPhotos(_ context.Context, ownerID string, photoIDs []string) error {
	f.published = append(f.published, ownerID+"/"+strings.Join(photoIDs, ","))
	return nil
}

func (f *fakeMedia) DeletePublishedPhotos(_ context.Context, ownerID string, photoIDs []string) error {
	f.unpublished = append(f.unpublished, ownerID+"/"+strings.Join(photoIDs, ","))
	return nil
}

func (f *fakeMedia) UploadClubAvatar(_ context.Context, _, clubID string, image []byte) (string, error) {
	f.uploaded, f.clubAvatarOf = image, clubID
	return "ka1", f.err
}

func (f *fakeMedia) DeleteClubAvatars(_ context.Context, _, clubID, keep string) error {
	f.clubAvatarOf, f.kept, f.deleted = clubID, keep, true
	return nil
}

func (f *fakeMedia) UploadChatAvatar(_ context.Context, _, chatID string, image []byte) (string, error) {
	f.uploaded = image
	f.chatPhotos = append(f.chatPhotos, "upload "+chatID)
	return "cp1", f.err
}

func (f *fakeMedia) DeleteChatAvatars(_ context.Context, _, chatID, keep string) error {
	f.chatPhotos = append(f.chatPhotos, "keep "+chatID+" "+keep)
	return nil
}

func (f *fakeMedia) UploadClubPhoto(_ context.Context, _, clubID string, image []byte) (string, error) {
	f.uploaded, f.clubAvatarOf = image, clubID
	return "kp1", f.err
}

func (f *fakeMedia) DeleteClubPhotos(_ context.Context, _, clubID string, keep []string) error {
	f.clubAvatarOf, f.keptPhotos, f.deleted = clubID, keep, true
	return nil
}

func (f *fakeMedia) UploadPhoto(_ context.Context, _ string, image []byte) (string, error) {
	f.uploaded = image
	return "ph9", f.err
}

// ClaimPhotos: the caller has uploaded ph1, ph2 and what UploadPhoto
// returned, ph9 once it did.
func (f *fakeMedia) ClaimPhotos(_ context.Context, _ string, photoIDs []string) (bool, error) {
	for _, id := range photoIDs {
		own := id == "ph1" || id == "ph2" || (id == "ph9" && f.uploaded != nil)
		if !own || slices.Contains(f.taken, id) {
			return false, f.err
		}
	}

	f.claimed = append(f.claimed, strings.Join(photoIDs, ","))

	return true, f.err
}

func (f *fakeMedia) ReleasePhotos(_ context.Context, _ string, photoIDs []string) error {
	if f.releaseFails > 0 {
		f.releaseFails--
		return failure.New(failure.Unavailable, "service unavailable")
	}

	f.released = append(f.released, strings.Join(photoIDs, ","))
	return nil
}

func (f *fakeMedia) Photo(_ context.Context, userID, photoID string, small bool) ([]byte, error) {
	if userID != "u1" || photoID != "ph1" {
		return nil, failure.New(failure.NotFound, "photo not found")
	}
	if small {
		return []byte("small-photo"), nil
	}
	return []byte("large-photo"), nil
}

func (f *fakeMedia) UploadAvatar(_ context.Context, _ string, image []byte) (string, error) {
	f.uploaded = image
	return "a2", f.err
}

func (f *fakeMedia) DeleteAvatars(_ context.Context, _, keep string) error {
	f.kept, f.deleted = keep, true
	return nil
}

func (f *fakeMedia) Avatar(_ context.Context, userID, avatarID string, small bool) ([]byte, error) {
	if userID != "u1" || avatarID != "a1" {
		return nil, failure.New(failure.NotFound, "avatar not found")
	}
	if small {
		return []byte("small-jpeg"), nil
	}
	return []byte("large-jpeg"), nil
}

// fakeNotifications has a like and a comment by bob (u2), a follow by an
// account that is gone and a like of a deleted post.
type fakeNotifications struct {
	token, cursor, markedUpTo string
	unread                    bool
	limit                     int
	// changes feed Subscribe; closing it ends the stream with streamEnd.
	changes   chan string
	streamEnd error
	mu        sync.Mutex // the event stream counts from its goroutines
	err       error
	// pushKey is what GetPushKey says; subscribed and unsubscribed are the
	// browsers the last calls named.
	pushKey      string
	subscribed   model.PushSubscription
	unsubscribed string
}

func (f *fakeNotifications) List(_ context.Context, token string, unread bool, cursor string, limit int) (model.NotificationPage, error) {
	f.token, f.unread, f.cursor, f.limit = token, unread, cursor, limit
	at := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)
	return model.NotificationPage{
		Notifications: []model.Notification{
			{ID: "n4", Kind: "comment", ActorID: "u2", PostID: "p1", CommentID: "c1", CreatedAt: at},
			{ID: "n3", Kind: "follow", ActorID: "gone", CreatedAt: at},
			{ID: "n2", Kind: "like", ActorID: "u2", PostID: "p-deleted", CreatedAt: at},
			{ID: "n1", Kind: "like", ActorID: "u2", PostID: "p1", CreatedAt: at},
			{ID: "n0", Kind: "follow", ActorID: "u2", CreatedAt: at},
			{ID: "n-1", Kind: "wall_post", ActorID: "u2", PostID: "p1", CreatedAt: at},
			{ID: "n-2", Kind: "repost", ActorID: "u2", PostID: "p1", CreatedAt: at},
			{ID: "n-3", Kind: "suggested_post", ActorID: "k1", PostID: "s1", CreatedAt: at},
			{ID: "n-4", Kind: "topic_message", ActorID: "u2", PostID: "t1", CommentID: "m1", CreatedAt: at},
			{ID: "n-5", Kind: "topic_message", ActorID: "u2", PostID: "t-deleted", CommentID: "m2", CreatedAt: at},
			{ID: "n-6", Kind: "club_admin", ActorID: "k1", CreatedAt: at},
			{ID: "n-7", Kind: "post_mention", ActorID: "u2", PostID: "p1", CreatedAt: at},
			{ID: "n-8", Kind: "comment_mention", ActorID: "u2", PostID: "p1", CommentID: "c1", CreatedAt: at},
			{ID: "n-9", Kind: "topic_mention", ActorID: "u2", PostID: "t1", CommentID: "m1", CreatedAt: at},
			{ID: "n-10", Kind: "topic_message_like", ActorID: "u2", PostID: "t1", CommentID: "m1", CreatedAt: at},
			{ID: "n-11", Kind: "photo_like", ActorID: "u2", PostID: "ph6", CreatedAt: at},
			{ID: "n-12", Kind: "photo_comment", ActorID: "u2", PostID: "ph6", CommentID: "pc1", CreatedAt: at},
			{ID: "n-13", Kind: "photo_like", ActorID: "u2", PostID: "gone", CreatedAt: at},
			{ID: "n-14", Kind: "event_reminder", ActorID: "k1", CreatedAt: at},
			{ID: "n-15", Kind: "photo_mention", ActorID: "u2", PostID: "ph6", CommentID: "pc1", CreatedAt: at},
			{ID: "n-16", Kind: "note_comment", ActorID: "u2", PostID: "note1", CommentID: "nc1", CreatedAt: at},
			{ID: "n-17", Kind: "note_comment", ActorID: "u2", PostID: "note-deleted", CommentID: "nc1", CreatedAt: at},
			{ID: "n-18", Kind: "note_comment", ActorID: "u2", PostID: "note1", CommentID: "nc-deleted", CreatedAt: at},
			{ID: "n-19", Kind: "gift", ActorID: "u2", PostID: "g1", CreatedAt: at},
		},
		NextCursor: "n-2",
	}, f.err
}

func (f *fakeNotifications) CountUnread(_ context.Context, token string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
	return 3, f.err
}

func (f *fakeNotifications) Subscribe(ctx context.Context, token string) (model.NotificationChanges, error) {
	if token != "tok" {
		return nil, failure.New(failure.Unauthenticated, "invalid access token")
	}
	return func() (string, error) {
		select {
		case c, ok := <-f.changes:
			if !ok {
				return "", f.streamEnd
			}
			return c, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}, nil
}

func (f *fakeNotifications) MarkRead(_ context.Context, token, id string) error {
	f.token, f.markedUpTo = token, id
	return f.err
}

func (f *fakeNotifications) PushKey(context.Context) (string, error) {
	return f.pushKey, f.err
}

func (f *fakeNotifications) SubscribePush(_ context.Context, token string, s model.PushSubscription) error {
	if token != "tok" {
		return failure.New(failure.Unauthenticated, "invalid access token")
	}
	if s.Endpoint == "" {
		return failure.New(failure.InvalidArgument, "invalid push endpoint")
	}
	f.subscribed = s
	return f.err
}

func (f *fakeNotifications) UnsubscribePush(_ context.Context, token, endpoint string) error {
	f.token, f.unsubscribed = token, endpoint
	return f.err
}

// fakeMessages: ann is u1, bob u2. Ann has dialogs with bob and with an
// account that is gone.
type fakeMessages struct {
	// sent is what the last message sent said, sealed.
	token, peerID, cursor, after, sent, markedUpTo, deleted string
	// hidden is the message deleted for the caller alone, edited
	// "id/sealed" of the last edit.
	hidden, edited string
	// typingTo is whom the caller last told they type to.
	typingTo string
	// addedPhotos and removedPhotos are those of the last edit;
	// deletedMany "ids forEveryone".
	addedPhotos, removedPhotos []string
	// baseSealed is what the last edit said the message read.
	baseSealed []byte
	// keys are the public keys of users by ID; key the last one set.
	keys        map[string][]byte
	key         []byte
	deletedMany string
	around      string
	// peerWrote: the peer has written to the caller.
	peerWrote bool
	// dialogs replace the default page when set.
	dialogs []model.Dialog
	// events feed Subscribe; closing it ends the stream with streamEnd.
	events    chan model.MessageEvent
	streamEnd error
	mu        sync.Mutex // the event stream counts from its goroutines
	err       error
}

func (f *fakeMessages) Subscribe(ctx context.Context, _ string) (model.MessageEvents, error) {
	return func() (model.MessageEvent, error) {
		select {
		case e, ok := <-f.events:
			if !ok {
				return model.MessageEvent{}, f.streamEnd
			}
			return e, nil
		case <-ctx.Done():
			return model.MessageEvent{}, ctx.Err()
		}
	}, nil
}

func (f *fakeMessages) HasMessagesFrom(_ context.Context, token, peerID string) (bool, error) {
	f.token, f.peerID = token, peerID
	return f.peerWrote, f.err
}

// Send carries each forwarded message on as one bob (u2) wrote, and
// "gone" as one of somebody whose page is no more.
func (f *fakeMessages) Send(_ context.Context, token, recipientID string, d model.MessageDraft) (model.Message, error) {
	f.token, f.peerID, f.sent = token, recipientID, string(d.Sealed)

	var forwarded []model.ForwardedMessage
	for _, id := range d.ForwardIDs {
		sender := "u2"
		if id == "gone" {
			sender = "gone"
		}
		// "under:m2" is a message carried on by the one before it.
		depth := 0
		if strings.HasPrefix(id, "under:") {
			depth = 1
		}
		forwarded = append(forwarded, model.ForwardedMessage{
			SenderID: sender, PhotoIDs: []string{"ph1"}, CreatedAt: albumTime, Depth: depth,
		})
	}

	return model.Message{
		ID: "m9", SenderID: "u1", RecipientID: recipientID, Sealed: d.Sealed, PhotoIDs: d.PhotoIDs, PostID: d.PostID, Forwarded: forwarded,
		SharedPhotoID: d.SharedPhotoID, Sticker: d.Sticker, DocumentIDs: d.DocumentIDs,
		CreatedAt: time.Date(2026, 10, 5, 22, 0, 0, 0, time.UTC),
	}, f.err
}

func (f *fakeMessages) Dialogs(_ context.Context, token, cursor string, _ int) (model.DialogPage, error) {
	f.token, f.cursor = token, cursor
	if f.dialogs != nil {
		return model.DialogPage{Dialogs: f.dialogs}, f.err
	}
	return model.DialogPage{
		Dialogs: []model.Dialog{
			{PeerID: "u2", LastMessage: model.Message{ID: "m2", SenderID: "u2", RecipientID: "u1", Sealed: []byte("как дела?")}, Unread: true},
			{PeerID: "gone", LastMessage: model.Message{ID: "m1", SenderID: "u1", RecipientID: "gone", Sealed: []byte("ау"), Read: true}},
		},
		NextCursor: "m1",
	}, f.err
}

func (f *fakeMessages) CountUnread(_ context.Context, token string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
	return 2, f.err
}

// History around a message ends with one the caller took back.
func (f *fakeMessages) History(_ context.Context, token, peerID string, at model.MessagesAt, _ int) (model.MessagePage, error) {
	f.token, f.peerID, f.cursor, f.after, f.around = token, peerID, at.Cursor, at.After, at.Around

	messages := []model.Message{
		{ID: "m1", SenderID: "u1", RecipientID: peerID, Sealed: []byte("привет"), Read: true, DeletableUntil: time.Date(2026, 10, 6, 22, 0, 0, 0, time.UTC)},
		{ID: "m2", SenderID: peerID, RecipientID: "u1", Sealed: []byte("как дела?")},
	}
	if at.Around != "" {
		messages = append(messages, model.Message{ID: "m3", SenderID: "u1", RecipientID: peerID, DeletedAt: albumTime, DeletableUntil: albumTime})
	}

	return model.MessagePage{
		Messages:     messages,
		NextCursor:   "m1",
		PeerReadUpTo: "m1",
	}, f.err
}

func (f *fakeMessages) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakeMessages) Hide(_ context.Context, token, id string) error {
	f.token, f.hidden = token, id
	return f.err
}

func (f *fakeMessages) DeleteMany(_ context.Context, token string, ids []string, forEveryone bool) error {
	f.token, f.deletedMany = token, fmt.Sprintf("%s %t", strings.Join(ids, ","), forEveryone)
	return f.err
}

func (f *fakeMessages) Edit(_ context.Context, token, id string, e model.MessageEdit) (model.Message, error) {
	f.token, f.edited = token, id+"/"+string(e.Sealed)
	f.addedPhotos, f.removedPhotos = e.AddPhotoIDs, e.RemovePhotoIDs
	f.baseSealed = e.BaseSealed

	return model.Message{ID: id, SenderID: "u1", RecipientID: "u2", Sealed: e.Sealed, PhotoIDs: e.AddPhotoIDs, EditedAt: albumTime, EditableUntil: albumTime}, f.err
}

func (f *fakeMessages) SetKey(_ context.Context, token string, publicKey []byte) error {
	f.token, f.key = token, publicKey
	return f.err
}

// Key is nil for a user with no key.
func (f *fakeMessages) Key(_ context.Context, token, userID string) ([]byte, error) {
	f.token = token
	return f.keys[userID], f.err
}

func (f *fakeMessages) MarkRead(_ context.Context, token, peerID, upTo string) error {
	f.token, f.peerID, f.markedUpTo = token, peerID, upTo
	return f.err
}

func (f *fakeMessages) Typing(_ context.Context, token, peerID string) error {
	f.token, f.typingTo = token, peerID
	return f.err
}

type harness struct {
	handler       http.Handler
	metrics       *prometheus.Registry
	auth          *fakeAuth
	users         *fakeUsers
	posts         *fakePosts
	media         *fakeMedia
	notifications *fakeNotifications
	messages      *fakeMessages
	chats         *fakeChats
	clubs         *fakeClubs
	topics        *fakeTopics
	albums        *fakeAlbums
	notes         *fakeNotes
	faves         *fakeFaves
	gifts         *fakeGifts
	stickers      *fakeStickers
	votes         *fakeVotes
	documents     *fakeDocuments
}

func newHarness(cfg rest.Config) harness {
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = time.Second
	}

	h := harness{
		auth:          &fakeAuth{},
		users:         &fakeUsers{},
		posts:         &fakePosts{},
		media:         &fakeMedia{},
		notifications: &fakeNotifications{},
		messages:      &fakeMessages{},
		chats:         &fakeChats{},
		clubs:         &fakeClubs{},
		topics:        &fakeTopics{},
		albums:        &fakeAlbums{},
		notes:         &fakeNotes{},
		gifts:         &fakeGifts{},
		stickers:      &fakeStickers{},
		votes:         &fakeVotes{},
		faves:         &fakeFaves{},
		documents:     &fakeDocuments{},
	}
	h.metrics = prometheus.NewRegistry()
	h.handler = mustServer(cfg, rest.Backends{
		Auth:          h.auth,
		Users:         h.users,
		Posts:         h.posts,
		Media:         h.media,
		Notifications: h.notifications,
		Messages:      h.messages,
		Chats:         h.chats,
		Clubs:         h.clubs,
		Topics:        h.topics,
		Albums:        h.albums,
		Notes:         h.notes,
		Gifts:         h.gifts,
		Stickers:      h.stickers,
		Votes:         h.votes,
		Faves:         h.faves,
		Documents:     h.documents,
	}, h.metrics).Handler()

	return h
}

// mustServer is rest.NewServer for tests, with metrics in reg.
func mustServer(cfg rest.Config, backends rest.Backends, reg prometheus.Registerer) *rest.Server {
	srv, err := rest.NewServer(cfg, backends, slog.New(slog.NewTextHandler(io.Discard, nil)), reg)
	if err != nil {
		panic(err)
	}

	return srv
}

func (h harness) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "203.0.113.9:51000"
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}

	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)

	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()

	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}

	return v
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestAuthEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("request code", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/auth/code", `{"email":"ann@example.com"}`)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d", w.Code)
		}
		if h.auth.clientIP != "203.0.113.9" {
			t.Fatalf("client ip = %q", h.auth.clientIP)
		}
	})

	t.Run("verify code returns tokens", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("POST", "/api/v1/auth/verify", `{"email":"ann@example.com","code":"123456"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}

		tokens := decode[map[string]string](t, w)
		if tokens["accessToken"] != "access-ann@example.com" || tokens["refreshToken"] != "refresh" {
			t.Fatalf("tokens = %v", tokens)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("token responses must not be cached")
		}
	})

	t.Run("wrong code is 401 with the service message", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("POST", "/api/v1/auth/verify", `{"email":"ann@example.com","code":"000000"}`)
		body := decode[errorBody](t, w)

		if w.Code != http.StatusUnauthorized || body.Error.Code != "unauthenticated" || body.Error.Message != "invalid or expired code" {
			t.Fatalf("%d %+v", w.Code, body)
		}
	})

	t.Run("refresh and logout", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		if w := h.do("POST", "/api/v1/auth/refresh", `{"refreshToken":"r1"}`); w.Code != http.StatusOK {
			t.Fatalf("refresh status = %d", w.Code)
		}
		if w := h.do("POST", "/api/v1/auth/logout", `{"refreshToken":"r1"}`); w.Code != http.StatusNoContent {
			t.Fatalf("logout status = %d", w.Code)
		}
	})
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err    error
		status int
		code   string
	}{
		{failure.New(failure.InvalidArgument, "invalid email"), 400, "invalid_argument"},
		{failure.New(failure.TooManyRequests, "too many requests, try again later"), 429, "too_many_requests"},
		{failure.New(failure.Unavailable, "service temporarily unavailable"), 503, "unavailable"},
		{failure.New(failure.Conflict, "nickname is already taken"), 409, "conflict"},
		{errors.New("dial tcp 10.0.0.5:50051: connection refused"), 500, "internal"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			h.auth.err = tt.err

			w := h.do("POST", "/api/v1/auth/code", `{"email":"ann@example.com"}`)
			body := decode[errorBody](t, w)

			if w.Code != tt.status || body.Error.Code != tt.code {
				t.Fatalf("got %d %+v, want %d %s", w.Code, body, tt.status, tt.code)
			}
			if tt.status == 500 && body.Error.Message != "internal error" {
				t.Fatalf("internal details leaked: %q", body.Error.Message)
			}
		})
	}
}

func TestUserEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("me requires a token", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("GET", "/api/v1/me", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("token is forwarded", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/me", "", "Authorization", "Bearer tok")

		if w.Code != http.StatusOK || h.users.token != "tok" {
			t.Fatalf("status = %d, token = %q", w.Code, h.users.token)
		}
		u := decode[map[string]any](t, w)
		if u["birthday"] != nil || u["profileCompleted"] != false {
			t.Fatalf("fresh user: %v", u)
		}

		// The clubs of the left menu come with the account.
		if pinned, _ := u["pinnedClubs"].([]any); len(pinned) != 1 || pinned[0].(map[string]any)["name"] != cats.Name {
			t.Fatalf("pinned clubs = %v", u["pinnedClubs"])
		}
	})

	t.Run("me without the clubs service", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.clubs.err = errors.New("club is down")

		w := h.do("GET", "/api/v1/me", "", "Authorization", "Bearer tok")
		if u := decode[map[string]any](t, w); w.Code != http.StatusOK || u["pinnedClubs"] != nil {
			t.Fatalf("%d %v", w.Code, u)
		}
	})

	t.Run("update profile", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("PUT", "/api/v1/me/profile",
			`{"name":"Ann","nickname":"ann","gender":"female","birthday":"1990-05-17"}`,
			"Authorization", "Bearer tok")

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body)
		}
		if got := h.users.profile.Birthday; !got.Equal(time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("birthday = %v", got)
		}

		u := decode[map[string]any](t, w)
		if u["birthday"] != "1990-05-17" || u["gender"] != "female" || u["profileCompleted"] != true {
			t.Fatalf("response: %v", u)
		}
	})

	t.Run("bad birthday", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("PUT", "/api/v1/me/profile",
			`{"name":"Ann","nickname":"ann","birthday":"17.05.1990"}`, "Authorization", "Bearer tok")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("update privacy", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("PUT", "/api/v1/me/privacy", `{"showWorldview":true}`, "Authorization", "Bearer tok")
		u := decode[map[string]any](t, w)

		if privacy, _ := u["privacy"].(map[string]any); privacy["showWorldview"] != true || privacy["showBirthday"] != false {
			t.Fatalf("privacy: %v", u["privacy"])
		}
	})

	t.Run("public profile", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/users/ann", "")
		p := decode[map[string]any](t, w)

		if w.Code != http.StatusOK || p["worldview"] != "buddhism" {
			t.Fatalf("%d %v", w.Code, p)
		}
		if _, ok := p["email"]; ok {
			t.Fatal("public profile must not have an email field")
		}

		if w = h.do("GET", "/api/v1/users/nobody", ""); w.Code != http.StatusNotFound {
			t.Fatalf("unknown nickname: %d", w.Code)
		}
	})
}

func TestRequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"malformed json", "POST", "/api/v1/auth/code", `{"email":`, 400},
		{"unknown field", "POST", "/api/v1/auth/code", `{"mail":"a@b.c"}`, 400},
		{"wrong type", "POST", "/api/v1/auth/code", `{"email":5}`, 400},
		{"trailing data", "POST", "/api/v1/auth/code", `{"email":"a@b.c"}{}`, 400},
		{"too large", "POST", "/api/v1/auth/code", `{"email":"` + strings.Repeat("a", 1100<<10) + `"}`, 400},
		{"unknown endpoint", "GET", "/api/v1/nope", "", 404},
		{"wrong method", "GET", "/api/v1/auth/code", "", 405},
		{"wrong method on a path with a parameter", "DELETE", "/api/v1/users/ann", "", 405},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if w := newHarness(rest.Config{}).do(tt.method, tt.path, tt.body); w.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.status, w.Body)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{AllowedOrigins: []string{"http://localhost:5173"}})

	t.Run("preflight from an allowed origin", func(t *testing.T) {
		t.Parallel()

		w := h.do("OPTIONS", "/api/v1/me", "",
			"Origin", "http://localhost:5173", "Access-Control-Request-Method", "PUT")

		if w.Code != http.StatusNoContent ||
			w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
			!strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
			t.Fatalf("%d %v", w.Code, w.Header())
		}
	})

	t.Run("other origins get no CORS headers", func(t *testing.T) {
		t.Parallel()

		w := h.do("GET", "/api/v1/users/ann", "", "Origin", "https://evil.example")
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("foreign origin must not be allowed")
		}
	})
}

func TestClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		trust bool
		xff   string
		want  string
	}{
		{"connection address by default", false, "198.51.100.1", "203.0.113.9"},
		{"rightmost forwarded address when trusted", true, "1.2.3.4, 198.51.100.1", "198.51.100.1"},
		{"garbage falls back to connection", true, "nonsense", "203.0.113.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{TrustForwardedFor: tt.trust})
			h.do("POST", "/api/v1/auth/code", `{"email":"ann@example.com"}`, "X-Forwarded-For", tt.xff)

			if h.auth.clientIP != tt.want {
				t.Fatalf("client ip = %q, want %q", h.auth.clientIP, tt.want)
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	if w := newHarness(rest.Config{}).do("GET", "/healthz", ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

// Suggestion knows s2, whose text lists cut.
func (f *fakePosts) Suggestion(_ context.Context, token, id string) (model.Suggestion, error) {
	f.token = token
	if id != "s2" {
		return model.Suggestion{}, failure.New(failure.NotFound, "suggestion not found")
	}

	return model.Suggestion{ID: id, WallID: "k1", AuthorID: "u2", Text: "от Петра, целиком"}, f.err
}

func TestWholeTexts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, path, token, want string
		status                  int
	}{
		{"topic message", "/api/v1/topic-messages/m1/text", "", "Привет, целиком", http.StatusOK},
		{"unknown topic message", "/api/v1/topic-messages/m404/text", "", "", http.StatusNotFound},
		{"photo comment", "/api/v1/photo-comments/pc1/text", "", "красиво, целиком", http.StatusOK},
		{"unknown photo comment", "/api/v1/photo-comments/pc404/text", "", "", http.StatusNotFound},
		{"suggestion", "/api/v1/suggestions/s2/text", "token", "от Петра, целиком", http.StatusOK},
		{"suggestion, anonymous", "/api/v1/suggestions/s2/text", "", "", http.StatusUnauthorized},
		{"unknown suggestion", "/api/v1/suggestions/s404/text", "token", "", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})

			headers := []string{}
			if tt.token != "" {
				headers = []string{"Authorization", "Bearer " + tt.token}
			}

			w := h.do("GET", tt.path, "", headers...)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.status, w.Body)
			}

			if got := decode[struct{ Text string }](t, w); tt.status == http.StatusOK && got.Text != tt.want {
				t.Errorf("text = %q, want %q", got.Text, tt.want)
			}
		})
	}
}

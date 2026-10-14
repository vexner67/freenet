package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Posts talks to the post service. Methods that read take an optional
// access token: with it, posts tell whether the viewer likes them.
type Posts struct {
	api postv1.PostServiceClient
}

func NewPosts(api postv1.PostServiceClient) *Posts {
	return &Posts{
		api: api,
	}
}

// Create writes a post and returns it without the profiles of Author and
// Wall: the caller knows who they are.
func (p *Posts) Create(ctx context.Context, accessToken string, np model.NewPost) (model.Post, error) {
	resp, err := p.api.CreatePost(withToken(ctx, accessToken), &postv1.CreatePostRequest{
		WallId:   np.WallID,
		Text:     np.Text,
		PhotoIds: np.PhotoIDs,
		ClubWall: np.ClubWall,
		FromWall: np.FromWall,
		Signed:   np.Signed,
		Nsfw:     np.NSFW,

		MentionedIds: np.Mentions,
		AvatarUpdate: np.AvatarUpdate,
		Poll:         toProtoPollDraft(np.Poll),
	})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.CreatePost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// Repost shares the post on the caller's wall and returns the repost.
func (p *Posts) Repost(ctx context.Context, accessToken, postID string, nr model.NewRepost) (model.Post, error) {
	resp, err := p.api.RepostPost(withToken(ctx, accessToken), &postv1.RepostPostRequest{
		PostId: postID, PhotoId: nr.PhotoID, Text: nr.Text, WallId: nr.WallID, FromWall: nr.FromWall, Signed: nr.Signed,
		PhotoIds: nr.PhotoIDs, MentionedIds: nr.Mentions,
	})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.RepostPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

func (p *Posts) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeletePost(withToken(ctx, accessToken), &postv1.DeletePostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeletePost", fromStatus(err))
	}

	return nil
}

// Edit changes the text and photos of the caller's post and returns it.
func (p *Posts) Edit(ctx context.Context, accessToken, id string, edit model.PostEdit) (model.Post, error) {
	resp, err := p.api.EditPost(withToken(ctx, accessToken), &postv1.EditPostRequest{
		Id: id, Text: edit.Text, PhotoIds: edit.PhotoIDs, Nsfw: edit.NSFW, MentionedIds: edit.Mentions,
	})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.EditPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// Pin shows the post first on its wall, which is the caller's.
func (p *Posts) Pin(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.PinPost(withToken(ctx, accessToken), &postv1.PinPostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.PinPost", fromStatus(err))
	}

	return nil
}

func (p *Posts) Unpin(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.UnpinPost(withToken(ctx, accessToken), &postv1.UnpinPostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.UnpinPost", fromStatus(err))
	}

	return nil
}

// Get returns the post without the profiles of Author and Wall; the caller
// looks them up. So do List, Feed and GetMany.
func (p *Posts) Get(ctx context.Context, accessToken, id string) (model.Post, error) {
	resp, err := p.api.GetPost(withToken(ctx, accessToken), &postv1.GetPostRequest{Id: id})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.GetPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// wallFilters are the tabs of openvk's wall.
var wallFilters = map[string]postv1.WallFilter{
	"":        postv1.WallFilter_WALL_FILTER_NONE,
	"owner":   postv1.WallFilter_WALL_FILTER_OWNER,
	"others":  postv1.WallFilter_WALL_FILTER_OTHERS,
	"archive": postv1.WallFilter_WALL_FILTER_ARCHIVE,
}

// Archive puts the post away in the archive of its wall, or back.
func (p *Posts) Archive(ctx context.Context, accessToken, id string, archived bool) error {
	var err error
	if archived {
		_, err = p.api.ArchivePost(withToken(ctx, accessToken), &postv1.ArchivePostRequest{Id: id})
	} else {
		_, err = p.api.UnarchivePost(withToken(ctx, accessToken), &postv1.UnarchivePostRequest{Id: id})
	}
	if err != nil {
		return errs.Wrap("p.api.ArchivePost", fromStatus(err))
	}

	return nil
}

// ArchiveWall puts away every post of the wall written in year, 0 for
// all of them, or brings them back; it tells how many it moved.
func (p *Posts) ArchiveWall(ctx context.Context, accessToken, wallID string, year int, archived bool) (int, error) {
	resp, err := p.api.ArchiveWall(withToken(ctx, accessToken), &postv1.ArchiveWallRequest{
		WallId: wallID, Year: toYear(year), Archived: archived,
	})
	if err != nil {
		return 0, errs.Wrap("p.api.ArchiveWall", fromStatus(err))
	}

	return int(resp.GetChanged()), nil
}

// WallYears are the years the posts of the wall were written in, the
// latest first; told to those who keep its archive.
func (p *Posts) WallYears(ctx context.Context, accessToken, wallID string) ([]int, error) {
	resp, err := p.api.ListWallYears(withToken(ctx, accessToken), &postv1.ListWallYearsRequest{WallId: wallID})
	if err != nil {
		return nil, errs.Wrap("p.api.ListWallYears", fromStatus(err))
	}

	years := make([]int, len(resp.GetYears()))
	for i, y := range resp.GetYears() {
		years[i] = int(y)
	}

	return years, nil
}

// toYear is a year as the post service takes it; one no calendar has
// is left for the service to refuse.
func toYear(year int) int32 {
	if year < 0 || year > math.MaxInt32 {
		return -1
	}

	return int32(year)
}

// CloseComments lets nobody comment the post anymore, or again.
func (p *Posts) CloseComments(ctx context.Context, accessToken, id string, closed bool) error {
	var err error
	if closed {
		_, err = p.api.CloseComments(withToken(ctx, accessToken), &postv1.CloseCommentsRequest{PostId: id})
	} else {
		_, err = p.api.OpenComments(withToken(ctx, accessToken), &postv1.OpenCommentsRequest{PostId: id})
	}
	if err != nil {
		return errs.Wrap("p.api.CloseComments", fromStatus(err))
	}

	return nil
}

// List is the wall of wallID: pick.Filter is "" for all posts, "owner" for
// the wall's own, "others" for the rest and "archive" for those put away.
func (p *Posts) List(ctx context.Context, accessToken, wallID string, pick model.WallPick, cursor string, limit int) (model.PostPage, error) {
	f, ok := wallFilters[pick.Filter]
	if !ok {
		return model.PostPage{}, failure.New(failure.InvalidArgument, "invalid wall type")
	}

	resp, err := p.api.ListPosts(withToken(ctx, accessToken), &postv1.ListPostsRequest{
		WallId: wallID,
		Cursor: cursor,
		Limit:  toLimit(limit),
		Filter: f,
		Query:  pick.Query,
		Year:   toYear(pick.Year),

		TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListPosts", fromStatus(err))
	}

	return model.PostPage{
		Posts:      toPosts(resp.GetPosts()),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}, nil
}

// Feed is the news of the owner of the token: the posts of whom they
// follow and of the walls they read.
func (p *Posts) Feed(ctx context.Context, accessToken string, sources model.FeedSources, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListFeed(withToken(ctx, accessToken), &postv1.ListFeedRequest{
		HideNsfw:        sources.HideNSFW,
		Everyone:        sources.Everyone,
		HiddenAuthorIds: sources.Hidden,
		OwnOnly:         sources.OwnOnly,
		IgnoredWallIds:  sources.Ignored,
		Cursor:          cursor,
		Limit:           toLimit(limit),
		TextLimit:       model.ExcerptLength,
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListFeed", fromStatus(err))
	}

	return model.PostPage{
		Posts:      toPosts(resp.GetPosts()),
		NextCursor: resp.GetNextCursor(),
	}, nil
}

// Reposts are the posts that share the post, newest first, with the
// beginnings of their texts; Total counts them all.
func (p *Posts) Reposts(ctx context.Context, accessToken, postID, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListReposts(withToken(ctx, accessToken), &postv1.ListRepostsRequest{
		PostId: postID, Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListReposts", fromStatus(err))
	}

	return model.PostPage{Posts: toPosts(resp.GetPosts()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// GetMany returns the posts that exist and that the caller, anonymous
// without token, may see, in no order, with the beginnings of their
// texts: they are shown inside something else.
func (p *Posts) GetMany(ctx context.Context, accessToken string, ids []string) ([]model.Post, error) {
	resp, err := p.api.GetPosts(withToken(ctx, accessToken), &postv1.GetPostsRequest{Ids: ids, TextLimit: model.ExcerptLength})
	if err != nil {
		return nil, errs.Wrap("p.api.GetPosts", fromStatus(err))
	}

	return toPosts(resp.GetPosts()), nil
}

// Comment is one comment, whole, without Author.
func (p *Posts) Comment(ctx context.Context, id string) (model.Comment, error) {
	resp, err := p.api.GetComments(ctx, &postv1.GetCommentsRequest{Ids: []string{id}})
	if err != nil {
		return model.Comment{}, errs.Wrap("p.api.GetComments", fromStatus(err))
	}

	if len(resp.GetComments()) == 0 {
		return model.Comment{}, failure.New(failure.NotFound, "comment not found")
	}

	return toComment(resp.GetComments()[0]), nil
}

// CommentsByID returns the comments that exist, without Author, in no
// order, with the beginnings of their texts.
func (p *Posts) CommentsByID(ctx context.Context, ids []string) ([]model.Comment, error) {
	resp, err := p.api.GetComments(ctx, &postv1.GetCommentsRequest{Ids: ids, TextLimit: model.ExcerptLength})
	if err != nil {
		return nil, errs.Wrap("p.api.GetComments", fromStatus(err))
	}

	comments := make([]model.Comment, 0, len(resp.GetComments()))
	for _, c := range resp.GetComments() {
		comments = append(comments, toComment(c))
	}

	return comments, nil
}

// LastComments are the newest three comments of each of the posts, oldest
// first, with liked for the caller of a token.
func (p *Posts) LastComments(ctx context.Context, accessToken string, postIDs []string) ([]model.Comment, error) {
	resp, err := p.api.ListLastComments(withToken(ctx, accessToken), &postv1.ListLastCommentsRequest{PostIds: postIDs, TextLimit: model.ExcerptLength})
	if err != nil {
		return nil, errs.Wrap("p.api.ListLastComments", fromStatus(err))
	}

	comments := make([]model.Comment, 0, len(resp.GetComments()))
	for _, c := range resp.GetComments() {
		comments = append(comments, toComment(c))
	}

	return comments, nil
}

// Like returns how many likes the post has now.
func (p *Posts) Like(ctx context.Context, accessToken, postID string) (int, error) {
	resp, err := p.api.LikePost(withToken(ctx, accessToken), &postv1.LikePostRequest{PostId: postID})
	if err != nil {
		return 0, errs.Wrap("p.api.LikePost", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (p *Posts) Unlike(ctx context.Context, accessToken, postID string) (int, error) {
	resp, err := p.api.UnlikePost(withToken(ctx, accessToken), &postv1.UnlikePostRequest{PostId: postID})
	if err != nil {
		return 0, errs.Wrap("p.api.UnlikePost", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

// PostLikers is who like the post, newest first.
func (p *Posts) PostLikers(ctx context.Context, postID, cursor string, limit int) (model.LikerPage, error) {
	resp, err := p.api.ListPostLikers(ctx, &postv1.ListPostLikersRequest{PostId: postID, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.LikerPage{}, errs.Wrap("p.api.ListPostLikers", fromStatus(err))
	}

	return model.LikerPage{UserIDs: resp.GetUserIds(), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// Polls are the polls of those of the posts that ask one and that the
// viewer may see, at most 100 posts at once.
func (p *Posts) Polls(ctx context.Context, accessToken string, postIDs []string) ([]model.Poll, error) {
	resp, err := p.api.GetPolls(withToken(ctx, accessToken), &postv1.GetPollsRequest{PostIds: postIDs})
	if err != nil {
		return nil, errs.Wrap("p.api.GetPolls", fromStatus(err))
	}

	polls := make([]model.Poll, 0, len(resp.GetPolls()))
	for _, poll := range resp.GetPolls() {
		polls = append(polls, toPoll(poll))
	}

	return polls, nil
}

// Vote chooses options of the poll of a post, by their places, and
// returns the poll as it is now.
func (p *Posts) Vote(ctx context.Context, accessToken, postID string, options []int) (model.Poll, error) {
	places := make([]int32, len(options))
	for i, o := range options {
		// A place out of range is no option of any poll.
		if o < 0 || o > math.MaxInt32 {
			return model.Poll{}, failure.New(failure.InvalidArgument, "invalid choice of poll options")
		}
		places[i] = int32(o)
	}

	resp, err := p.api.VotePoll(withToken(ctx, accessToken), &postv1.VotePollRequest{PostId: postID, Options: places})
	if err != nil {
		return model.Poll{}, errs.Wrap("p.api.VotePoll", fromStatus(err))
	}

	return toPoll(resp.GetPoll()), nil
}

// RetractVote takes the caller's vote back ("Отменить голос").
func (p *Posts) RetractVote(ctx context.Context, accessToken, postID string) (model.Poll, error) {
	resp, err := p.api.RetractPollVote(withToken(ctx, accessToken), &postv1.RetractPollVoteRequest{PostId: postID})
	if err != nil {
		return model.Poll{}, errs.Wrap("p.api.RetractPollVote", fromStatus(err))
	}

	return toPoll(resp.GetPoll()), nil
}

// PollVoters is who chose the option of the poll of a post, newest first;
// Total is the counter of the option.
func (p *Posts) PollVoters(ctx context.Context, postID string, option int, cursor string, limit int) (model.LikerPage, error) {
	if option < 0 || option > math.MaxInt32 {
		return model.LikerPage{}, failure.New(failure.InvalidArgument, "invalid choice of poll options")
	}

	resp, err := p.api.ListPollVoters(ctx, &postv1.ListPollVotersRequest{
		PostId: postID, Option: int32(option), Cursor: cursor, Limit: toLimit(limit),
	})
	if err != nil {
		return model.LikerPage{}, errs.Wrap("p.api.ListPollVoters", fromStatus(err))
	}

	return model.LikerPage{UserIDs: resp.GetUserIds(), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// CommentLikers is who like the comment, newest first.
func (p *Posts) CommentLikers(ctx context.Context, commentID, cursor string, limit int) (model.LikerPage, error) {
	resp, err := p.api.ListCommentLikers(ctx, &postv1.ListCommentLikersRequest{CommentId: commentID, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.LikerPage{}, errs.Wrap("p.api.ListCommentLikers", fromStatus(err))
	}

	return model.LikerPage{UserIDs: resp.GetUserIds(), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// CreateComment returns the comment without Author: the caller knows who it is.
func (p *Posts) CreateComment(ctx context.Context, accessToken, postID string, c model.NewComment) (model.Comment, error) {
	resp, err := p.api.CreateComment(withToken(ctx, accessToken), &postv1.CreateCommentRequest{
		PostId:    postID,
		Text:      c.Text,
		PhotoIds:  c.PhotoIDs,
		ReplyToId: c.ReplyTo,
		FromWall:  c.FromWall,

		MentionedIds: c.Mentions,
	})
	if err != nil {
		return model.Comment{}, errs.Wrap("p.api.CreateComment", fromStatus(err))
	}

	return toComment(resp.GetComment()), nil
}

// LikeComment and UnlikeComment return how many likes the comment has now.
func (p *Posts) LikeComment(ctx context.Context, accessToken, id string) (int, error) {
	resp, err := p.api.LikeComment(withToken(ctx, accessToken), &postv1.LikeCommentRequest{CommentId: id})
	if err != nil {
		return 0, errs.Wrap("p.api.LikeComment", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (p *Posts) UnlikeComment(ctx context.Context, accessToken, id string) (int, error) {
	resp, err := p.api.UnlikeComment(withToken(ctx, accessToken), &postv1.UnlikeCommentRequest{CommentId: id})
	if err != nil {
		return 0, errs.Wrap("p.api.UnlikeComment", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (p *Posts) DeleteComment(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeleteComment(withToken(ctx, accessToken), &postv1.DeleteCommentRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeleteComment", fromStatus(err))
	}

	return nil
}

// Comments returns comments without Author; the caller looks the authors up.
// Comments are a page of a post's comments; with a token, Liked says
// which the caller likes.
func (p *Posts) Comments(ctx context.Context, accessToken, postID, cursor string, limit int) (model.CommentPage, error) {
	resp, err := p.api.ListComments(withToken(ctx, accessToken), &postv1.ListCommentsRequest{
		PostId: postID,
		Cursor: cursor,
		Limit:  toLimit(limit),

		TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.CommentPage{}, errs.Wrap("p.api.ListComments", fromStatus(err))
	}

	page := model.CommentPage{
		Comments:   make([]model.Comment, 0, len(resp.GetComments())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}

	for _, c := range resp.GetComments() {
		page.Comments = append(page.Comments, toComment(c))
	}

	return page, nil
}

func toLimit(limit int) int32 {
	return int32(min(max(limit, 0), 1<<10)) //nolint:gosec // clamped
}

// Suggest suggests a post for the club wallID; the caller has checked that
// the user may and that the photos are theirs.
func (p *Posts) Suggest(ctx context.Context, accessToken, wallID, text string, photoIDs []string) (model.Suggestion, error) {
	resp, err := p.api.SuggestPost(withToken(ctx, accessToken), &postv1.SuggestPostRequest{WallId: wallID, Text: text, PhotoIds: photoIDs})
	if err != nil {
		return model.Suggestion{}, errs.Wrap("p.api.SuggestPost", fromStatus(err))
	}

	return toSuggestion(resp.GetSuggestion()), nil
}

// Suggestion is a suggestion with its whole text, for its author and the
// admins of the club.
func (p *Posts) Suggestion(ctx context.Context, accessToken, id string) (model.Suggestion, error) {
	resp, err := p.api.GetSuggestion(withToken(ctx, accessToken), &postv1.GetSuggestionRequest{Id: id})
	if err != nil {
		return model.Suggestion{}, errs.Wrap("p.api.GetSuggestion", fromStatus(err))
	}

	return toSuggestion(resp.GetSuggestion()), nil
}

// Suggestions pages the suggestions for a club's wall: the caller's own
// with mine, all of them for its admins.
func (p *Posts) Suggestions(ctx context.Context, accessToken, wallID string, mine bool, cursor string, limit int) (model.SuggestionPage, error) {
	resp, err := p.api.ListSuggestions(withToken(ctx, accessToken), &postv1.ListSuggestionsRequest{
		WallId: wallID, Mine: mine, Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.SuggestionPage{}, errs.Wrap("p.api.ListSuggestions", fromStatus(err))
	}

	page := model.SuggestionPage{
		Suggestions: make([]model.Suggestion, 0, len(resp.GetSuggestions())),
		NextCursor:  resp.GetNextCursor(),
		Total:       int(resp.GetTotal()),
	}
	for _, s := range resp.GetSuggestions() {
		page.Suggestions = append(page.Suggestions, toSuggestion(s))
	}

	return page, nil
}

// PublishSuggestion makes a post for the club of a suggestion, saying text
// and, signed, naming its author.
func (p *Posts) PublishSuggestion(ctx context.Context, accessToken, id, text string, signed bool) (model.Post, error) {
	resp, err := p.api.PublishSuggestion(withToken(ctx, accessToken), &postv1.PublishSuggestionRequest{Id: id, Text: text, Signed: signed})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.PublishSuggestion", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// DeleteSuggestion withdraws or declines a suggestion.
func (p *Posts) DeleteSuggestion(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeleteSuggestion(withToken(ctx, accessToken), &postv1.DeleteSuggestionRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeleteSuggestion", fromStatus(err))
	}

	return nil
}

// Postpone keeps a post to publish at its time; the caller has checked
// that the user may speak for the community and that the photos are
// theirs.
func (p *Posts) Postpone(ctx context.Context, accessToken string, in model.NewPostponed) (model.Postponed, error) {
	resp, err := p.api.PostponePost(withToken(ctx, accessToken), &postv1.PostponePostRequest{
		WallId: in.WallID, Text: in.Text, PhotoIds: in.PhotoIDs, FromWall: in.FromWall, Signed: in.Signed, Nsfw: in.NSFW,
		MentionedIds: in.Mentions, PublishAt: timestamppb.New(in.PublishAt),
	})
	if err != nil {
		return model.Postponed{}, errs.Wrap("p.api.PostponePost", fromStatus(err))
	}

	return toPostponed(resp.GetPostponed()), nil
}

// PostponedPosts are all the posts that wait for the wall wallID, the
// soonest first, with the beginnings of their texts.
func (p *Posts) PostponedPosts(ctx context.Context, accessToken, wallID string) ([]model.Postponed, error) {
	resp, err := p.api.ListPostponed(withToken(ctx, accessToken), &postv1.ListPostponedRequest{
		WallId: wallID, TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return nil, errs.Wrap("p.api.ListPostponed", fromStatus(err))
	}

	found := make([]model.Postponed, 0, len(resp.GetPostponed()))
	for _, pp := range resp.GetPostponed() {
		found = append(found, toPostponed(pp))
	}

	return found, nil
}

// PostponedPost is one postponed post, whole.
func (p *Posts) PostponedPost(ctx context.Context, accessToken, id string) (model.Postponed, error) {
	resp, err := p.api.GetPostponed(withToken(ctx, accessToken), &postv1.GetPostponedRequest{Id: id})
	if err != nil {
		return model.Postponed{}, errs.Wrap("p.api.GetPostponed", fromStatus(err))
	}

	return toPostponed(resp.GetPostponed()), nil
}

// EditPostponed changes what a postponed post says and when it comes out.
func (p *Posts) EditPostponed(ctx context.Context, accessToken, id string, edit model.PostponedEdit) (model.Postponed, error) {
	resp, err := p.api.EditPostponed(withToken(ctx, accessToken), &postv1.EditPostponedRequest{
		Id: id, Text: edit.Text, Nsfw: edit.NSFW, MentionedIds: edit.Mentions, PublishAt: timestamppb.New(edit.PublishAt),
	})
	if err != nil {
		return model.Postponed{}, errs.Wrap("p.api.EditPostponed", fromStatus(err))
	}

	return toPostponed(resp.GetPostponed()), nil
}

// PublishPostponed publishes a postponed post at once.
func (p *Posts) PublishPostponed(ctx context.Context, accessToken, id string) (model.Post, error) {
	resp, err := p.api.PublishPostponed(withToken(ctx, accessToken), &postv1.PublishPostponedRequest{Id: id})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.PublishPostponed", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// DeletePostponed deletes a postponed post with its photos.
func (p *Posts) DeletePostponed(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeletePostponed(withToken(ctx, accessToken), &postv1.DeletePostponedRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeletePostponed", fromStatus(err))
	}

	return nil
}

func toPostponed(p *postv1.PostponedPost) model.Postponed {
	return model.Postponed{
		ID:        p.GetId(),
		WallID:    p.GetWallId(),
		AuthorID:  p.GetAuthorId(),
		Text:      p.GetText(),
		TextCut:   p.GetTextCut(),
		PhotoIDs:  p.GetPhotoIds(),
		FromWall:  p.GetFromWall(),
		Signed:    p.GetSigned(),
		NSFW:      p.GetNsfw(),
		PublishAt: p.GetPublishAt().AsTime(),
		CreatedAt: p.GetCreatedAt().AsTime(),
	}
}

func toSuggestion(s *postv1.Suggestion) model.Suggestion {
	return model.Suggestion{
		ID:        s.GetId(),
		WallID:    s.GetWallId(),
		AuthorID:  s.GetAuthorId(),
		Text:      s.GetText(),
		TextCut:   s.GetTextCut(),
		PhotoIDs:  s.GetPhotoIds(),
		CreatedAt: s.GetCreatedAt().AsTime(),
	}
}

func toPost(p *postv1.Post) model.Post {
	return model.Post{
		ID:        p.GetId(),
		Author:    model.Author{ID: p.GetAuthorId()},
		Wall:      model.Author{ID: p.GetWallId()},
		Text:      p.GetText(),
		TextCut:   p.GetTextCut(),
		PhotoIDs:  p.GetPhotoIds(),
		CreatedAt: p.GetCreatedAt().AsTime(),
		Likes:     int(p.GetLikeCount()),
		Comments:  int(p.GetCommentCount()),
		Reposts:   int(p.GetRepostCount()),
		Liked:     p.GetLiked(),
		RepostOf:  p.GetRepostOf(),
		// Unset timestamps are zero times, not the Unix epoch.
		EditedAt:       fromTimestamp(p.GetEditedAt()),
		EditableUntil:  fromTimestamp(p.GetEditableUntil()),
		Pinned:         p.GetPinned(),
		Archived:       p.GetArchived(),
		CommentsClosed: p.GetCommentsClosed(),
		NSFW:           p.GetNsfw(),
		FromWall:       p.GetFromWall(),
		Signed:         p.GetSigned(),
		AvatarUpdate:   p.GetAvatarUpdate(),
		SharedPhotoID:  p.GetSharedPhotoId(),
		Mine:           p.GetMine(),
		Polled:         p.GetPolled(),
	}
}

func toProtoPollDraft(p *model.NewPoll) *postv1.PollDraft {
	if p == nil {
		return nil
	}

	return &postv1.PollDraft{
		Title:     p.Title,
		Options:   p.Options,
		Anonymous: p.Anonymous,
		Multiple:  p.Multiple,
		Locked:    p.Locked,
		// More days than a poll may last, whatever the number.
		Days: int32(min(max(p.Days, -1), math.MaxInt32)), //nolint:gosec // clamped to int32
	}
}

func toPoll(p *postv1.Poll) model.Poll {
	options := make([]model.PollOption, 0, len(p.GetOptions()))
	for _, o := range p.GetOptions() {
		options = append(options, model.PollOption{Name: o.GetName(), Votes: int(o.GetVoteCount())})
	}

	chosen := make([]int, 0, len(p.GetChosen()))
	for _, place := range p.GetChosen() {
		chosen = append(chosen, int(place))
	}

	return model.Poll{
		PostID:    p.GetPostId(),
		Title:     p.GetTitle(),
		Options:   options,
		Voters:    int(p.GetVoterCount()),
		Anonymous: p.GetAnonymous(),
		Multiple:  p.GetMultiple(),
		Locked:    p.GetLocked(),
		EndsAt:    fromTimestamp(p.GetEndsAt()),
		Ended:     p.GetEnded(),
		Chosen:    chosen,
	}
}

func toPosts(posts []*postv1.Post) []model.Post {
	out := make([]model.Post, 0, len(posts))
	for _, p := range posts {
		out = append(out, toPost(p))
	}

	return out
}

func toComment(c *postv1.Comment) model.Comment {
	return model.Comment{
		ID:        c.GetId(),
		PostID:    c.GetPostId(),
		Author:    model.Author{ID: c.GetAuthorId()},
		Text:      c.GetText(),
		TextCut:   c.GetTextCut(),
		PhotoIDs:  c.GetPhotoIds(),
		CreatedAt: c.GetCreatedAt().AsTime(),
		ReplyTo:   c.GetReplyToId(),
		Likes:     int(c.GetLikeCount()),
		Liked:     c.GetLiked(),
		FromWall:  c.GetFromWall(),
		Mine:      c.GetMine(),
	}
}

// Count counts the posts on all walls.
func (p *Posts) Count(ctx context.Context) (int, error) {
	resp, err := p.api.CountPosts(ctx, &postv1.CountPostsRequest{})
	if err != nil {
		return 0, errs.Wrap("p.api.CountPosts", fromStatus(err))
	}

	return int(resp.GetCount()), nil
}

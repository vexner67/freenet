package grpc

import (
	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fromProtoContent reads what a new post says and the poll it asks, nil
// for none.
func fromProtoContent(req *postv1.CreatePostRequest) (post.Content, *post.PollDraft, error) {
	text, photos, err := fromProtoTextPhotos(req.GetText(), req.GetPhotoIds())
	if err != nil {
		return post.Content{}, nil, err
	}

	documents, err := fromProtoDocuments(req.GetDocumentIds())
	if err != nil {
		return post.Content{}, nil, err
	}

	link, err := fromProtoLink(req.GetLink())
	if err != nil {
		return post.Content{}, nil, err
	}

	if req.GetPoll() != nil {
		poll, err := fromProtoPollDraft(req.GetPoll())
		if err != nil {
			return post.Content{}, nil, status.Error(codes.InvalidArgument, err.Error())
		}

		return post.NewPolled(text, photos).WithDocuments(documents).WithLink(link), &poll, nil
	}

	content, err := post.NewDocumented(text, photos, documents)
	if err != nil {
		return post.Content{}, nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return content.WithLink(link), nil, nil
}

// fromProtoLink is the card of a new post; zero for none.
func fromProtoLink(l *postv1.Link) (post.Link, error) {
	if l == nil {
		return post.Link{}, nil
	}

	d := post.LinkDraft{URL: l.GetUrl(), Title: l.GetTitle(), Description: l.GetDescription(), Site: l.GetSite()}

	if l.GetPhotoId() != "" {
		id, err := uuid.Parse(l.GetPhotoId())
		if err != nil || id == uuid.Nil {
			return post.Link{}, status.Error(codes.InvalidArgument, post.ErrInvalidLink.Error())
		}
		d.PhotoID = id
	}

	link, err := post.NewLink(d)
	if err != nil {
		return post.Link{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return link, nil
}

// toProtoLink is the card of a post, nil for none.
func toProtoLink(l post.Link) *postv1.Link {
	if l.IsZero() {
		return nil
	}

	return &postv1.Link{Url: l.URL(), Title: l.Title(), Description: l.Description(), Site: l.Site(), PhotoId: idString(l.PhotoID())}
}

func fromProtoPollDraft(d *postv1.PollDraft) (post.PollDraft, error) {
	title, err := post.NewPollTitle(d.GetTitle())
	if err != nil {
		return post.PollDraft{}, err
	}

	options, err := post.NewPollOptions(d.GetOptions())
	if err != nil {
		return post.PollDraft{}, err
	}

	return post.PollDraft{
		Title:     title,
		Options:   options,
		Anonymous: d.GetAnonymous(),
		Multiple:  d.GetMultiple(),
		Locked:    d.GetLocked(),
		Days:      int(d.GetDays()),
	}, nil
}

func toProtoPoll(v service.PollView) *postv1.Poll {
	p := v.Poll
	tally := p.Tally()

	options := make([]*postv1.PollOption, len(p.Options()))
	for i, name := range p.Options() {
		options[i] = &postv1.PollOption{
			Name:      name.String(),
			VoteCount: int32(tally.Votes[i]), //nolint:gosec // counters come from int4 columns
		}
	}

	chosen := make([]int32, len(v.Chosen))
	for i, place := range v.Chosen {
		chosen[i] = int32(place) //nolint:gosec // a place is below post.MaxPollOptions
	}

	pp := &postv1.Poll{
		PostId:     p.PostID().String(),
		Title:      p.Title().String(),
		Options:    options,
		VoterCount: int32(tally.Voters), //nolint:gosec // counters come from int4 columns
		Anonymous:  p.Anonymous(),
		Multiple:   p.Multiple(),
		Locked:     p.Locked(),
		Ended:      v.Ended,
		Chosen:     chosen,
	}

	if !p.EndsAt().IsZero() {
		pp.EndsAt = timestamppb.New(p.EndsAt())
	}

	return pp
}

// fromProtoTextPhotos reads what a post says, for a new post or an edit.
func fromProtoTextPhotos(rawText string, rawPhotos []string) (post.Text, post.Photos, error) {
	text, err := post.NewText(rawText)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, err.Error())
	}

	photos, err := fromProtoPhotos(rawPhotos)
	if err != nil {
		return "", nil, err
	}

	return text, photos, nil
}

// fromProtoCommentContent reads what a new comment says.
func fromProtoCommentContent(req *postv1.CreateCommentRequest) (post.CommentContent, error) {
	return commentContent(req.GetText(), req.GetPhotoIds())
}

// commentContent reads what a comment or a message in a topic says.
func commentContent(rawText string, rawPhotos []string) (post.CommentContent, error) {
	text, err := post.NewCommentText(rawText)
	if err != nil {
		return post.CommentContent{}, status.Error(codes.InvalidArgument, err.Error())
	}

	photos, err := fromProtoPhotos(rawPhotos)
	if err != nil {
		return post.CommentContent{}, err
	}

	content, err := post.NewCommentContent(text, photos)
	if err != nil {
		return post.CommentContent{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return content, nil
}

func fromProtoPhotos(raw []string) (post.Photos, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, post.ErrInvalidPhoto.Error())
		}
		ids = append(ids, id)
	}

	photos, err := post.NewPhotos(ids)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if len(photos) == 0 {
		return nil, nil // no photos, as storage reads them back
	}

	return photos, nil
}

func photoStrings(photos post.Photos) []string {
	out := make([]string, len(photos))
	for i, id := range photos {
		out[i] = id.String()
	}

	return out
}

// excerpted cuts the text of a post to limit characters, as a list asks;
// 0 leaves it whole.
func excerpted(pp *postv1.Post, p *post.Post, limit int32) *postv1.Post {
	pp.Text, pp.TextCut = p.Content().Text().Excerpt(int(limit))

	return pp
}

// excerptedComment is excerpted for a comment.
func excerptedComment(pc *postv1.Comment, c *post.Comment, limit int32) *postv1.Comment {
	pc.Text, pc.TextCut = c.Text().Excerpt(int(limit))

	return pc
}

// toProtoPost: viewerID is the caller, uuid.Nil if anonymous.
func toProtoPost(p *post.Post, liked bool, viewerID uuid.UUID) *postv1.Post {
	counters := p.Counters()

	pp := &postv1.Post{
		Id:             p.ID().String(),
		WallId:         p.WallID().String(),
		AuthorId:       p.AuthorID().String(),
		Text:           p.Content().Text().String(),
		PhotoIds:       photoStrings(p.Content().Photos()),
		CreatedAt:      timestamppb.New(p.CreatedAt()),
		LikeCount:      int32(counters.Likes),    //nolint:gosec // counters come from int4 columns
		CommentCount:   int32(counters.Comments), //nolint:gosec // counters come from int4 columns
		RepostCount:    int32(counters.Reposts),  //nolint:gosec // counters come from int4 columns
		RepostOf:       idString(p.Content().RepostOf()),
		Liked:          liked,
		Pinned:         p.Pinned(),
		EditableUntil:  timestamppb.New(p.EditableUntil()),
		FromWall:       p.FromWall(),
		Signed:         p.Signed(),
		AvatarUpdate:   p.AvatarUpdate(),
		SharedPhotoId:  idString(p.Content().SharedPhoto()),
		Mine:           viewerID != uuid.Nil && p.BelongsTo(viewerID),
		Archived:       p.Archived(),
		CommentsClosed: p.CommentsClosed(),
		Nsfw:           p.NSFW(),
		Polled:         p.Content().Polled(),
		DocumentIds:    idStrings(p.Content().Documents()),
		Link:           toProtoLink(p.Content().Link()),
	}

	if !p.EditedAt().IsZero() {
		pp.EditedAt = timestamppb.New(p.EditedAt())
	}

	return pp
}

// toProtoPosts are the posts of a page, their texts cut to textLimit.
func toProtoPosts(page service.Page, viewerID uuid.UUID, textLimit int32) []*postv1.Post {
	posts := make([]*postv1.Post, 0, len(page.Posts))
	for _, p := range page.Posts {
		posts = append(posts, excerpted(toProtoPost(p, page.Liked[p.ID()], viewerID), p, textLimit))
	}

	return posts
}

func toProtoComment(c *post.Comment, liked bool, viewerID uuid.UUID) *postv1.Comment {
	return &postv1.Comment{
		Id:        c.ID().String(),
		PostId:    c.PostID().String(),
		AuthorId:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
		PhotoIds:  photoStrings(c.Photos()),
		ReplyToId: idString(c.ReplyTo()),
		LikeCount: int32(c.Likes()), //nolint:gosec // counters come from int4 columns
		Liked:     liked,
		FromWall:  c.FromWall(),
		Mine:      viewerID != uuid.Nil && viewerID == c.AuthorID(),
	}
}

// cursorString is "" on the last page.
func cursorString(cursor uuid.UUID) string {
	return idString(cursor)
}

// idString is "" for uuid.Nil, as proto fields say "none".
func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}

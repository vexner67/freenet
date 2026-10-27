package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// articleExcerpt is how long the start of the text of an article is.
const articleExcerpt = 300

type ArticleService interface {
	Create(ctx context.Context, authorID uuid.UUID, d post.ArticleDraft) (*post.Article, error)
	Edit(ctx context.Context, requesterID, id uuid.UUID, d post.ArticleDraft) (*post.Article, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (*post.Article, error)
	List(ctx context.Context, authorID, cursor uuid.UUID, size int) (service.ArticlePage, error)
}

func (h *PostHandler) CreateArticle(ctx context.Context, req *postv1.CreateArticleRequest) (*postv1.CreateArticleResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	d, err := fromProtoArticle(req.GetTitle(), req.GetCoverId(), req.GetBlocks())
	if err != nil {
		return nil, err
	}

	a, err := h.articleService.Create(ctx, userID, d)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateArticleResponse{Article: toProtoArticle(a, false)}, nil
}

func (h *PostHandler) EditArticle(ctx context.Context, req *postv1.EditArticleRequest) (*postv1.EditArticleResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseArticleID(req.GetId())
	if err != nil {
		return nil, err
	}

	d, err := fromProtoArticle(req.GetTitle(), req.GetCoverId(), req.GetBlocks())
	if err != nil {
		return nil, err
	}

	a, err := h.articleService.Edit(ctx, userID, id, d)
	if err != nil {
		return nil, err
	}

	return &postv1.EditArticleResponse{Article: toProtoArticle(a, false)}, nil
}

func (h *PostHandler) DeleteArticle(ctx context.Context, req *postv1.DeleteArticleRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseArticleID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.articleService.Delete(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) GetArticle(ctx context.Context, req *postv1.GetArticleRequest) (*postv1.GetArticleResponse, error) {
	id, err := parseArticleID(req.GetId())
	if err != nil {
		return nil, err
	}

	a, err := h.articleService.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetArticleResponse{Article: toProtoArticle(a, req.GetBrief())}, nil
}

func (h *PostHandler) ListArticles(ctx context.Context, req *postv1.ListArticlesRequest) (*postv1.ListArticlesResponse, error) {
	authorID, err := uuid.Parse(req.GetAuthorId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid author id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.articleService.List(ctx, authorID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListArticlesResponse{Total: int32(page.Total)}
	for _, a := range page.Articles {
		resp.Articles = append(resp.Articles, toProtoArticle(a, true))
	}
	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func parseArticleID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid article id")
	}

	return id, nil
}

// optionalPhotoID reads the ID of a photo, uuid.Nil for none.
func optionalPhotoID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid photo id")
	}

	return id, nil
}

var articleBlockKinds = map[postv1.ArticleBlockKind]post.BlockKind{
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PARAGRAPH:  post.BlockParagraph,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_HEADING:    post.BlockHeading,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_SUBHEADING: post.BlockSubheading,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_QUOTE:      post.BlockQuote,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_BULLET:     post.BlockBullet,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_NUMBER:     post.BlockNumber,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_CODE:       post.BlockCode,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PHOTO:      post.BlockPhoto,
	postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_DIVIDER:    post.BlockDivider,
}

func fromProtoArticle(title, cover string, raw []*postv1.ArticleBlock) (post.ArticleDraft, error) {
	coverID, err := optionalPhotoID(cover)
	if err != nil {
		return post.ArticleDraft{}, err
	}

	d := post.ArticleDraft{Title: title, CoverID: coverID, Blocks: make([]post.Block, 0, len(raw))}
	for _, rb := range raw {
		kind, ok := articleBlockKinds[rb.GetKind()]
		if !ok {
			return post.ArticleDraft{}, status.Error(codes.InvalidArgument, post.ErrArticleBlock.Error())
		}

		photoID, err := optionalPhotoID(rb.GetPhotoId())
		if err != nil {
			return post.ArticleDraft{}, err
		}

		b := post.Block{Kind: kind, PhotoID: photoID, Caption: rb.GetCaption()}
		for _, s := range rb.GetSpans() {
			b.Spans = append(b.Spans, post.Span{Text: s.GetText(), Bold: s.GetBold(), Italic: s.GetItalic(), Strike: s.GetStrike(), Link: s.GetLink()})
		}
		d.Blocks = append(d.Blocks, b)
	}

	return d, nil
}

// toProtoArticle is a, without its blocks when brief.
func toProtoArticle(a *post.Article, brief bool) *postv1.Article {
	excerpt, _ := a.Excerpt(articleExcerpt)
	pa := &postv1.Article{
		Id:        a.ID().String(),
		AuthorId:  a.AuthorID().String(),
		Title:     a.Title(),
		Excerpt:   excerpt,
		Minutes:   int32(a.Minutes()),
		CreatedAt: timestamppb.New(a.CreatedAt()),
	}
	if a.CoverID() != uuid.Nil {
		pa.CoverId = a.CoverID().String()
	}
	if a.EditedAt() != nil {
		pa.EditedAt = timestamppb.New(*a.EditedAt())
	}

	if brief {
		return pa
	}

	for _, b := range a.Blocks() {
		pb := &postv1.ArticleBlock{Caption: b.Caption}
		for kind, k := range articleBlockKinds {
			if k == b.Kind {
				pb.Kind = kind
			}
		}
		if b.PhotoID != uuid.Nil {
			pb.PhotoId = b.PhotoID.String()
		}
		for _, s := range b.Spans {
			pb.Spans = append(pb.Spans, &postv1.ArticleSpan{Text: s.Text, Bold: s.Bold, Italic: s.Italic, Strike: s.Strike, Link: s.Link})
		}
		pa.Blocks = append(pa.Blocks, pb)
	}

	return pa
}

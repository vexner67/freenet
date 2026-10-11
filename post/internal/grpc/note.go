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

type NoteService interface {
	Create(ctx context.Context, authorID uuid.UUID, title post.NoteTitle, text post.Text) (*post.Note, error)
	Get(ctx context.Context, id uuid.UUID) (*post.Note, error)
	List(ctx context.Context, authorID, cursor uuid.UUID, size int) (service.NotePage, error)
	Change(ctx context.Context, requesterID, id uuid.UUID, title post.NoteTitle, text post.Text) (*post.Note, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	Comment(ctx context.Context, authorID, noteID uuid.UUID, text post.CommentText) (*post.NoteComment, error)
	Comments(ctx context.Context, noteID, after uuid.UUID, size int) (service.NoteCommentPage, error)
	CommentByID(ctx context.Context, id uuid.UUID) (*post.NoteComment, error)
	CommentsByID(ctx context.Context, ids []uuid.UUID) ([]*post.NoteComment, error)
	DeleteComment(ctx context.Context, requesterID, id uuid.UUID) error
}

func (h *PostHandler) CreateNote(ctx context.Context, req *postv1.CreateNoteRequest) (*postv1.CreateNoteResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	title, text, err := noteWords(req.GetTitle(), req.GetText())
	if err != nil {
		return nil, err
	}

	n, err := h.noteService.Create(ctx, userID, title, text)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateNoteResponse{Note: toProtoNote(n, userID, 0)}, nil
}

func (h *PostHandler) GetNote(ctx context.Context, req *postv1.GetNoteRequest) (*postv1.GetNoteResponse, error) {
	id, err := parseNoteID(req.GetId())
	if err != nil {
		return nil, err
	}

	n, err := h.noteService.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetNoteResponse{Note: toProtoNote(n, viewerID(ctx), 0)}, nil
}

func (h *PostHandler) ListNotes(ctx context.Context, req *postv1.ListNotesRequest) (*postv1.ListNotesResponse, error) {
	authorID, err := uuid.Parse(req.GetAuthorId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid author id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.noteService.List(ctx, authorID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListNotesResponse{
		Notes: make([]*postv1.Note, 0, len(page.Notes)),
		Total: int32(page.Total), //nolint:gosec // a count of rows
	}

	for _, n := range page.Notes {
		resp.Notes = append(resp.Notes, toProtoNote(n, viewer, int(req.GetTextLimit())))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *PostHandler) EditNote(ctx context.Context, req *postv1.EditNoteRequest) (*postv1.EditNoteResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseNoteID(req.GetId())
	if err != nil {
		return nil, err
	}

	title, text, err := noteWords(req.GetTitle(), req.GetText())
	if err != nil {
		return nil, err
	}

	n, err := h.noteService.Change(ctx, userID, id, title, text)
	if err != nil {
		return nil, err
	}

	return &postv1.EditNoteResponse{Note: toProtoNote(n, userID, 0)}, nil
}

func (h *PostHandler) DeleteNote(ctx context.Context, req *postv1.DeleteNoteRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseNoteID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.noteService.Delete(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) CreateNoteComment(ctx context.Context, req *postv1.CreateNoteCommentRequest) (*postv1.CreateNoteCommentResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	noteID, err := parseNoteID(req.GetNoteId())
	if err != nil {
		return nil, err
	}

	text, err := post.NewCommentText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	c, err := h.noteService.Comment(ctx, userID, noteID, text)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateNoteCommentResponse{Comment: toProtoNoteComment(c, userID, 0)}, nil
}

func (h *PostHandler) DeleteNoteComment(ctx context.Context, req *postv1.DeleteNoteCommentRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	if err = h.noteService.DeleteComment(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) ListNoteComments(ctx context.Context, req *postv1.ListNoteCommentsRequest) (*postv1.ListNoteCommentsResponse, error) {
	noteID, err := parseNoteID(req.GetNoteId())
	if err != nil {
		return nil, err
	}

	after, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.noteService.Comments(ctx, noteID, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListNoteCommentsResponse{
		Comments: make([]*postv1.NoteComment, 0, len(page.Comments)),
		Total:    int32(page.Total), //nolint:gosec // a counter from an int4 column
	}

	for _, c := range page.Comments {
		resp.Comments = append(resp.Comments, toProtoNoteComment(c, viewer, int(req.GetTextLimit())))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *PostHandler) GetNoteComment(ctx context.Context, req *postv1.GetNoteCommentRequest) (*postv1.GetNoteCommentResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	c, err := h.noteService.CommentByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetNoteCommentResponse{Comment: toProtoNoteComment(c, viewerID(ctx), 0)}, nil
}

func (h *PostHandler) GetNoteComments(ctx context.Context, req *postv1.GetNoteCommentsRequest) (*postv1.GetNoteCommentsResponse, error) {
	ids, err := parseIDs(req.GetIds())
	if err != nil {
		return nil, err
	}

	found, err := h.noteService.CommentsByID(ctx, ids)
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.GetNoteCommentsResponse{Comments: make([]*postv1.NoteComment, 0, len(found))}

	for _, c := range found {
		resp.Comments = append(resp.Comments, toProtoNoteComment(c, viewer, int(req.GetTextLimit())))
	}

	return resp, nil
}

// noteWords reads the title and the text of a note; its errors are meant
// for the client.
func noteWords(rawTitle, rawText string) (post.NoteTitle, post.Text, error) {
	title, err := post.NewNoteTitle(rawTitle)
	if err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}

	text, err := post.NewNoteText(rawText)
	if err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}

	return title, text, nil
}

func parseNoteID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid note id")
	}

	return id, nil
}

// toProtoNote: the text is cut to textLimit characters, 0 for whole.
func toProtoNote(n *post.Note, viewerID uuid.UUID, textLimit int) *postv1.Note {
	pn := &postv1.Note{
		Id:           n.ID().String(),
		AuthorId:     n.AuthorID().String(),
		Title:        n.Title().String(),
		CommentCount: int32(n.Comments()), //nolint:gosec // a counter from an int4 column
		CreatedAt:    timestamppb.New(n.CreatedAt()),
		Mine:         viewerID != uuid.Nil && viewerID == n.AuthorID(),
	}
	pn.Text, pn.TextCut = n.Text().Excerpt(textLimit)

	if !n.EditedAt().IsZero() {
		pn.EditedAt = timestamppb.New(n.EditedAt())
	}

	return pn
}

func toProtoNoteComment(c *post.NoteComment, viewerID uuid.UUID, textLimit int) *postv1.NoteComment {
	pc := &postv1.NoteComment{
		Id:        c.ID().String(),
		NoteId:    c.NoteID().String(),
		AuthorId:  c.AuthorID().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
		Mine:      viewerID != uuid.Nil && viewerID == c.AuthorID(),
	}
	pc.Text, pc.TextCut = c.Text().Excerpt(textLimit)

	return pc
}

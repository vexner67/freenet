package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Notes is rest.Notes over the post service, which keeps the notes of
// people and the comments under them.
type Notes struct {
	api postv1.PostServiceClient
}

func NewNotes(api postv1.PostServiceClient) *Notes {
	return &Notes{
		api: api,
	}
}

func (n *Notes) Create(ctx context.Context, accessToken, title, text string) (model.Note, error) {
	resp, err := n.api.CreateNote(withToken(ctx, accessToken), &postv1.CreateNoteRequest{Title: title, Text: text})
	if err != nil {
		return model.Note{}, errs.Wrap("n.api.CreateNote", fromStatus(err))
	}

	return toNote(resp.GetNote()), nil
}

// Get is a note with its whole text.
func (n *Notes) Get(ctx context.Context, accessToken, id string) (model.Note, error) {
	resp, err := n.api.GetNote(withToken(ctx, accessToken), &postv1.GetNoteRequest{Id: id})
	if err != nil {
		return model.Note{}, errs.Wrap("n.api.GetNote", fromStatus(err))
	}

	return toNote(resp.GetNote()), nil
}

// List pages the notes of authorID, newest first, with the beginnings of
// their texts.
func (n *Notes) List(ctx context.Context, accessToken, authorID, cursor string, limit int) (model.NotePage, error) {
	resp, err := n.api.ListNotes(withToken(ctx, accessToken), &postv1.ListNotesRequest{
		AuthorId: authorID, Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.NotePage{}, errs.Wrap("n.api.ListNotes", fromStatus(err))
	}

	page := model.NotePage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pn := range resp.GetNotes() {
		page.Notes = append(page.Notes, toNote(pn))
	}

	return page, nil
}

func (n *Notes) Edit(ctx context.Context, accessToken, id, title, text string) (model.Note, error) {
	resp, err := n.api.EditNote(withToken(ctx, accessToken), &postv1.EditNoteRequest{Id: id, Title: title, Text: text})
	if err != nil {
		return model.Note{}, errs.Wrap("n.api.EditNote", fromStatus(err))
	}

	return toNote(resp.GetNote()), nil
}

func (n *Notes) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := n.api.DeleteNote(withToken(ctx, accessToken), &postv1.DeleteNoteRequest{Id: id}); err != nil {
		return errs.Wrap("n.api.DeleteNote", fromStatus(err))
	}

	return nil
}

func (n *Notes) Comment(ctx context.Context, accessToken, noteID, text string) (model.NoteComment, error) {
	resp, err := n.api.CreateNoteComment(withToken(ctx, accessToken), &postv1.CreateNoteCommentRequest{NoteId: noteID, Text: text})
	if err != nil {
		return model.NoteComment{}, errs.Wrap("n.api.CreateNoteComment", fromStatus(err))
	}

	return toNoteComment(resp.GetComment()), nil
}

func (n *Notes) DeleteComment(ctx context.Context, accessToken, id string) error {
	if _, err := n.api.DeleteNoteComment(withToken(ctx, accessToken), &postv1.DeleteNoteCommentRequest{Id: id}); err != nil {
		return errs.Wrap("n.api.DeleteNoteComment", fromStatus(err))
	}

	return nil
}

// Comments pages the comments of a note, oldest first, with the
// beginnings of their texts.
func (n *Notes) Comments(ctx context.Context, accessToken, noteID, cursor string, limit int) (model.NoteCommentPage, error) {
	resp, err := n.api.ListNoteComments(withToken(ctx, accessToken), &postv1.ListNoteCommentsRequest{
		NoteId: noteID, Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.NoteCommentPage{}, errs.Wrap("n.api.ListNoteComments", fromStatus(err))
	}

	page := model.NoteCommentPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, c := range resp.GetComments() {
		page.Comments = append(page.Comments, toNoteComment(c))
	}

	return page, nil
}

// CommentByID is a comment under a note with its whole text.
func (n *Notes) CommentByID(ctx context.Context, id string) (model.NoteComment, error) {
	resp, err := n.api.GetNoteComment(ctx, &postv1.GetNoteCommentRequest{Id: id})
	if err != nil {
		return model.NoteComment{}, errs.Wrap("n.api.GetNoteComment", fromStatus(err))
	}

	return toNoteComment(resp.GetComment()), nil
}

// CommentsByID returns the comments of notes that exist, in no order,
// with the beginnings of their texts.
func (n *Notes) CommentsByID(ctx context.Context, ids []string) ([]model.NoteComment, error) {
	resp, err := n.api.GetNoteComments(ctx, &postv1.GetNoteCommentsRequest{Ids: ids, TextLimit: model.ExcerptLength})
	if err != nil {
		return nil, errs.Wrap("n.api.GetNoteComments", fromStatus(err))
	}

	comments := make([]model.NoteComment, 0, len(resp.GetComments()))
	for _, c := range resp.GetComments() {
		comments = append(comments, toNoteComment(c))
	}

	return comments, nil
}

func toNote(pn *postv1.Note) model.Note {
	return model.Note{
		ID:        pn.GetId(),
		AuthorID:  pn.GetAuthorId(),
		Title:     pn.GetTitle(),
		Text:      pn.GetText(),
		TextCut:   pn.GetTextCut(),
		Comments:  int(pn.GetCommentCount()),
		CreatedAt: fromTimestamp(pn.GetCreatedAt()),
		EditedAt:  fromTimestamp(pn.GetEditedAt()),
		Mine:      pn.GetMine(),
	}
}

func toNoteComment(pc *postv1.NoteComment) model.NoteComment {
	return model.NoteComment{
		ID:        pc.GetId(),
		NoteID:    pc.GetNoteId(),
		AuthorID:  pc.GetAuthorId(),
		Text:      pc.GetText(),
		TextCut:   pc.GetTextCut(),
		CreatedAt: fromTimestamp(pc.GetCreatedAt()),
		Mine:      pc.GetMine(),
	}
}

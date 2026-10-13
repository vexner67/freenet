package service

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// Pages knows the pages their owners deleted and may still restore. What
// such a page wrote nobody is shown: the lists and the counts leave it
// out in the stores, and what is read by its ID asks here, so the answer
// does not depend on whoever calls hiding it.
type Pages interface {
	// Deleted tells which of the users deleted their page.
	Deleted(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// deletedPages tells which of the users deleted their page; without
// pages nobody did. uuid.Nil stands for no one and is not asked about.
func deletedPages(ctx context.Context, pages Pages, userIDs ...uuid.UUID) (map[uuid.UUID]bool, error) {
	userIDs = slices.DeleteFunc(userIDs, func(id uuid.UUID) bool { return id == uuid.Nil })

	if pages == nil || len(userIDs) == 0 {
		return nil, nil
	}

	gone, err := pages.Deleted(ctx, userIDs)
	if err != nil {
		return nil, errs.Wrap("pages.Deleted", err)
	}

	return gone, nil
}

// written leaves out of items those a deleted page wrote. writer is whose
// words an item is: uuid.Nil for what speaks for a community, which is
// the community's.
func written[T any](ctx context.Context, pages Pages, items []T, writer func(T) uuid.UUID) ([]T, error) {
	writers := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		writers = append(writers, writer(item))
	}

	gone, err := deletedPages(ctx, pages, writers...)
	if err != nil {
		return nil, errs.Wrap("deletedPages", err)
	}

	if len(gone) == 0 {
		return items, nil
	}

	return slices.DeleteFunc(items, func(item T) bool { return gone[writer(item)] }), nil
}

// own is the author of what was written in their own name; uuid.Nil for
// what speaks for a wall.
func own(authorID uuid.UUID, fromWall bool) uuid.UUID {
	if fromWall {
		return uuid.Nil
	}

	return authorID
}

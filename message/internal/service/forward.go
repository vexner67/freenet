package service

import (
	"bytes"
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// forwarder reads the messages a user carries on ("Переслать"), from
// their dialogs and from their chats: a message of a chat goes into
// either, one of a dialog only into a dialog (dialogs), since what it
// says is sealed for two.
type forwarder struct {
	messages     MessageRepository
	chatMessages ChatMessageRepository
	members      ChatMemberRepository
	dialogs      bool
}

// photoCopies are the photos of one forwarded message to keep once more.
type photoCopies struct {
	ownerID uuid.UUID
	photos  []PhotoCopy
}

// forwarded reads the messages userID forwards, oldest first, as a new
// message carries them on: only what userID still has in their dialogs
// and chats and what was not taken back. The photos get new IDs: copies,
// so that they outlive the messages they were sent in.
func (f forwarder) forwarded(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (message.Forwards, []photoCopies, error) {
	if len(ids) > message.MaxForwarded {
		return nil, nil, message.ErrTooManyForwarded
	}

	// UUIDv7 bytes sort by time; the same message twice is forwarded once.
	ids = slices.Clone(ids)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	ids = slices.Compact(ids)

	var (
		carried []message.Forwarded
		copies  []photoCopies
	)

	for _, id := range ids {
		one, err := f.carried(ctx, userID, id)
		if err != nil {
			return nil, nil, errs.Wrap("f.carried", err)
		}

		for _, c := range one {
			pc, err := copyPhotos(&c)
			if err != nil {
				return nil, nil, errs.Wrap("copyPhotos", err)
			}

			carried = append(carried, c)
			if len(pc.photos) > 0 {
				copies = append(copies, pc)
			}
		}
	}

	forwards, err := message.NewForwards(carried)
	if err != nil {
		return nil, nil, errs.Wrap("message.NewForwards", err)
	}

	return forwards, copies, nil
}

// carried is message id of a dialog or of a chat as userID carries it on;
// message.ErrForwardNotFound for one they do not have or that says
// nothing.
func (f forwarder) carried(ctx context.Context, userID, id uuid.UUID) ([]message.Forwarded, error) {
	m, err := f.messages.Get(ctx, id)
	if err == nil {
		if !m.VisibleTo(userID) || m.IsDeleted() {
			return nil, message.ErrForwardNotFound
		}

		if !f.dialogs {
			return nil, message.ErrForwardIntoChat
		}

		return m.Carried(), nil
	}

	if !errors.Is(err, message.ErrNotFound) {
		return nil, errs.Wrap("f.messages.Get", err)
	}

	cm, err := f.chatMessages.Get(ctx, id)
	if errors.Is(err, message.ErrNotFound) {
		return nil, message.ErrForwardNotFound
	}
	if err != nil {
		return nil, errs.Wrap("f.chatMessages.Get", err)
	}

	me, known, err := f.members.Get(ctx, cm.ChatID(), userID)
	if err != nil {
		return nil, errs.Wrap("f.members.Get", err)
	}

	carried := cm.Carried()
	if !known || !me.Sees(id) || len(carried) == 0 {
		return nil, message.ErrForwardNotFound
	}

	return carried, nil
}

// copyPhotos gives the photos of f new IDs and tells what to copy.
func copyPhotos(f *message.Forwarded) (photoCopies, error) {
	c := photoCopies{ownerID: f.SenderID}
	if len(f.Photos) == 0 {
		return c, nil
	}

	fresh := make(message.Photos, len(f.Photos))
	for i, from := range f.Photos {
		to, err := uuid.NewV7()
		if err != nil {
			return photoCopies{}, errs.Wrap("uuid.NewV7", err)
		}

		fresh[i] = to
		c.photos = append(c.photos, PhotoCopy{From: from, To: to})
	}

	f.Photos = fresh

	return c, nil
}

// keepPhotos has media keep the photos a sent message shows for good and
// those of the messages it carries on once more, with the message.
func keepPhotos(ctx context.Context, keeper PhotoKeeper, senderID uuid.UUID, photos message.Photos, copies []photoCopies) error {
	if len(photos) > 0 {
		if err := keeper.Publish(ctx, senderID, photos); err != nil {
			return errs.Wrap("keeper.Publish", err)
		}
	}

	for _, c := range copies {
		if err := keeper.Copy(ctx, c.ownerID, c.photos); err != nil {
			return errs.Wrap("keeper.Copy", err)
		}
	}

	return nil
}

// dropPhotos has media delete the photos a message showed, its own and
// the copies it carried on, except those of the user except: media erases
// everything of a user whose page is gone by itself.
func dropPhotos(ctx context.Context, keeper PhotoKeeper, senderID uuid.UUID, photos message.Photos, forwarded message.Forwards, except uuid.UUID) error {
	if len(photos) > 0 && senderID != except {
		if err := keeper.Delete(ctx, senderID, photos); err != nil {
			return errs.Wrap("keeper.Delete", err)
		}
	}

	for _, f := range forwarded {
		if len(f.Photos) == 0 || f.SenderID == except {
			continue
		}

		if err := keeper.Delete(ctx, f.SenderID, f.Photos); err != nil {
			return errs.Wrap("keeper.Delete", err)
		}
	}

	return nil
}

// movePhotos keeps the photos an edit added for good and deletes those it
// removed, with the edit.
func movePhotos(ctx context.Context, keeper PhotoKeeper, ownerID uuid.UUID, added, removed []uuid.UUID) error {
	if len(added) > 0 {
		if err := keeper.Publish(ctx, ownerID, added); err != nil {
			return errs.Wrap("keeper.Publish", err)
		}
	}

	if len(removed) > 0 {
		if err := keeper.Delete(ctx, ownerID, removed); err != nil {
			return errs.Wrap("keeper.Delete", err)
		}
	}

	return nil
}

// without are the photos of a that b does not have.
func without(a, b message.Photos) []uuid.UUID {
	var out []uuid.UUID

	for _, id := range a {
		if !slices.Contains(b, id) {
			out = append(out, id)
		}
	}

	return out
}

package service

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// ChatRepository keeps chats and tells how each of their people has them.
type ChatRepository interface {
	Create(ctx context.Context, c *message.Chat) error
	// Lock reads the chat and holds it until the transaction ends: what
	// happens in one chat happens in turn. message.ErrChatNotFound.
	Lock(ctx context.Context, id uuid.UUID) (*message.Chat, error)
	// Rename saves the title of the chat.
	Rename(ctx context.Context, c *message.Chat) error
	// View is the chat as userID has it; message.ErrChatNotFound for
	// someone who was never in it.
	View(ctx context.Context, chatID, userID uuid.UUID) (message.ChatView, error)
	// List returns up to limit chats of a user whose last message is below
	// before (uuid.Nil: from the newest), most recent first.
	List(ctx context.Context, userID, before uuid.UUID, limit int) ([]message.ChatView, error)
	// CountUnread counts, up to limit, the chats with unread messages.
	CountUnread(ctx context.Context, userID uuid.UUID, limit int) (int, error)
	// DeleteByUser takes a user out of every chat with what they wrote
	// there; a chat they created goes to who has been in it longest. It
	// returns the deleted messages that carried other messages on.
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.ChatMessage, error)
}

// ChatMemberRepository keeps who is in a chat and how far they have read.
type ChatMemberRepository interface {
	// Get tells how userID stands in the chat; false for someone never in it.
	Get(ctx context.Context, chatID, userID uuid.UUID) (message.Member, bool, error)
	// Active are the people in the chat now, who joined first first.
	Active(ctx context.Context, chatID uuid.UUID) ([]message.Member, error)
	// Set puts userID in the chat as state says, new or known.
	Set(ctx context.Context, chatID, userID uuid.UUID, state message.MemberState, now time.Time) error
	// MarkRead moves the member's mark to upTo, never back.
	MarkRead(ctx context.Context, chatID, userID, upTo uuid.UUID) error
}

// ChatMessageRepository keeps the messages of chats.
type ChatMessageRepository interface {
	// Save stores the message and brings everyone in the chat up to it.
	Save(ctx context.Context, m *message.ChatMessage) error
	// Get returns message.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*message.ChatMessage, error)
	// ListBefore returns up to limit messages of the chat with IDs below
	// before (uuid.Nil: from the newest) and not above upTo, newest first.
	ListBefore(ctx context.Context, chatID, before, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error)
	// ListAfter returns up to limit such messages with IDs above after,
	// oldest first.
	ListAfter(ctx context.Context, chatID, after, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error)
	// Update saves what an edited message says and shows;
	// message.ErrNotFound.
	Update(ctx context.Context, m *message.ChatMessage) error
	// Search returns up to limit messages people wrote that userID is
	// shown and that say q, in the chat or, for uuid.Nil, in all of
	// theirs, newest first, with IDs below before (uuid.Nil: from the
	// newest).
	Search(ctx context.Context, userID, chatID uuid.UUID, q message.Query, before uuid.UUID, limit int) ([]*message.ChatMessage, error)
	// Retract saves a message taken back; message.ErrNotFound if it is gone
	// or was taken back already.
	Retract(ctx context.Context, m *message.ChatMessage) error
}

// ChatEvent is what happened in a chat, told to each of its people: a new
// message, written or telling of what someone did, a message that says
// something else now, or how far the one who hears it has read on
// another page.
type ChatEvent struct {
	Message *message.ChatMessage
	Edited  *message.ChatMessage
	Read    *ChatRead
}

// ChatRead: whoever hears it has read the chat up to UpTo.
type ChatRead struct {
	ChatID uuid.UUID
	UpTo   uuid.UUID
}

// ChatDetails is a chat as one of its people opens it; Members are those
// in it now, and nobody for who is not in it anymore.
type ChatDetails struct {
	View    message.ChatView
	Members []message.Member
}

// ChatPage is one screen of chats; NextCursor is uuid.Nil on the last.
type ChatPage struct {
	Chats      []message.ChatView
	NextCursor uuid.UUID
}

// ChatHistoryPage is a run of messages of a chat, oldest first, as Me has
// them. NextCursor pages to older messages and is uuid.Nil when there are
// none.
type ChatHistoryPage struct {
	Messages   []*message.ChatMessage
	Me         message.Member
	NextCursor uuid.UUID
}

// ChatSearchPage is one screen of found messages of chats, newest first;
// NextCursor is uuid.Nil on the last.
type ChatSearchPage struct {
	Messages   []*message.ChatMessage
	NextCursor uuid.UUID
}

// ChatDeps are what ChatService works with. DialogMessages are read when
// a message of a dialog is carried on into a chat.
type ChatDeps struct {
	Chats          ChatRepository
	Members        ChatMemberRepository
	Messages       ChatMessageRepository
	DialogMessages MessageRepository
	Events         Events
	Tx             TxManager
	Photos         PhotoKeeper
}

// ChatService is the chats of several people ("беседы" of openvk).
type ChatService struct {
	chatRepo    ChatRepository
	memberRepo  ChatMemberRepository
	messageRepo ChatMessageRepository
	events      Events
	tx          TxManager
	photos      PhotoKeeper
	forwards    forwarder
	now         func() time.Time
}

func NewChatService(d ChatDeps) *ChatService {
	return &ChatService{
		chatRepo:    d.Chats,
		memberRepo:  d.Members,
		messageRepo: d.Messages,
		events:      d.Events,
		tx:          d.Tx,
		photos:      d.Photos,
		forwards:    forwarder{messages: d.DialogMessages, chatMessages: d.Messages, members: d.Members},
		now:         time.Now,
	}
}

// Create starts a chat of ownerID with memberIDs, two others at least;
// the caller has made sure they exist and may be added. Everyone hears
// that the chat was created.
func (s *ChatService) Create(ctx context.Context, ownerID uuid.UUID, title message.Title, memberIDs []uuid.UUID) (message.ChatView, error) {
	invited, err := message.Invited(ownerID, memberIDs)
	if err != nil {
		return message.ChatView{}, errs.Wrap("message.Invited", err)
	}

	now := s.now()

	chat, err := message.NewChat(ownerID, title, now)
	if err != nil {
		return message.ChatView{}, errs.Wrap("message.NewChat", err)
	}

	created, err := message.NewChatAction(chat.ID(), ownerID, message.ActionCreate, uuid.Nil, title, now)
	if err != nil {
		return message.ChatView{}, errs.Wrap("message.NewChatAction", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.chatRepo.Create(ctx, chat); err != nil {
			return errs.Wrap("s.chatRepo.Create", err)
		}

		for _, id := range append([]uuid.UUID{ownerID}, invited...) {
			if err := s.memberRepo.Set(ctx, chat.ID(), id, message.MemberActive, now); err != nil {
				return errs.Wrap("s.memberRepo.Set", err)
			}
		}

		return errs.Wrap("s.messageRepo.Save", s.messageRepo.Save(ctx, created))
	})
	if err != nil {
		return message.ChatView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	s.tell(ctx, Event{Chat: &ChatEvent{Message: created}})

	view, err := s.chatRepo.View(ctx, chat.ID(), ownerID)

	return view, errs.Wrap("s.chatRepo.View", err)
}

// Get is the chat as userID has it, with who is in it;
// message.ErrChatNotFound for someone who was never in it.
func (s *ChatService) Get(ctx context.Context, userID, chatID uuid.UUID) (ChatDetails, error) {
	view, err := s.chatRepo.View(ctx, chatID, userID)
	if err != nil {
		return ChatDetails{}, errs.Wrap("s.chatRepo.View", err)
	}

	details := ChatDetails{View: view}
	if !view.Me.IsActive() {
		return details, nil
	}

	if details.Members, err = s.memberRepo.Active(ctx, chatID); err != nil {
		return ChatDetails{}, errs.Wrap("s.memberRepo.Active", err)
	}

	return details, nil
}

// Chats pages a user's chats, the most recently written first; cursor is
// the last message of the previous page's last chat.
func (s *ChatService) Chats(ctx context.Context, userID, cursor uuid.UUID, size int) (ChatPage, error) {
	size = pageSize(size)

	chats, err := s.chatRepo.List(ctx, userID, cursor, size+1)
	if err != nil {
		return ChatPage{}, errs.Wrap("s.chatRepo.List", err)
	}

	page := ChatPage{Chats: chats}

	if len(chats) > size {
		page.Chats = chats[:size]
		page.NextCursor = chats[size-1].LastMessage.ID()
	}

	return page, nil
}

// CountUnread counts up to MaxUnreadCount chats with unread messages.
func (s *ChatService) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := s.chatRepo.CountUnread(ctx, userID, MaxUnreadCount)

	return count, errs.Wrap("s.chatRepo.CountUnread", err)
}

// change is one thing done to a chat by one of its people: check tells
// whether it may be done and what to tell the chat of it, apply does it.
// before says whether the chat hears of it before it is done: who goes
// still hears that they went.
type change struct {
	check  func(ctx context.Context, chat *message.Chat, me message.Member) ([]*message.ChatMessage, error)
	apply  func(ctx context.Context, chat *message.Chat) error
	before bool
}

// act does c to the chat as userID, who is in it, and tells everyone.
func (s *ChatService) act(ctx context.Context, userID, chatID uuid.UUID, c change) error {
	var told []*message.ChatMessage

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		chat, me, err := s.locked(ctx, chatID, userID)
		if err != nil {
			return errs.Wrap("s.locked", err)
		}

		if told, err = c.check(ctx, chat, me); err != nil {
			return errs.Wrap("c.check", err)
		}

		if c.before {
			if err = s.save(ctx, told); err != nil {
				return errs.Wrap("s.save", err)
			}
		}

		if err = c.apply(ctx, chat); err != nil {
			return errs.Wrap("c.apply", err)
		}

		if !c.before {
			return errs.Wrap("s.save", s.save(ctx, told))
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	for _, m := range told {
		s.tell(ctx, Event{Chat: &ChatEvent{Message: m}}, m.SenderID(), m.TargetID())
	}

	return nil
}

// locked holds the chat for the transaction and tells how userID stands
// in it; message.ErrChatNotFound for someone who was never in it.
func (s *ChatService) locked(ctx context.Context, chatID, userID uuid.UUID) (*message.Chat, message.Member, error) {
	chat, err := s.chatRepo.Lock(ctx, chatID)
	if err != nil {
		return nil, message.Member{}, errs.Wrap("s.chatRepo.Lock", err)
	}

	me, known, err := s.memberRepo.Get(ctx, chatID, userID)
	if err != nil {
		return nil, message.Member{}, errs.Wrap("s.memberRepo.Get", err)
	}

	if !known {
		return nil, message.Member{}, message.ErrChatNotFound
	}

	return chat, me, nil
}

func (s *ChatService) save(ctx context.Context, messages []*message.ChatMessage) error {
	for _, m := range messages {
		if err := s.messageRepo.Save(ctx, m); err != nil {
			return errs.Wrap("s.messageRepo.Save", err)
		}
	}

	return nil
}

// tell publishes e to everyone in the chat now and to also, who may have
// just gone.
func (s *ChatService) tell(ctx context.Context, e Event, also ...uuid.UUID) {
	chatID := e.Chat.chatID()

	members, err := s.memberRepo.Active(ctx, chatID)
	if err != nil {
		// Best effort, as every event: the pages catch up from storage.
		return
	}

	heard := make(map[uuid.UUID]bool, len(members)+len(also))

	for _, m := range members {
		heard[m.UserID] = true
		s.events.Publish(ctx, m.UserID, e)
	}

	for _, id := range also {
		if id != uuid.Nil && !heard[id] {
			heard[id] = true
			s.events.Publish(ctx, id, e)
		}
	}
}

func (e *ChatEvent) chatID() uuid.UUID {
	switch {
	case e.Message != nil:
		return e.Message.ChatID()
	case e.Edited != nil:
		return e.Edited.ChatID()
	default:
		return e.Read.ChatID
	}
}

// Rename calls the chat title: anyone in it may.
func (s *ChatService) Rename(ctx context.Context, userID, chatID uuid.UUID, title message.Title) error {
	now := s.now()

	return s.act(ctx, userID, chatID, change{
		check: func(_ context.Context, chat *message.Chat, me message.Member) ([]*message.ChatMessage, error) {
			if err := me.CheckActive(); err != nil {
				return nil, errs.Wrap("me.CheckActive", err)
			}

			if chat.Title() == title {
				return nil, nil
			}

			renamed, err := message.NewChatAction(chatID, userID, message.ActionTitle, uuid.Nil, title, now)

			return []*message.ChatMessage{renamed}, errs.Wrap("message.NewChatAction", err)
		},
		apply: func(ctx context.Context, chat *message.Chat) error {
			return errs.Wrap("s.chatRepo.Rename", s.chatRepo.Rename(ctx, chat.Renamed(title)))
		},
	})
}

// Invite adds people to the chat: anyone in it may, while there is room;
// the caller has made sure they exist and may be added. Who is in it
// already stays, who left on their own is not brought back
// (message.ErrMemberLeft).
func (s *ChatService) Invite(ctx context.Context, userID, chatID uuid.UUID, ids []uuid.UUID) error {
	now := s.now()

	var admitted []uuid.UUID

	return s.act(ctx, userID, chatID, change{
		check: func(ctx context.Context, _ *message.Chat, me message.Member) ([]*message.ChatMessage, error) {
			if err := me.CheckActive(); err != nil {
				return nil, errs.Wrap("me.CheckActive", err)
			}

			var err error
			if admitted, err = s.admitted(ctx, chatID, userID, ids); err != nil {
				return nil, errs.Wrap("s.admitted", err)
			}

			told := make([]*message.ChatMessage, 0, len(admitted))

			for _, id := range admitted {
				invited, err := message.NewChatAction(chatID, userID, message.ActionInvite, id, "", now)
				if err != nil {
					return nil, errs.Wrap("message.NewChatAction", err)
				}

				told = append(told, invited)
			}

			return told, nil
		},
		apply: func(ctx context.Context, _ *message.Chat) error {
			for _, id := range admitted {
				if err := s.memberRepo.Set(ctx, chatID, id, message.MemberActive, now); err != nil {
					return errs.Wrap("s.memberRepo.Set", err)
				}
			}

			return nil
		},
	})
}

// admitted are those of ids who join the locked chat, in the order
// named; message.ErrChatFull when they do not all fit.
func (s *ChatService) admitted(ctx context.Context, chatID, userID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	var admitted []uuid.UUID

	for _, id := range ids {
		if id == userID || id == uuid.Nil || slices.Contains(admitted, id) {
			continue
		}

		m, known, err := s.memberRepo.Get(ctx, chatID, id)
		if err != nil {
			return nil, errs.Wrap("s.memberRepo.Get", err)
		}

		admission, err := message.Admit(m, known)
		if err != nil {
			return nil, errs.Wrap("message.Admit", err)
		}

		if admission == message.Admitted {
			admitted = append(admitted, id)
		}
	}

	active, err := s.memberRepo.Active(ctx, chatID)
	if err != nil {
		return nil, errs.Wrap("s.memberRepo.Active", err)
	}

	if len(active)+len(admitted) > message.MaxChatMembers {
		return nil, message.ErrChatFull
	}

	return admitted, nil
}

// Kick removes targetID from the chat: only its creator may.
func (s *ChatService) Kick(ctx context.Context, userID, chatID, targetID uuid.UUID) error {
	now := s.now()

	return s.act(ctx, userID, chatID, change{
		before: true,
		check: func(ctx context.Context, chat *message.Chat, me message.Member) ([]*message.ChatMessage, error) {
			if err := me.CheckActive(); err != nil {
				return nil, errs.Wrap("me.CheckActive", err)
			}

			if err := chat.CheckKick(userID, targetID); err != nil {
				return nil, errs.Wrap("chat.CheckKick", err)
			}

			target, known, err := s.memberRepo.Get(ctx, chatID, targetID)
			if err != nil {
				return nil, errs.Wrap("s.memberRepo.Get", err)
			}

			if !known || !target.IsActive() {
				return nil, message.ErrMemberNotFound
			}

			kicked, err := message.NewChatAction(chatID, userID, message.ActionKick, targetID, "", now)

			return []*message.ChatMessage{kicked}, errs.Wrap("message.NewChatAction", err)
		},
		apply: func(ctx context.Context, _ *message.Chat) error {
			return errs.Wrap("s.memberRepo.Set", s.memberRepo.Set(ctx, chatID, targetID, message.MemberKicked, now))
		},
	})
}

// Leave takes userID out of the chat on their own ("Покинуть чат"): they
// keep what was said until then and may come back.
func (s *ChatService) Leave(ctx context.Context, userID, chatID uuid.UUID) error {
	now := s.now()

	return s.act(ctx, userID, chatID, change{
		before: true,
		check: func(_ context.Context, _ *message.Chat, me message.Member) ([]*message.ChatMessage, error) {
			if err := me.CheckActive(); err != nil {
				return nil, errs.Wrap("me.CheckActive", err)
			}

			left, err := message.NewChatAction(chatID, userID, message.ActionLeave, uuid.Nil, "", now)

			return []*message.ChatMessage{left}, errs.Wrap("message.NewChatAction", err)
		},
		apply: func(ctx context.Context, _ *message.Chat) error {
			return errs.Wrap("s.memberRepo.Set", s.memberRepo.Set(ctx, chatID, userID, message.MemberLeft, now))
		},
	})
}

// Return brings userID back into a chat they left ("Вернуться в чат"),
// while there is room; who was removed waits to be added
// (message.ErrKicked).
func (s *ChatService) Return(ctx context.Context, userID, chatID uuid.UUID) error {
	now := s.now()
	back := false

	return s.act(ctx, userID, chatID, change{
		check: func(ctx context.Context, _ *message.Chat, me message.Member) ([]*message.ChatMessage, error) {
			switch me.State {
			case message.MemberActive:
				return nil, nil
			case message.MemberKicked:
				return nil, message.ErrKicked
			}

			active, err := s.memberRepo.Active(ctx, chatID)
			if err != nil {
				return nil, errs.Wrap("s.memberRepo.Active", err)
			}

			if len(active) >= message.MaxChatMembers {
				return nil, message.ErrChatFull
			}

			back = true

			returned, err := message.NewChatAction(chatID, userID, message.ActionReturn, uuid.Nil, "", now)

			return []*message.ChatMessage{returned}, errs.Wrap("message.NewChatAction", err)
		},
		apply: func(ctx context.Context, _ *message.Chat) error {
			if !back {
				return nil
			}

			return errs.Wrap("s.memberRepo.Set", s.memberRepo.Set(ctx, chatID, userID, message.MemberActive, now))
		},
	})
}

// Send writes into the chat as senderID, who is in it: what a message of
// a dialog may be (Draft). The caller has made sure the photos are the
// sender's and what is shared exists. Media keeps the photos, and those
// of the forwarded messages once more. Everyone in the chat hears of it.
func (s *ChatService) Send(ctx context.Context, senderID, chatID uuid.UUID, d Draft) (*message.ChatMessage, error) {
	forwarded, copies, err := s.forwards.forwarded(ctx, senderID, d.Forward)
	if err != nil {
		return nil, errs.Wrap("s.forwards.forwarded", err)
	}

	content, err := d.content(forwarded)
	if err != nil {
		return nil, errs.Wrap("d.content", err)
	}

	m, err := message.NewChatMessage(chatID, senderID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("message.NewChatMessage", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		_, me, err := s.locked(ctx, chatID, senderID)
		if err != nil {
			return errs.Wrap("s.locked", err)
		}

		if err = me.CheckActive(); err != nil {
			return errs.Wrap("me.CheckActive", err)
		}

		if err = s.messageRepo.Save(ctx, m); err != nil {
			return errs.Wrap("s.messageRepo.Save", err)
		}

		return errs.Wrap("keepPhotos", keepPhotos(ctx, s.photos, senderID, m.Photos(), copies))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	s.tell(ctx, Event{Chat: &ChatEvent{Message: m}})

	return m, nil
}

// own reads message id for userID, who is in its chat now;
// message.ErrNotFound for anyone the message is not shown to.
func (s *ChatService) own(ctx context.Context, userID, id uuid.UUID) (*message.ChatMessage, error) {
	m, me, err := s.shown(ctx, userID, id)
	if err != nil {
		return nil, errs.Wrap("s.shown", err)
	}

	if err = me.CheckActive(); err != nil {
		return nil, errs.Wrap("me.CheckActive", err)
	}

	return m, nil
}

// shown reads message id as userID has it: message.ErrNotFound for anyone
// who was never in its chat or went before it was written.
func (s *ChatService) shown(ctx context.Context, userID, id uuid.UUID) (*message.ChatMessage, message.Member, error) {
	m, err := s.messageRepo.Get(ctx, id)
	if err != nil {
		return nil, message.Member{}, errs.Wrap("s.messageRepo.Get", err)
	}

	me, known, err := s.memberRepo.Get(ctx, m.ChatID(), userID)
	if err != nil {
		return nil, message.Member{}, errs.Wrap("s.memberRepo.Get", err)
	}

	if !known || !me.Sees(id) {
		return nil, message.Member{}, message.ErrNotFound
	}

	return m, me, nil
}

// Message is a message of a chat, whole, as userID has it.
func (s *ChatService) Message(ctx context.Context, userID, id uuid.UUID) (*message.ChatMessage, message.Member, error) {
	m, me, err := s.shown(ctx, userID, id)

	return m, me, errs.Wrap("s.shown", err)
}

// Edit changes what a message says and the photos it shows: only its
// sender, still in the chat, for a day after sending. The caller has made
// sure new photos are the sender's; media keeps them and deletes those
// the edit took away. The chat is held while the message is read and
// saved, so two edits made at once go one after the other. Everyone in
// the chat hears of it.
func (s *ChatService) Edit(ctx context.Context, userID, id uuid.UUID, rev message.Revision) (*message.ChatMessage, error) {
	was, err := s.own(ctx, userID, id)
	if err != nil {
		return nil, errs.Wrap("s.own", err)
	}

	var edited *message.ChatMessage

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, _, err := s.locked(ctx, was.ChatID(), userID); err != nil {
			return errs.Wrap("s.locked", err)
		}

		m, err := s.messageRepo.Get(ctx, id)
		if err != nil {
			return errs.Wrap("s.messageRepo.Get", err)
		}

		if edited, err = m.Edited(userID, rev, s.now()); err != nil {
			return errs.Wrap("m.Edited", err)
		}

		if err = s.messageRepo.Update(ctx, edited); err != nil {
			return errs.Wrap("s.messageRepo.Update", err)
		}

		return errs.Wrap("movePhotos", movePhotos(ctx, s.photos, userID, without(edited.Photos(), m.Photos()), without(m.Photos(), edited.Photos())))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	s.tell(ctx, Event{Chat: &ChatEvent{Edited: edited}})

	return edited, nil
}

// Delete takes a message back from the chat: only its sender, still in
// the chat, for a day after sending. It stays there saying nothing; media
// deletes its photos and the copies it carried on. Everyone in the chat
// hears what it is now.
func (s *ChatService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	m, err := s.own(ctx, userID, id)
	if err != nil {
		return errs.Wrap("s.own", err)
	}

	gone, err := m.TakenBack(userID, s.now())
	if err != nil {
		return errs.Wrap("m.TakenBack", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.messageRepo.Retract(ctx, gone); err != nil {
			return errs.Wrap("s.messageRepo.Retract", err)
		}

		return errs.Wrap("dropPhotos", dropPhotos(ctx, s.photos, userID, m.Photos(), m.Forwarded(), uuid.Nil))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	s.tell(ctx, Event{Chat: &ChatEvent{Edited: gone}})

	return nil
}

// member tells how userID stands in the chat; message.ErrChatNotFound for
// someone who was never in it.
func (s *ChatService) member(ctx context.Context, chatID, userID uuid.UUID) (message.Member, error) {
	me, known, err := s.memberRepo.Get(ctx, chatID, userID)
	if err != nil {
		return message.Member{}, errs.Wrap("s.memberRepo.Get", err)
	}

	if !known {
		return message.Member{}, message.ErrChatNotFound
	}

	return me, nil
}

// History pages the chat back in time as userID has it: cursor is the
// oldest message of the previous page (uuid.Nil: from the newest).
func (s *ChatService) History(ctx context.Context, userID, chatID, cursor uuid.UUID, size int) (ChatHistoryPage, error) {
	me, err := s.member(ctx, chatID, userID)
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.member", err)
	}

	size = pageSize(size)

	newestFirst, err := s.messageRepo.ListBefore(ctx, chatID, cursor, me.Bound(), size+1)
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.messageRepo.ListBefore", err)
	}

	page := ChatHistoryPage{Messages: newestFirst, Me: me}

	if len(newestFirst) > size {
		page.Messages = newestFirst[:size]
		page.NextCursor = newestFirst[size-1].ID()
	}

	slices.Reverse(page.Messages)

	return page, nil
}

// Newer returns up to size messages of the chat after the given one,
// oldest first: what came since the page last looked.
func (s *ChatService) Newer(ctx context.Context, userID, chatID, after uuid.UUID, size int) (ChatHistoryPage, error) {
	me, err := s.member(ctx, chatID, userID)
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.member", err)
	}

	messages, err := s.messageRepo.ListAfter(ctx, chatID, after, me.Bound(), pageSize(size))
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.messageRepo.ListAfter", err)
	}

	return ChatHistoryPage{Messages: messages, Me: me}, nil
}

// Around returns the part of the chat around message id, which userID is
// shown in it: about half of size before it and half after, oldest first,
// so that a found message opens where it was said. NextCursor pages to
// older messages; newer ones come with Newer.
func (s *ChatService) Around(ctx context.Context, userID, chatID, id uuid.UUID, size int) (ChatHistoryPage, error) {
	m, me, err := s.shown(ctx, userID, id)
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.shown", err)
	}

	if m.ChatID() != chatID {
		return ChatHistoryPage{}, message.ErrNotFound
	}

	half := max(pageSize(size)/2, 1)

	older, err := s.messageRepo.ListBefore(ctx, chatID, id, me.Bound(), half+1)
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.messageRepo.ListBefore", err)
	}

	newer, err := s.messageRepo.ListAfter(ctx, chatID, id, me.Bound(), half)
	if err != nil {
		return ChatHistoryPage{}, errs.Wrap("s.messageRepo.ListAfter", err)
	}

	page := ChatHistoryPage{Me: me}

	if len(older) > half {
		older = older[:half]
		page.NextCursor = older[half-1].ID()
	}

	slices.Reverse(older)
	page.Messages = append(append(older, m), newer...)

	return page, nil
}

// Search pages the messages userID is shown in their chats that say q,
// newest first: in one chat, or in all of theirs for uuid.Nil.
func (s *ChatService) Search(ctx context.Context, userID, chatID uuid.UUID, q message.Query, cursor uuid.UUID, size int) (ChatSearchPage, error) {
	if chatID != uuid.Nil {
		if _, err := s.member(ctx, chatID, userID); err != nil {
			return ChatSearchPage{}, errs.Wrap("s.member", err)
		}
	}

	size = pageSize(size)

	found, err := s.messageRepo.Search(ctx, userID, chatID, q, cursor, size+1)
	if err != nil {
		return ChatSearchPage{}, errs.Wrap("s.messageRepo.Search", err)
	}

	page := ChatSearchPage{Messages: found}
	if len(found) > size {
		page.Messages = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
}

// MarkRead reads the chat up to upTo, a message of it the user is shown:
// an ID made up "in the future" would mark every message still to come
// as read. The user's other pages hear of it.
func (s *ChatService) MarkRead(ctx context.Context, userID, chatID, upTo uuid.UUID) error {
	m, _, err := s.shown(ctx, userID, upTo)
	if err != nil {
		return errs.Wrap("s.shown", err)
	}

	if m.ChatID() != chatID {
		return message.ErrNotFound
	}

	if err = s.memberRepo.MarkRead(ctx, chatID, userID, upTo); err != nil {
		return errs.Wrap("s.memberRepo.MarkRead", err)
	}

	s.events.Publish(ctx, userID, Event{Chat: &ChatEvent{Read: &ChatRead{ChatID: chatID, UpTo: upTo}}})

	return nil
}

// PurgeUser takes a user whose page was deleted for good out of every
// chat, with what they wrote there; their photos media erases with the
// rest of the user, the copies of others' photos their messages carried
// on go here.
func (s *ChatService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		carrying, err := s.chatRepo.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.chatRepo.DeleteByUser", err)
		}

		for _, m := range carrying {
			if err = dropPhotos(ctx, s.photos, m.SenderID(), nil, m.Forwarded(), userID); err != nil {
				return errs.Wrap("dropPhotos", err)
			}
		}

		return nil
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

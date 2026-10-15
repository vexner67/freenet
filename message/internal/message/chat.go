package message

import (
	"bytes"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxTitleLen = 64

	// MaxChatMembers is how many people one chat holds, its creator among
	// them.
	MaxChatMembers = 50

	// MinChatInvited is how many others a chat starts with, as on openvk:
	// "В беседе должно быть как минимум два других пользователя". Two
	// people have a dialog.
	MinChatInvited = 2
)

var (
	// ErrChatNotFound: no such chat, or the user was never in it.
	ErrChatNotFound = errors.New("chat not found")
	ErrTitleTooLong = errors.New("chat title is too long")
	// ErrTooFewMembers: a chat starts with MinChatInvited others.
	ErrTooFewMembers = errors.New("a chat needs at least two other people")
	ErrChatFull      = errors.New("the chat is full")
	// ErrNotInChat: the user has left the chat or was removed from it:
	// they read what was said before and do nothing else.
	ErrNotInChat = errors.New("you are not in this chat")
	// ErrCannotKick: only the creator removes others.
	ErrCannotKick   = errors.New("only the creator can remove members")
	ErrKickYourself = errors.New("you cannot remove yourself, leave the chat instead")
	// ErrMemberNotFound: whoever is to be removed is not in the chat.
	ErrMemberNotFound = errors.New("member not found")
	// ErrMemberLeft: who left on their own comes back on their own.
	ErrMemberLeft = errors.New("the user has left the chat")
	// ErrKicked: who was removed comes back when someone adds them.
	ErrKicked = errors.New("you were removed from this chat")
)

// Title is what a chat is called: one line, trimmed. It may be empty: the
// chat is then called by who is in it.
type Title string

func NewTitle(s string) (Title, error) {
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > maxTitleLen {
		return "", ErrTitleTooLong
	}

	return Title(s), nil
}

func (t Title) String() string {
	return string(t)
}

// Chat is a conversation of several people ("беседа" of openvk). Whoever
// created it removes others; everyone in it writes, adds people, renames
// it, sets its photo and pins a message.
type Chat struct {
	id        uuid.UUID
	title     Title
	ownerID   uuid.UUID
	createdAt time.Time
	// photoID is the photo of the chat in the media service, pinnedID the
	// message pinned above it; uuid.Nil for none.
	photoID  uuid.UUID
	pinnedID uuid.UUID
}

func NewChat(ownerID uuid.UUID, title Title, now time.Time) (*Chat, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Chat{id: id, title: title, ownerID: ownerID, createdAt: now.UTC().Truncate(time.Microsecond)}, nil
}

// StoredChat is a chat as storage keeps it.
type StoredChat struct {
	ID        uuid.UUID
	Title     Title
	OwnerID   uuid.UUID
	CreatedAt time.Time
	// PhotoID and PinnedID are uuid.Nil for none.
	PhotoID  uuid.UUID
	PinnedID uuid.UUID
}

// RestoreChat rebuilds a chat from storage without validation or new IDs.
func RestoreChat(s StoredChat) *Chat {
	return &Chat{
		id:        s.ID,
		title:     s.Title,
		ownerID:   s.OwnerID,
		createdAt: s.CreatedAt,
		photoID:   s.PhotoID,
		pinnedID:  s.PinnedID,
	}
}

// Stored is c as storage keeps it.
func (c *Chat) Stored() StoredChat {
	return StoredChat{
		ID:        c.id,
		Title:     c.title,
		OwnerID:   c.ownerID,
		CreatedAt: c.createdAt,
		PhotoID:   c.photoID,
		PinnedID:  c.pinnedID,
	}
}

func (c *Chat) ID() uuid.UUID {
	return c.id
}

func (c *Chat) Title() Title {
	return c.title
}

func (c *Chat) OwnerID() uuid.UUID {
	return c.ownerID
}

func (c *Chat) CreatedAt() time.Time {
	return c.createdAt
}

// PhotoID is the photo of the chat, uuid.Nil for none.
func (c *Chat) PhotoID() uuid.UUID {
	return c.photoID
}

// PinnedID is the message pinned in the chat, uuid.Nil for none.
func (c *Chat) PinnedID() uuid.UUID {
	return c.pinnedID
}

// Renamed returns c called title.
func (c *Chat) Renamed(title Title) *Chat {
	renamed := *c
	renamed.title = title

	return &renamed
}

// WithPhoto returns c showing photo id, uuid.Nil for none.
func (c *Chat) WithPhoto(id uuid.UUID) *Chat {
	changed := *c
	changed.photoID = id

	return &changed
}

// Pinned returns c with m pinned above it, as VK's "Закрепить": one
// message at a time, something a person wrote in this chat and has not
// taken back (ErrNotFound otherwise).
func (c *Chat) Pinned(m *ChatMessage) (*Chat, error) {
	if m.ChatID() != c.id || m.IsAction() || m.IsDeleted() {
		return nil, ErrNotFound
	}

	pinned := *c
	pinned.pinnedID = m.ID()

	return &pinned, nil
}

// Unpinned returns c with nothing pinned.
func (c *Chat) Unpinned() *Chat {
	unpinned := *c
	unpinned.pinnedID = uuid.Nil

	return &unpinned
}

// PinnedFor is the pinned message as me has it: nil when nothing is
// pinned, when me went before it was written or when its sender took it
// back.
func PinnedFor(m *ChatMessage, me Member) *ChatMessage {
	if m == nil || m.IsDeleted() || !me.Sees(m.ID()) {
		return nil
	}

	return m
}

// CheckKick tells whether byID may remove targetID: the creator, anyone
// but themselves.
func (c *Chat) CheckKick(byID, targetID uuid.UUID) error {
	if byID != c.ownerID {
		return ErrCannotKick
	}

	if targetID == byID {
		return ErrKickYourself
	}

	return nil
}

// Invited are the people a new chat of ownerID starts with: each once,
// without the owner, MinChatInvited at least and no more than the chat
// holds.
func Invited(ownerID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	invited := distinct(ids, ownerID)

	if len(invited) < MinChatInvited {
		return nil, ErrTooFewMembers
	}

	if len(invited) > MaxChatMembers-1 {
		return nil, ErrChatFull
	}

	return invited, nil
}

// distinct are ids, each once, without except and without uuid.Nil.
func distinct(ids []uuid.UUID, except uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{except: true, uuid.Nil: true}
	out := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}

	return out
}

// MemberState is how a person stands in a chat.
type MemberState string

const (
	// MemberActive is in the chat.
	MemberActive MemberState = "active"
	// MemberLeft left on their own ("Покинуть чат") and may come back
	// ("Вернуться в чат").
	MemberLeft MemberState = "left"
	// MemberKicked was removed by the creator ("Исключить из беседы").
	MemberKicked MemberState = "kicked"
)

// Member is a person of a chat, now or before. Who is not in it anymore
// keeps the chat as it was when they went: SeenUpTo is the last message
// they are shown.
type Member struct {
	UserID   uuid.UUID
	State    MemberState
	JoinedAt time.Time
	// SeenUpTo is the newest message of the chat the member has; uuid.Nil
	// for none.
	SeenUpTo uuid.UUID
	// LastIncomingID is the newest message by someone else, ReadUpTo the
	// newest of them the member has read; uuid.Nil for none.
	LastIncomingID uuid.UUID
	ReadUpTo       uuid.UUID
}

// IsActive tells whether the member is in the chat now.
func (m Member) IsActive() bool {
	return m.State == MemberActive
}

// CheckActive is ErrNotInChat for who has gone.
func (m Member) CheckActive() error {
	if !m.IsActive() {
		return ErrNotInChat
	}

	return nil
}

// HasUnread tells whether others wrote something the member has not read.
func (m Member) HasUnread() bool {
	return bytes.Compare(m.LastIncomingID[:], m.ReadUpTo[:]) > 0
}

// Sees tells whether the member is shown message id: everything while in
// the chat, and what was there when they went after.
func (m Member) Sees(id uuid.UUID) bool {
	return m.IsActive() || bytes.Compare(id[:], m.SeenUpTo[:]) <= 0
}

// Bound is the newest message the member may be shown: uuid.Max while in
// the chat.
func (m Member) Bound() uuid.UUID {
	if m.IsActive() {
		return uuid.Max
	}

	return m.SeenUpTo
}

// Admission is what adding a person to a chat comes to.
type Admission int

const (
	// Admitted: the person joins.
	Admitted Admission = iota
	// AlreadyIn: the person is in the chat, nothing changes.
	AlreadyIn
)

// Admit tells what adding the person who stands as m does: someone new or
// removed before joins, who is in stays, and who left on their own is not
// brought back by others (ErrMemberLeft). known is false for someone
// never in the chat.
func Admit(m Member, known bool) (Admission, error) {
	switch {
	case !known, m.State == MemberKicked:
		return Admitted, nil
	case m.State == MemberLeft:
		return 0, ErrMemberLeft
	default:
		return AlreadyIn, nil
	}
}

// Action is what a message of a chat tells of instead of saying
// something: openvk shows such messages between the others, "Анна
// пригласила Петра".
type Action string

const (
	// ActionNone is a message a person wrote.
	ActionNone Action = ""
	// ActionCreate: the sender created the chat; the text is its title.
	ActionCreate Action = "create"
	// ActionTitle: the sender renamed the chat; the text is the new title.
	ActionTitle Action = "title"
	// ActionInvite: the sender added the target.
	ActionInvite Action = "invite"
	// ActionKick: the sender removed the target.
	ActionKick Action = "kick"
	// ActionLeave: the sender left.
	ActionLeave Action = "leave"
	// ActionReturn: the sender came back.
	ActionReturn Action = "return"
	// ActionPin: the sender pinned a message, ActionUnpin took it down.
	ActionPin   Action = "pin"
	ActionUnpin Action = "unpin"
	// ActionPhoto: the sender set the photo of the chat,
	// ActionPhotoRemove removed it.
	ActionPhoto       Action = "photo"
	ActionPhotoRemove Action = "photo_remove"
)

// ChatMessage is a message of a chat: what a member wrote there — text,
// photos, a shared post or photo of an album, messages carried on, as a
// message of a dialog (Content) — or what happened there (Action). As
// Message, it does not change once made.
type ChatMessage struct {
	id       uuid.UUID
	chatID   uuid.UUID
	senderID uuid.UUID
	content  Content
	action   Action
	// targetID is whom the action is about, uuid.Nil for none.
	targetID  uuid.UUID
	createdAt time.Time
	editedAt  time.Time
	deletedAt time.Time
}

// NewChatMessage is what senderID writes in a chat.
func NewChatMessage(chatID, senderID uuid.UUID, content Content, now time.Time) (*ChatMessage, error) {
	return newChatMessage(chatID, senderID, content, ActionNone, uuid.Nil, now)
}

// NewChatAction tells the chat what senderID did: to targetID, uuid.Nil
// for nobody, or to the title, which a renamed or created chat says.
func NewChatAction(chatID, senderID uuid.UUID, action Action, targetID uuid.UUID, title Title, now time.Time) (*ChatMessage, error) {
	return newChatMessage(chatID, senderID, Content{text: Text(title)}, action, targetID, now)
}

func newChatMessage(chatID, senderID uuid.UUID, content Content, action Action, targetID uuid.UUID, now time.Time) (*ChatMessage, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &ChatMessage{
		id:        id,
		chatID:    chatID,
		senderID:  senderID,
		content:   content,
		action:    action,
		targetID:  targetID,
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredChatMessage is a message of a chat as storage keeps it.
type StoredChatMessage struct {
	ID       uuid.UUID
	ChatID   uuid.UUID
	SenderID uuid.UUID
	// Content of an action is the title it tells of, as text.
	Content   Content
	Action    Action
	TargetID  uuid.UUID
	CreatedAt time.Time
	// EditedAt and DeletedAt are zero for never.
	EditedAt  time.Time
	DeletedAt time.Time
}

// RestoreChatMessage rebuilds a message from storage without validation.
func RestoreChatMessage(s StoredChatMessage) *ChatMessage {
	return &ChatMessage{
		id:        s.ID,
		chatID:    s.ChatID,
		senderID:  s.SenderID,
		content:   s.Content,
		action:    s.Action,
		targetID:  s.TargetID,
		createdAt: s.CreatedAt,
		editedAt:  s.EditedAt,
		deletedAt: s.DeletedAt,
	}
}

func (m *ChatMessage) ID() uuid.UUID {
	return m.id
}

func (m *ChatMessage) ChatID() uuid.UUID {
	return m.chatID
}

func (m *ChatMessage) SenderID() uuid.UUID {
	return m.senderID
}

// Text is what the message says; of an action, the title it tells of.
func (m *ChatMessage) Text() Text {
	return m.content.text
}

func (m *ChatMessage) Photos() Photos {
	return m.content.photos
}

// Post is the post shared in the message, uuid.Nil for none.
func (m *ChatMessage) Post() uuid.UUID {
	return m.content.post
}

// SharedPhoto is the photo of an album shared in the message, uuid.Nil
// for none.
func (m *ChatMessage) SharedPhoto() uuid.UUID {
	return m.content.sharedPhoto
}

// Sticker is the sticker m is, "" for none.
func (m *ChatMessage) Sticker() Sticker {
	return m.content.sticker
}

// Documents are the documents m attaches, nil for none.
func (m *ChatMessage) Documents() Documents {
	return m.content.documents
}

// Forwarded are the messages m carries on, nil for none.
func (m *ChatMessage) Forwarded() Forwards {
	return m.content.forwarded
}

// Carried is m as another message carries it on, as Message.Carried: what
// happened in the chat and a message taken back carry nothing.
func (m *ChatMessage) Carried() []Forwarded {
	if m.IsDeleted() || m.IsAction() {
		return nil
	}

	return m.content.carried(m.senderID, m.createdAt)
}

// Action is ActionNone for a message a person wrote.
func (m *ChatMessage) Action() Action {
	return m.action
}

// TargetID is whom the action is about, uuid.Nil for none.
func (m *ChatMessage) TargetID() uuid.UUID {
	return m.targetID
}

func (m *ChatMessage) CreatedAt() time.Time {
	return m.createdAt
}

// EditedAt is zero for a message never edited.
func (m *ChatMessage) EditedAt() time.Time {
	return m.editedAt
}

// DeletedAt is zero for a message never taken back.
func (m *ChatMessage) DeletedAt() time.Time {
	return m.deletedAt
}

// IsDeleted tells whether the sender took the message back.
func (m *ChatMessage) IsDeleted() bool {
	return !m.deletedAt.IsZero()
}

// IsAction tells whether the message tells of what happened in the chat.
func (m *ChatMessage) IsAction() bool {
	return m.action != ActionNone
}

// EditableUntil and DeletableUntil are when the sender can no longer edit
// the message and take it back, as in a dialog.
func (m *ChatMessage) EditableUntil() time.Time {
	return m.createdAt.Add(editWindow)
}

func (m *ChatMessage) DeletableUntil() time.Time {
	return m.createdAt.Add(deleteWindow)
}

// Edited returns m as rev makes it, edited by requesterID at now: only its
// sender, for editWindow after sending, and never what happened in the
// chat. What it shares and carries on stays, as in a dialog
// (Message.Edited).
func (m *ChatMessage) Edited(requesterID uuid.UUID, rev Revision, now time.Time) (*ChatMessage, error) {
	if requesterID != m.senderID || m.IsAction() {
		return nil, ErrCannotEdit
	}

	if rev.Sealed != "" {
		return nil, ErrSealedInChat
	}

	if m.IsDeleted() {
		return nil, ErrNotFound
	}

	if now.After(m.EditableUntil()) {
		return nil, ErrEditExpired
	}

	if rev.outdated(m.content.text) {
		return nil, ErrEditConflict
	}

	content, err := m.content.revised(rev)
	if err != nil {
		return nil, err
	}

	edited := *m
	edited.content = content
	edited.editedAt = now.UTC().Truncate(time.Microsecond)

	return &edited, nil
}

// TakenBack returns m taken back by requesterID at now: only its sender,
// for deleteWindow after sending, and never what happened in the chat. It
// stays where it was, saying nothing: "Сообщение удалено".
func (m *ChatMessage) TakenBack(requesterID uuid.UUID, now time.Time) (*ChatMessage, error) {
	if requesterID != m.senderID || m.IsAction() {
		return nil, ErrCannotDelete
	}

	if now.After(m.DeletableUntil()) {
		return nil, ErrDeleteExpired
	}

	if m.IsDeleted() {
		return nil, ErrNotFound
	}

	gone := *m
	gone.content = Content{}
	gone.editedAt = time.Time{}
	gone.deletedAt = now.UTC().Truncate(time.Microsecond)

	return &gone, nil
}

// ChatView is a chat as one of its people has it: how they stand in it,
// how many are in it now and the last message they are shown, nil for
// none.
type ChatView struct {
	Chat        *Chat
	Me          Member
	Members     int
	LastMessage *ChatMessage
}

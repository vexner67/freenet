package model

import "time"

// How the caller stands in a chat.
const (
	ChatStateActive = "active"
	ChatStateLeft   = "left"
	ChatStateKicked = "kicked"
)

// Chat is a chat of several people ("беседа") as the caller has it.
type Chat struct {
	ID string
	// Title may be empty: the chat is then called by who is in it.
	Title       string
	OwnerID     string
	CreatedAt   time.Time
	MemberCount int
	// State is one of the ChatState values.
	State string
	// LastMessage is the last one the caller is shown, nil for none.
	LastMessage *ChatMessage
	Unread      bool
}

// ChatView is a chat with who is in it, who joined first first; nobody
// for a caller who is not in it anymore.
type ChatView struct {
	Chat    Chat
	Members []ChatMember
}

type ChatMember struct {
	UserID   string
	JoinedAt time.Time
}

// ChatMessage is what SenderID said in a chat or, with Action, what they
// did there: "create" and "title" (Text is the title), "invite" and "kick"
// (of TargetID), "leave", "return".
type ChatMessage struct {
	ID       string
	ChatID   string
	SenderID string
	Text     string
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut   bool
	PhotoIDs  []string
	Action    string
	TargetID  string
	CreatedAt time.Time
	EditedAt  time.Time
	DeletedAt time.Time
	// The sender may take it back and edit it until then; zero for an
	// action and for a message taken back.
	DeletableUntil time.Time
	EditableUntil  time.Time
	// Unread: someone else's, which the caller has not read.
	Unread bool
	// What a message of a dialog may show besides its words: a shared
	// post, a shared photo of an album ("" for none) and the messages it
	// carries on.
	PostID        string
	SharedPhotoID string
	Forwarded     []ForwardedMessage
}

// FoundChatMessages is one screen of messages a search of chats found,
// newest first.
type FoundChatMessages struct {
	Messages   []ChatMessage
	NextCursor string
}

// ChatPage is one screen of chats, the most recent first.
type ChatPage struct {
	Chats      []Chat
	NextCursor string
}

type ChatMessagePage struct {
	Messages   []ChatMessage
	NextCursor string
}

// Invitee is someone the caller names to add to a chat; Allowed: their
// privacy and blacklists let the caller.
type Invitee struct {
	Nickname string
	UserID   string
	Allowed  bool
}

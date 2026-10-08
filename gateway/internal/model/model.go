// Package model is what the public API talks about, independent of both
// JSON and the internal gRPC contracts.
package model

import "time"

type Tokens struct {
	AccessToken  string
	RefreshToken string
}

// Profile fields hold the values of the user service: enums are lowercase
// names ("male", "in_love"), "" means not set.
type Profile struct {
	Name           string
	Surname        string
	Nickname       string
	Status         string
	Gender         string
	Relationship   string
	Worldview      string
	PoliticalViews string
	// Birthday is a date at midnight UTC; zero means not set.
	Birthday time.Time
}

// Privacy: WallWrite is who may write on the wall besides its owner,
// "everyone", "following" (the people the owner follows) or "only_me";
// MessagesWrite who may write private messages, "everyone", "following"
// or "nobody".
type Privacy struct {
	ShowBirthday       bool
	ShowRelationship   bool
	ShowWorldview      bool
	ShowPoliticalViews bool
	WallWrite          string
	MessagesWrite      string
}

// Relation is who of the viewer and a user blacklisted the other.
type Relation struct {
	BlockedByMe bool
	BlockedMe   bool
}

// Blocked: either blacklisted the other, which shuts messages, wall posts,
// comments and follows between them.
func (r Relation) Blocked() bool {
	return r.BlockedByMe || r.BlockedMe
}

// User is the caller's own account. AvatarID is empty without a photo.
type User struct {
	ID               string
	AvatarID         string
	Email            string
	Profile          Profile
	ProfileCompleted bool
	Privacy          Privacy
	// Deactivated: the owner deleted the page, which can be restored until
	// PurgeAt.
	Deactivated bool
	PurgeAt     time.Time
}

// PublicProfile is what anyone may see; fields hidden by the owner are empty.
type PublicProfile struct {
	ID       string
	AvatarID string
	Profile  Profile
	// Deactivated: the page is deleted, only names and the photo are left.
	Deactivated bool
}

// Author is who wrote a post or owns a wall, as shown next to a post.
type Author struct {
	ID       string
	AvatarID string
	Nickname string
	Name     string
	Surname  string
	Gender   string
	// Deactivated: the page is deleted; what its owner wrote is hidden.
	Deactivated bool
}

type Post struct {
	ID     string
	Author Author
	// Wall is the owner of the wall the post is on; Wall.ID is Author.ID
	// on the author's own wall.
	Wall Author
	Text string
	// PhotoIDs are the author's photos in the media service, in order.
	PhotoIDs  []string
	CreatedAt time.Time
	Likes     int
	Comments  int
	Reposts   int
	// Liked: the viewer likes the post; false for anonymous viewers.
	Liked bool
	// RepostOf is the shared post of a repost, "" for other posts; the
	// shared post may be gone.
	RepostOf string
	// EditedAt is zero for a post never edited; the author may edit it
	// until EditableUntil.
	EditedAt      time.Time
	EditableUntil time.Time
	// Pinned: the owner of the wall shows it first there.
	Pinned bool
}

// PostPage is one screen of a wall; NextCursor is empty on the last one.
type PostPage struct {
	Posts      []Post
	NextCursor string
	Total      int
}

type Comment struct {
	ID     string
	PostID string
	Author Author
	Text   string
	// PhotoIDs are the author's photos in the media service, in order.
	PhotoIDs  []string
	CreatedAt time.Time
	// ReplyTo is the comment this one answers, "" for none.
	ReplyTo string
	Likes   int
	// Liked: the viewer likes the comment; false for anonymous viewers.
	Liked bool
}

// NewComment is what a comment to write says: text, photos or both, and
// the comment it answers, "" for none.
type NewComment struct {
	Text     string
	PhotoIDs []string
	ReplyTo  string
}

// CommentPage is one screen of the comments of a post, oldest first;
// NextCursor is empty on the last one.
type CommentPage struct {
	Comments   []Comment
	NextCursor string
	Total      int
}

// Notification kinds are "like", "comment" and "follow". PostID is set
// for likes and comments, CommentID for comments.
type Notification struct {
	ID        string
	Kind      string
	ActorID   string
	PostID    string
	CommentID string
	CreatedAt time.Time
}

// NotificationPage is one screen of notifications, newest first;
// NextCursor is empty on the last one.
type NotificationPage struct {
	Notifications []Notification
	NextCursor    string
}

// Message is one private message; Read means its recipient has read it.
type Message struct {
	ID          string
	SenderID    string
	RecipientID string
	Text        string
	// PhotoIDs are the sender's photos in the media service, in order.
	PhotoIDs  []string
	CreatedAt time.Time
	Read      bool
}

// Dialog is the caller's conversation with PeerID; Unread means the peer
// wrote something the caller has not read.
type Dialog struct {
	PeerID      string
	LastMessage Message
	Unread      bool
}

// DialogPage is one screen of dialogs, the most recent first.
type DialogPage struct {
	Dialogs    []Dialog
	NextCursor string
}

// MessageEvent is what happened in the caller's dialogs: a new message,
// sent or received, or a read mark that moved. One of the two is set.
type MessageEvent struct {
	Message *Message
	Read    *ReadMark
}

// ReadMark: ReaderID has read the dialog with PeerID up to UpTo.
type ReadMark struct {
	ReaderID string
	PeerID   string
	UpTo     string
}

// MessageEvents yields the caller's message events as they happen, until
// it returns an error: the stream ended.
type MessageEvents func() (MessageEvent, error)

// NotificationChanges yields what happens to the caller's notifications,
// "new", "gone" or "read", until it returns an error.
type NotificationChanges func() (string, error)

// MessagePage is a run of messages of one dialog, oldest first.
// NextCursor pages to older ones; PeerReadUpTo is the newest of the
// caller's messages the peer has read ("" if none).
type MessagePage struct {
	Messages     []Message
	NextCursor   string
	PeerReadUpTo string
}

type FollowStats struct {
	Followers int
	Following int
	// IsFollowing: the viewer follows the user; false for anonymous viewers.
	IsFollowing bool
}

// UserPage is one screen of a follow list; NextCursor is empty on the last.
type UserPage struct {
	Users      []PublicProfile
	NextCursor string
	Total      int
}

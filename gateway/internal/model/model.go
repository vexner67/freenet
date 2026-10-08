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
// or "nobody"; ClubsRead who sees the user's communities, "everyone",
// "following" or "only_me".
type Privacy struct {
	ShowBirthday       bool
	ShowRelationship   bool
	ShowWorldview      bool
	ShowPoliticalViews bool
	WallWrite          string
	MessagesWrite      string
	ClubsRead          string
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

// Author is who wrote a post or owns a wall, as shown next to a post: a
// user, or a community (Club), which has only a name and maybe an
// address.
type Author struct {
	ID       string
	AvatarID string
	Nickname string
	Name     string
	Surname  string
	Gender   string
	// Deactivated: the page is deleted; what its owner wrote is hidden.
	Deactivated bool
	Club        bool
	Address     string
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
	// FromWall: the post speaks for its wall, a community; Author is
	// still its writer, whom the API shows only when Signed.
	FromWall bool
	Signed   bool
	// Mine: the viewer wrote it; false for anonymous viewers.
	Mine bool
}

// NewPost is what a post to write says and where: on the wall of WallID
// ("" for the caller's own), a community's if ClubWall, speaking for it
// if FromWall.
type NewPost struct {
	WallID   string
	Text     string
	PhotoIDs []string
	ClubWall bool
	FromWall bool
	Signed   bool
}

// FeedSources are what a news feed shows: the posts of Authors and every
// post on Walls, the clubs of the reader.
type FeedSources struct {
	Authors []string
	Walls   []string
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
	// DeletableUntil: the sender may delete it from both sides until then.
	DeletableUntil time.Time
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
// sent or received, a read mark that moved or a message deleted. One of
// the three is set.
type MessageEvent struct {
	Message *Message
	Read    *ReadMark
	Deleted *DeletedMessage
}

// DeletedMessage: SenderID took message ID back from the dialog with
// RecipientID.
type DeletedMessage struct {
	ID          string
	SenderID    string
	RecipientID string
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

// Club is a community. Wall is who may write on its wall: "open" for
// everyone, "closed" for its admins; AdminsDisplay what its page shows of
// those who run it: "creator", "all" or "none".
type Club struct {
	ID          string
	Name        string
	Description string
	// Address is the short name of its page, "" for none.
	Address string
	// Website is an http or https link, "" for none.
	Website       string
	Wall          string
	AdminsDisplay string
	Members       int
	CreatedAt     time.Time
	// AvatarID is the club's photo, filed under its ID; "" for none.
	AvatarID string
}

// ClubView is a club as the caller sees it. Role is what the caller is in
// it: "" (not a member, or anonymous), "member", "admin" or "owner";
// OwnerID is "" when its owner's page was erased.
type ClubView struct {
	Club    Club
	Role    string
	OwnerID string
}

// ClubChanges are a club's settings form: everything at once.
type ClubChanges struct {
	Name          string
	Description   string
	Address       string
	Website       string
	Wall          string
	AdminsDisplay string
}

// ClubMember is one member of a club, with their role.
type ClubMember struct {
	UserID string
	Role   string
}

// ClubMemberPage is one screen of members, the newest first; Total counts
// them, or the admins, on all pages.
type ClubMemberPage struct {
	Members    []ClubMember
	NextCursor string
	Total      int
}

// Membership is one club of the caller and what they are in it; Pinned:
// in their left menu.
type Membership struct {
	Club   Club
	Role   string
	Pinned bool
}

// ClubPage is one screen of the caller's clubs, the newest joined first;
// Total counts all of them.
type ClubPage struct {
	Clubs      []Membership
	NextCursor string
	Total      int
}

// ClubSearchPage is one screen of found clubs, the newest first.
type ClubSearchPage struct {
	Clubs      []Club
	NextCursor string
}

// UserClubPage is one screen of the clubs of a user, the newest joined
// first; Total counts all of them.
type UserClubPage struct {
	Clubs      []Club
	NextCursor string
	Total      int
}

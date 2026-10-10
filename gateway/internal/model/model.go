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
	// HideBirthYear: the owner shows only the day and the month.
	HideBirthYear bool
	Hometown      string
	// PartnerID is whom the relationship names, "" for nobody (or hidden).
	PartnerID string
}

// About is the rest of openvk's page: "Контактная информация", "Личная
// информация" and "Дополнительно", public, and the date of registration.
type About struct {
	Contacts     Contacts
	Interests    Interests
	ExtraFields  []ExtraField
	RegisteredAt time.Time
}

type Contacts struct {
	Email    string
	Telegram string
	Website  string
	City     string
	Address  string
}

type Interests struct {
	Interests string
	Music     string
	Films     string
	Shows     string
	Books     string
	Quotes    string
	Games     string
	About     string
}

// ExtraField is a field of the page its owner names; Place is "contacts"
// or "interests", where it shows.
type ExtraField struct {
	Name  string
	Text  string
	Place string
}

// UserQuery is a search for people: words and the filters openvk's page
// links to, all optional but not all empty.
type UserQuery struct {
	Text     string
	Hometown string
	City     string
	Film     string
	Show     string
	Book     string
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
	// PageRead ("everyone" or "users"), InfoRead, PhotosRead and
	// FriendsRead are openvk's "Кому видно …": "everyone", "users",
	// "following" or "only_me"; OnlineRead is "Кто видит мою
	// неактивность", the same.
	PageRead    string
	InfoRead    string
	PhotosRead  string
	FriendsRead string
	OnlineRead  string
}

// Access is what of a page the viewer may see: the page, its information,
// albums, follows and communities.
type Access struct {
	Page    bool
	Info    bool
	Photos  bool
	Friends bool
	Clubs   bool
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
	About       About
	Backdrop    Backdrop
	// Completeness is "Заполненность страницы" in percent, Unfilled the
	// parts missing ("interests", "email", "telegram", "status", "city").
	Completeness int
	Unfilled     []string
	Interface    Interface
}

// Interface is how the site looks to its owner ("Интерфейс" of the
// settings): AvatarStyle is "square" or "round", MainPage "page" or
// "feed", HiddenMenu the links taken out of the left menu.
type Interface struct {
	AvatarStyle string
	MainPage    string
	HiddenMenu  []string
}

// Backdrop is "Фон страницы" of a person: photos of the media service
// under them, "" for a side without one.
type Backdrop struct {
	Left  string
	Right string
}

// PublicProfile is what anyone may see; fields hidden by the owner are empty.
type PublicProfile struct {
	ID       string
	AvatarID string
	Profile  Profile
	// Deactivated: the page is deleted, only names and the photo are left.
	Deactivated bool
	// BirthYearHidden: Profile.Birthday has no real year.
	BirthYearHidden bool
	// PageHidden: the viewer may not see the page, only the names and the
	// photo; InfoHidden: nor its information, left empty.
	PageHidden bool
	InfoHidden bool
	// LastSeen is when the user last visited ("был в сети"), zero if never
	// or hidden; Online: just now ("Онлайн").
	LastSeen time.Time
	Online   bool
	Backdrop Backdrop
	// Completeness is "Заполненность страницы" in percent; 0 on a deleted
	// or hidden page.
	Completeness int
	About        About
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
	// Online: the person is on the site now ("Онлайн"), if their page
	// tells everyone.
	Online bool
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
	// Archived: the owner of the wall put it away, only they see it.
	// CommentsClosed: nobody comments it anymore.
	Archived       bool
	CommentsClosed bool
	// FromWall: the post speaks for its wall, a community; Author is
	// still its writer, whom the API shows only when Signed.
	FromWall bool
	Signed   bool
	// AvatarUpdate: the post shows the new photo of its wall's owner, a
	// user or a community; nobody edits it.
	AvatarUpdate bool
	// Mine: the viewer wrote it; false for anonymous viewers.
	Mine bool
	// SharedPhotoID is a photo of an album the post shares, "" for none.
	SharedPhotoID string
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
	// AvatarUpdate: the post of the new photo of the wall's owner, the
	// photo alone.
	AvatarUpdate bool
}

// Suggestion is a post suggested for a community's wall ("Предложка"),
// seen by its author and the community's admins until published.
type Suggestion struct {
	ID        string
	WallID    string
	AuthorID  string
	Text      string
	PhotoIDs  []string
	CreatedAt time.Time
}

// SuggestionPage is one screen of suggestions, newest first; Total counts
// them on all pages.
type SuggestionPage struct {
	Suggestions []Suggestion
	NextCursor  string
	Total       int
}

// WallAccess is what a user may do on a club's wall: Post, or else
// Suggest a post for its admins to publish.
type WallAccess struct {
	Post    bool
	Suggest bool
}

// FeedSources are what a news feed shows: the posts of Authors and every
// post on Walls, the clubs of the reader.
type FeedSources struct {
	Authors []string
	Walls   []string
}

// WallPick is which posts of a wall to read: Filter is a tab of openvk's
// wall ("", "owner", "others" or "archive"), Query what the posts say.
type WallPick struct {
	Filter string
	Query  string
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
	// FromWall: the comment speaks for the community whose wall its post
	// is on, shown instead of the author; Mine tells the author it is
	// theirs.
	FromWall bool
	Mine     bool
}

// NewComment is what a comment to write says: text, photos or both, and
// the comment it answers, "" for none; FromWall: for the community whose
// wall the post is on.
type NewComment struct {
	Text     string
	PhotoIDs []string
	ReplyTo  string
	FromWall bool
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
	// PostID is a post shared into the dialog, "" for none.
	PostID string
	// EditedAt is zero for a message never edited; the sender may edit it
	// until EditableUntil.
	EditedAt      time.Time
	EditableUntil time.Time
}

// FoundMessages is one screen of messages a search found, newest first.
type FoundMessages struct {
	Messages   []Message
	NextCursor string
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
// sent or received, a read mark that moved, a message deleted or one
// edited. One of the four is set.
type MessageEvent struct {
	Message *Message
	Read    *ReadMark
	Deleted *DeletedMessage
	Edited  *Message
}

// DeletedMessage: message ID of SenderID to RecipientID is gone from the
// caller's dialog, taken back from both sides or deleted for themselves.
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
	// OpenTopics: anyone starts topics in its discussions, not only its
	// admins; TopicsAboveWall: its page lists them above the wall.
	OpenTopics      bool
	TopicsAboveWall bool
	// BackdropLeft and BackdropRight are "Фон страницы": photos of the
	// club, filed under its ID; "" for none.
	BackdropLeft  string
	BackdropRight string
	// Kind is "group" or "event" ("встреча"): an event has the time it
	// StartsAt and, unless zero, EndsAt, and Maybes of its Members only
	// might come.
	Kind     string
	StartsAt time.Time
	EndsAt   time.Time
	Maybes   int
}

// ClubKinds are what a club may be.
const (
	ClubKindGroup = "group"
	ClubKindEvent = "event"
)

// ClubView is a club as the caller sees it. Role is what the caller is in
// it: "" (not a member, or anonymous), "member", "admin" or "owner";
// OwnerID is "" when its owner's page was erased or the club hides its
// owner from the caller; OwnerTitle is what the owner does in the club.
// CanStartTopic: the caller may start a topic in its discussions. Maybe:
// the caller only might come to the event.
type ClubView struct {
	Club          Club
	Role          string
	Maybe         bool
	OwnerID       string
	OwnerTitle    string
	CanStartTopic bool
}

// ClubChanges are a club's settings form: everything at once.
type ClubChanges struct {
	Name            string
	Description     string
	Address         string
	Website         string
	Wall            string
	AdminsDisplay   string
	OpenTopics      bool
	TopicsAboveWall bool
	// StartsAt and EndsAt are the schedule of an event; zero for none.
	StartsAt time.Time
	EndsAt   time.Time
}

// ClubMember is one member of a club, with their role. Title is what an
// admin does in the club; Hidden: the club shows them as a plain member
// to everyone who does not run it, and only those who do are told.
type ClubMember struct {
	UserID string
	Role   string
	Title  string
	Hidden bool
	// Maybe: they only might come to the event.
	Maybe bool
}

// NewEvent is an event to create; EndsAt is zero for one with no end.
type NewEvent struct {
	Name        string
	Description string
	StartsAt    time.Time
	EndsAt      time.Time
}

// ClubMemberFilter picks the members of a club: only its owner and
// admins, or those who come to the event for sure ("sure") or only might
// ("maybe").
type ClubMemberFilter struct {
	AdminsOnly bool
	Going      string
}

// ClubFilter picks the clubs of a user: only those they run, only those
// of Kind unless it is "".
type ClubFilter struct {
	Managed bool
	Kind    string
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

// Topic is a discussion of a community ("Обсуждения"). AuthorID started
// it; FromWall shows the community instead. Last is the newest message,
// set in lists only and nil without one; Mine: the viewer started it.
type Topic struct {
	ID         string
	ClubID     string
	AuthorID   string
	Title      string
	FromWall   bool
	Closed     bool
	Pinned     bool
	Restricted bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Messages   int
	Last       *TopicMessage
	Mine       bool
}

// TopicMessage is what someone wrote in a topic: text, photos or both,
// for the community if FromWall; Mine: the viewer wrote it.
type TopicMessage struct {
	ID        string
	TopicID   string
	AuthorID  string
	Text      string
	PhotoIDs  []string
	FromWall  bool
	CreatedAt time.Time
	Mine      bool
	// Likes are the hearts of the message; Liked: the caller put one.
	Likes int
	Liked bool
}

// TopicPage and TopicMessagePage are one screen of a list; NextCursor is
// empty on the last, Total counts them on all pages.
type TopicPage struct {
	Topics     []Topic
	NextCursor string
	Total      int
}

type TopicMessagePage struct {
	Messages   []TopicMessage
	NextCursor string
	Total      int
}

// NewTopic is a topic to start in ClubID with a first message, if Text
// or PhotoIDs say anything.
type NewTopic struct {
	ClubID   string
	Title    string
	Text     string
	PhotoIDs []string
	FromWall bool
}

type NewTopicMessage struct {
	Text     string
	PhotoIDs []string
	FromWall bool
}

// TopicChanges are the title and the settings, all at once.
type TopicChanges struct {
	Title      string
	Closed     bool
	Pinned     bool
	Restricted bool
}

// Album is a photo album of a person or a community ("Альбомы"); OwnerID
// is either. Cover is its first photo, nil while it has none.
type Album struct {
	ID          string
	OwnerID     string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Photos      int
	Cover       *AlbumPhoto
	// Kind is "" for an album people make, "wall" for "Фотографии со
	// стены" and "avatars" for "Фотографии со
	// страницы", which the site keeps.
	Kind string
}

// AlbumPhoto is a photo of an album: media keeps the image under its ID
// and AuthorID, who uploaded it; Mine: the viewer did.
type AlbumPhoto struct {
	ID          string
	AlbumID     string
	AuthorID    string
	Description string
	CreatedAt   time.Time
	Mine        bool
	// Likes and Comments count them; Liked: the viewer likes it.
	Likes    int
	Comments int
	Liked    bool
}

// PhotoComment is a comment under a photo of an album, text only; Mine:
// the viewer wrote it.
type PhotoComment struct {
	ID        string
	PhotoID   string
	AuthorID  string
	Text      string
	CreatedAt time.Time
	Mine      bool
}

// PhotoCommentPage is one screen of the comments of a photo, oldest first.
type PhotoCommentPage struct {
	Comments   []PhotoComment
	NextCursor string
	Total      int
}

// AlbumPage and AlbumPhotoPage are one screen of a list; NextCursor is
// empty on the last, Total counts them on all pages.
type AlbumPage struct {
	Albums     []Album
	NextCursor string
	Total      int
}

type AlbumPhotoPage struct {
	Photos     []AlbumPhoto
	NextCursor string
	Total      int
}

// NewAlbum is an album to create: the caller's own, or of ClubID.
type NewAlbum struct {
	ClubID      string
	Name        string
	Description string
}

// NewAlbumPhoto is a photo the caller uploaded, to add to an album.
type NewAlbumPhoto struct {
	PhotoID     string
	Description string
}

// LikerPage is one screen of who like a post or a comment, newest first
// ("Понравилось N людям"); Total is its like counter.
type LikerPage struct {
	UserIDs    []string
	NextCursor string
	Total      int
}

// NewRepost is where and how to share a post: on the caller's wall, or on
// a community's (WallID) for it (FromWall), maybe signed.
type NewRepost struct {
	// PhotoID shares a photo of an album instead of a post.
	PhotoID  string
	Text     string
	WallID   string
	FromWall bool
	Signed   bool
}

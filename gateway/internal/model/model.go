// Package model is what the public API talks about, independent of both
// JSON and the internal gRPC contracts.
package model

import "time"

type Tokens struct {
	AccessToken  string
	RefreshToken string
}

// Client is where a request comes from, as far as the gateway can tell:
// the session it signs in or refreshes remembers it for its owner.
type Client struct {
	IP string
	// UserAgent is what the browser calls itself.
	UserAgent string
}

// Session is one place the user is signed in at.
type Session struct {
	ID        string
	CreatedAt time.Time
	// LastUsedAt is when its tokens were last issued.
	LastUsedAt time.Time
	// IP and UserAgent are where from, "" when unknown.
	IP        string
	UserAgent string
	// Current: the session of the access token that asks.
	Current bool
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
	// Verified keeps the pages with the checkmark only, Banned the pages
	// blocked now.
	Verified bool
	Banned   bool
}

// Ban is a block of a page by the administrators of the site ("Заблокировать
// пользователя"): why, since when and until when, the zero Until for good.
type Ban struct {
	Reason string
	At     time.Time
	Until  time.Time
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
	// неактивность" and GiftsRead "Кому видны мои подарки", the same.
	PageRead    string
	InfoRead    string
	PhotosRead  string
	FriendsRead string
	OnlineRead  string
	GiftsRead   string
	// ChatsInvite is "Кто может добавлять меня в чаты": "everyone",
	// "following" or "nobody"; "" leaves the default.
	ChatsInvite string
	// Closed is openvk's "Тип профиля: закрытый": only the owner and the
	// people the owner follows see the page.
	Closed bool
	// MessagesPrime: only people with Freenet Prime write to the user;
	// MessagePrice is what a message to them costs, 0 for free. Neither
	// holds for those the user wrote to first.
	MessagesPrime bool
	MessagePrice  int64
}

// MessageTerms are on what terms the caller writes to someone: Allowed or
// not, refused only for having no Freenet Prime, and for how many votes a
// message.
type MessageTerms struct {
	Allowed   bool
	PrimeOnly bool
	Price     int64
}

// MessagePayment is a message paid for; ID is "" when it is free.
type MessagePayment struct {
	ID      string
	Balance int64
}

// People is what the site counts of its people: the pages that are not
// deleted, those of them on the site now and those with the checkmark.
type People struct {
	Users    int
	Online   int
	Verified int
}

// Access is what of a page the viewer may see: the page, its information,
// albums, follows, communities and gifts.
type Access struct {
	Page    bool
	Info    bool
	Photos  bool
	Friends bool
	Clubs   bool
	Gifts   bool
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
	// Votes is "Баланс": what the user has to pay for gifts with.
	Votes int64
	// Admin: the user makes the vouchers.
	Admin bool
	// Verified: the administrators confirmed the page (a checkmark).
	Verified bool
	// Ban: the administrators blocked the page, nil if they did not.
	Ban *Ban
	// Alert is the label the administrators put on the page (an orange
	// one), "" for none.
	Alert string
	// PrimeUntil is when Freenet Prime of the user ends, zero without it;
	// PrimeInvisible: they hide when they are online.
	PrimeUntil     time.Time
	PrimeInvisible bool
}

// HasPrime tells whether u has Freenet Prime at now.
func (u User) HasPrime(now time.Time) bool {
	return now.Before(u.PrimeUntil)
}

// Interface is how the site looks to its owner ("Интерфейс" of the
// settings): AvatarStyle is "square" or "round", MainPage "page" or
// "feed", HiddenMenu the links taken out of the left menu, Theme
// "default", "blue" or "dark", NSFW "blur", "hide" or "show", PostView
// "old" or "microblog"; HideRating takes "Заполненность страницы" away,
// and Timezone is minutes east of UTC, nil for the browser's clock.
type Interface struct {
	AvatarStyle string
	MainPage    string
	HiddenMenu  []string
	Theme       string
	HideRating  bool
	NSFW        string
	PostView    string
	Timezone    *int
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
	// Closed: the page is a closed one ("Закрытая страница"), which a
	// hidden page tells too.
	Closed     bool
	InfoHidden bool
	// Verified: the administrators confirmed the page ("Подтверждённая
	// страница"), a checkmark by the name.
	Verified bool
	// Ban: the administrators blocked the page, which then tells only the
	// names, the photo and the ban; nil if they did not.
	Ban *Ban
	// Alert is the label the administrators put on the page: openvk's
	// orange "user-alert" above the name, "" for none.
	Alert string
	// Prime: the page has Freenet Prime (a badge by the name).
	Prime bool
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
	// Verified: the page is confirmed by the administrators (a checkmark).
	Verified bool
	// Prime: the person has Freenet Prime (a badge by the name).
	Prime bool
}

type Post struct {
	ID     string
	Author Author
	// Wall is the owner of the wall the post is on; Wall.ID is Author.ID
	// on the author's own wall.
	Wall Author
	Text string
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut bool
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
	// NSFW: its author marked it as adult.
	NSFW bool
	// FromWall: the post speaks for its wall, a community; Author is
	// still its writer, whom the API shows only when Signed.
	FromWall bool
	Signed   bool
	// Mine: the viewer wrote it; false for anonymous viewers.
	Mine bool
	// SharedPhotoID is a photo of an album the post shares, "" for none.
	SharedPhotoID string
	// Polled: the post asks a poll, which Posts.Polls tells.
	Polled bool
	// DocumentIDs are documents of its author it attaches, in order; a
	// deleted one is gone from it.
	DocumentIDs []string
	// Link is the card of the page the text links to, nil for none.
	Link *Link
	// Price is what the photos and documents cost to see, 0 for free; a
	// viewer who has not paid sees them Locked, only how many there are.
	Price           int64
	Locked          bool
	LockedPhotos    int
	LockedDocuments int
}

// Link is the card of a page a post links to, as the page told it when
// the post was written. PhotoID is its picture, a photo of the post's
// author in media; "" for none.
type Link struct {
	URL         string
	Title       string
	Description string
	Site        string
	PhotoID     string
}

// LinkPreview is what a page says of itself: its card, and the bytes of
// its picture, nil for none.
type LinkPreview struct {
	URL         string
	Title       string
	Description string
	Site        string
	Image       []byte
}

// NewPoll is the poll a new post asks ("Новый опрос"): a title and up to
// ten options. Days is how long it takes votes, 0 for ever.
type NewPoll struct {
	Title     string
	Options   []string
	Anonymous bool
	Multiple  bool
	Locked    bool
	Days      int
}

// PollOption is an answer of a poll with how many chose it.
type PollOption struct {
	Name  string
	Votes int
}

// Poll is the poll of the post PostID as its viewer sees it: Voters is
// how many people voted, Chosen the places of the options the viewer
// chose (none for one who has not voted, and for anonymous viewers).
// Anonymous: who voted is not told; Multiple: several options may be
// chosen; Locked: a vote cannot be taken back; Ended: EndsAt has passed.
type Poll struct {
	PostID    string
	Title     string
	Options   []PollOption
	Voters    int
	Anonymous bool
	Multiple  bool
	Locked    bool
	EndsAt    time.Time
	Ended     bool
	Chosen    []int
}

// Lists carry the beginnings of texts, which say up to 60 000
// characters: ExcerptLength of a post or a comment, more than the site
// shows before "Показать больше", and PreviewLength of the last message of
// a dialog, shown in one line.
const (
	ExcerptLength = 1000
	PreviewLength = 200
)

// NewPost is what a post to write says and where: on the wall of WallID
// ("" for the caller's own), a community's if ClubWall, speaking for it
// if FromWall.
type NewPost struct {
	WallID   string
	Text     string
	PhotoIDs []string
	// DocumentIDs are documents of the caller the post attaches.
	DocumentIDs []string
	ClubWall    bool
	FromWall    bool
	Signed      bool
	// NSFW marks the post as adult ("Содержит NSFW-контент").
	NSFW bool
	// Mentions are the IDs of the people the text names, who hear of it.
	Mentions []string
	// Poll is the poll the post asks, nil for none; with it the text and
	// the photos may be none.
	Poll *NewPoll
	// Link is the card of the page the text links to, nil for none; its
	// picture is published already.
	Link *Link
	// Price is what others pay to see its photos and documents, 0 for free.
	Price int64
}

// PostEdit is what an edit makes of a post: its text and photos, whole,
// and, unless nil, whether it is marked as adult.
type PostEdit struct {
	Text     string
	PhotoIDs []string
	NSFW     *bool
	// Mentions are the IDs of the people the new text names.
	Mentions []string
}

// Suggestion is a post suggested for a community's wall ("Предложка"),
// seen by its author and the community's admins until published.
type Suggestion struct {
	ID       string
	WallID   string
	AuthorID string
	Text     string
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut   bool
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

// Postponed is a post that waits for its time (VK's "отложенная
// запись"), seen by its author and, for a community, the community's
// admins until it comes out.
type Postponed struct {
	ID       string
	WallID   string
	AuthorID string
	Text     string
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut   bool
	PhotoIDs  []string
	FromWall  bool
	Signed    bool
	NSFW      bool
	PublishAt time.Time
	CreatedAt time.Time
}

// NewPostponed is a post to postpone: on the author's own wall (WallID
// "") or, FromWall, for a community.
type NewPostponed struct {
	WallID   string
	Text     string
	PhotoIDs []string
	FromWall bool
	Signed   bool
	NSFW     bool
	// Mentions are the IDs of the people the text names.
	Mentions  []string
	PublishAt time.Time
}

// PostponedEdit is what an edit of a postponed post sets; its photos stay.
type PostponedEdit struct {
	Text      string
	NSFW      bool
	Mentions  []string
	PublishAt time.Time
}

// WallAccess is what a user may do on a club's wall: Post, or else
// Suggest a post for its admins to publish.
type WallAccess struct {
	Post    bool
	Suggest bool
}

// FeedSources are what a news feed shows: what the people the reader
// follows wrote, every post on their walls and on the walls of the clubs
// of the reader. Whom the reader reads the posts service knows itself.
//
// Everyone is "Все новости": the posts of every wall, whomever the reader
// follows.
type FeedSources struct {
	Everyone bool
	// HideNSFW leaves out the posts marked as adult.
	HideNSFW bool
	// Hidden are the people whose posts are left out: the reader's
	// blacklist.
	Hidden []string
	// OwnOnly leaves only what a wall says itself: its owner on their
	// wall, a community in its name.
	OwnOnly bool
	// Ignored are the walls whose posts are left out: the reader's
	// "Игнорируемые источники".
	Ignored []string
}

// WallPick is which posts of a wall to read: Filter is a tab of openvk's
// wall ("", "owner", "others" or "archive"), Query what the posts say,
// Year the year they were written in, 0 for any.
type WallPick struct {
	Filter string
	Query  string
	Year   int
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
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut bool
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
	// Mentions are the IDs of the people the text names.
	Mentions []string
}

// CommentPage is one screen of the comments of a post, oldest first;
// NextCursor is empty on the last one.
type CommentPage struct {
	Comments   []Comment
	NextCursor string
	Total      int
}

// PushSubscription is a browser that takes pushes (Web Push): where its
// push service takes them and the keys, base64url, as the browser gives
// them.
type PushSubscription struct {
	Endpoint string
	P256DH   string
	Auth     string
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
// What it says, and what the messages it carries on said, is Sealed: the
// browsers of its two people seal and open it, the server cannot.
type Message struct {
	ID          string
	SenderID    string
	RecipientID string
	Sealed      []byte
	// PhotoIDs are the sender's photos in the media service, in order.
	PhotoIDs  []string
	CreatedAt time.Time
	Read      bool
	// DeletableUntil: the sender may delete it from both sides until then.
	DeletableUntil time.Time
	// PostID is a post shared into the dialog, "" for none.
	PostID string
	// SharedPhotoID is a photo of an album shared into it, "" for none.
	SharedPhotoID string
	// Sticker is the sticker the message is, "" for none.
	Sticker string
	// EditedAt is zero for a message never edited; the sender may edit it
	// until EditableUntil.
	EditedAt      time.Time
	EditableUntil time.Time
	// Forwarded are the messages it carries on from other dialogs.
	Forwarded []ForwardedMessage
	// DeletedAt is when the sender took it back from both sides, zero if
	// never: it says nothing then.
	DeletedAt time.Time
	// DocumentIDs are documents of the sender it attaches.
	DocumentIDs []string
	// ReplyTo is what it shows of the message it answers, nil for none.
	ReplyTo *Quote
	// Reactions are those to it, the oldest first.
	Reactions []Reaction
}

// Quote is what a message answering another shows of it ("Ответить").
// Only its ID is known of one gone for good (Found is false); Text is a
// chat's, a message of a dialog says its words sealed.
type Quote struct {
	ID       string
	SenderID string
	Text     string
	TextCut  bool
	// PhotoCount, ForwardedCount and DocumentCount are how many it shows.
	PhotoCount     int
	ForwardedCount int
	DocumentCount  int
	Sticker        string
	// Post and SharedPhoto: it shares a post, a photo of an album.
	Post        bool
	SharedPhoto bool
	// Deleted: it was taken back.
	Deleted bool
	Found   bool
}

// Reaction is one person's reaction to a message: heart, like, laugh,
// wow, sad or fire.
type Reaction struct {
	UserID   string
	Reaction string
}

// ForwardedMessage is a message as another one carries it on: a copy of
// what SenderID wrote at CreatedAt.
type ForwardedMessage struct {
	SenderID string
	Text     string
	PhotoIDs []string
	PostID   string
	// SharedPhotoID is a photo of an album it shared, "" for none.
	SharedPhotoID string
	CreatedAt     time.Time
	// Depth is how deep it is carried: 0 for a message forwarded itself,
	// 1 for one that message carried on.
	Depth int
	// Sticker is the sticker it was, "" for none.
	Sticker     string
	DocumentIDs []string
}

// MessageDraft is a message to send, into a dialog or a chat: text,
// photos, a shared post ("" for none), messages of the caller's dialogs
// and chats to forward, or some of them;
// or a photo of an album to share (SharedPhotoID), with text alone; or a
// Sticker, alone. Into a dialog the text goes Sealed, with that of the
// messages it forwards.
type MessageDraft struct {
	Text          string
	Sealed        []byte
	PhotoIDs      []string
	PostID        string
	SharedPhotoID string
	ForwardIDs    []string
	Sticker       string
	// DocumentIDs are documents of the caller the message attaches.
	DocumentIDs []string
	// ReplyToID is the message of the dialog or chat it answers, "" for
	// none.
	ReplyToID string
}

// MessageEdit is what an edit makes of a message: its words and, with
// ReplacePhotos, its photos; without, the photos stay. The words of a
// message of a dialog are Sealed.
type MessageEdit struct {
	Text   string
	Sealed []byte
	// BaseText (BaseSealed), when set, is what the message said when the
	// edit started: a message that says something else by now is not
	// edited.
	BaseText   *string
	BaseSealed []byte
	// The photos the message shows no more, and the uploads it shows
	// from now on.
	RemovePhotoIDs []string
	AddPhotoIDs    []string
}

// MessagesAt names a part of a dialog or of a chat: older messages with Cursor, those
// after After, the messages around Around, or, with none of them, the
// newest.
type MessagesAt struct {
	Cursor string
	After  string
	Around string
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
// sent or received, a read mark that moved, a message deleted by the
// caller for themselves, or one edited or taken back by its sender. One
// of the four is set, or one of those of the caller's chats: a new
// message, one edited or taken back, the caller's own read mark.
type MessageEvent struct {
	Message *Message
	Read    *ReadMark
	Deleted *DeletedMessage
	Edited  *Message

	ChatMessage *ChatMessage
	ChatEdited  *ChatMessage
	ChatRead    *ChatRead
	Typing      *Typing
}

// Typing: UserID is writing to the caller, in chat ChatID or, when it is
// "", in their dialog.
type Typing struct {
	UserID string
	ChatID string
}

// ChatRead: the caller has read chat ChatID up to UpTo.
type ChatRead struct {
	ChatID string
	UpTo   string
}

// DeletedMessage: message ID of SenderID to RecipientID is gone from the
// caller's dialog: they deleted it for themselves.
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
	// Closed: one joins by a request its admins accept, and only the
	// members see what is in the club ("Закрытая группа").
	Closed bool
	// Verified: the administrators of the site confirmed the club (a
	// checkmark).
	Verified bool
}

// ClubView is a club as the caller sees it. Role is what the caller is in
// it: "" (not a member, or anonymous), "member", "admin" or "owner";
// OwnerID is "" when its owner's page was erased or the club hides its
// owner from the caller; OwnerTitle is what the owner does in the club.
// CanStartTopic: the caller may start a topic in its discussions.
type ClubView struct {
	Club          Club
	Role          string
	OwnerID       string
	OwnerTitle    string
	CanStartTopic bool
	// Requested: the caller asked to join the closed club and waits;
	// Requests: how many do, told to those who run it.
	Requested bool
	Requests  int
}

// Admits tells whether the caller sees what is in the club: anyone in an
// open one, only the members of a closed one.
func (v ClubView) Admits() bool {
	return !v.Club.Closed || v.Role != ""
}

// JoinRequestPage is one screen of those who ask to join a club, the
// newest first; Total counts them all.
type JoinRequestPage struct {
	UserIDs    []string
	NextCursor string
	Total      int
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
	// Closed is "Тип группы: закрытая".
	Closed bool
}

// SeatChange is what a change of an admin's seat names: the title, the
// hiding or both; what is nil stays as it is. BaseTitle, when set, is
// the title the change started from: a seat that says another by now
// keeps it.
type SeatChange struct {
	Title     *string
	BaseTitle *string
	Hidden    *bool
}

// ClubMember is one member of a club, with their role. Title is what an
// admin does in the club; Hidden: the club shows them as a plain member
// to everyone who does not run it, and only those who do are told.
type ClubMember struct {
	UserID string
	Role   string
	Title  string
	Hidden bool
}

// ClubMemberFilter picks the members of a club: only its owner and
// admins with AdminsOnly.
type ClubMemberFilter struct {
	AdminsOnly bool
}

// ClubFilter picks the clubs of a user: only those they run with Managed.
type ClubFilter struct {
	Managed bool
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

// KnownNicknames are those of the nicknames asked that are someone's, as
// their owners write them, and those of them whose pages wear the
// checkmark.
type KnownNicknames struct {
	Known    []string
	Verified []string
}

// ClubQuery is a search for clubs: words of the name or the address, or
// none with Verified, which keeps the clubs with the checkmark only.
type ClubQuery struct {
	Text     string
	Verified bool
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
	ID       string
	TopicID  string
	AuthorID string
	Text     string
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut   bool
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
	// Mentions are the IDs of the people the first message names.
	Mentions []string
}

type NewTopicMessage struct {
	Text     string
	PhotoIDs []string
	FromWall bool
	// Mentions are the IDs of the people the text names.
	Mentions []string
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
	// NSFW: it came from a post marked as adult.
	NSFW bool
}

// PhotoComment is a comment under a photo of an album, text only; Mine:
// the viewer wrote it.
type PhotoComment struct {
	ID       string
	PhotoID  string
	AuthorID string
	Text     string
	// TextCut: Text is only its beginning, as lists carry it.
	TextCut bool
	// ReplyTo is the comment of the same photo it answers; "" for none.
	ReplyTo string
	// Likes are its hearts; Liked: the caller put one.
	Likes     int
	Liked     bool
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

// FavePhoto is a photo of an album the viewer likes, with whom its album
// belongs to, a person or a community.
type FavePhoto struct {
	AlbumPhoto
	OwnerID string
}

// FavePhotoPage is one screen of them, newest like first.
type FavePhotoPage struct {
	Photos     []FavePhoto
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
	PhotoID string
	Text    string
	// PhotoIDs are the caller's photos added to the words about a post.
	PhotoIDs []string
	WallID   string
	FromWall bool
	Signed   bool
	// Mentions are the IDs of the people the text names.
	Mentions []string
}

// NewPhotoComment is a comment to put under a photo of an album.
type NewPhotoComment struct {
	Text string
	// ReplyTo is the comment of the same photo it answers, "" for none.
	ReplyTo string
	// Mentions are the IDs of the people the text names.
	Mentions []string
}

// GiftCollection is a shelf of the catalogue of gifts ("Коллекции"): the
// names of its gifts, in the order they are shown, and their price.
type GiftCollection struct {
	ID    string
	Gifts []string
	// Price is what each of them costs the sender, in votes, unless
	// priced otherwise; 0 for free.
	Price int64
	// Prices are what each of Gifts costs now, in their order: the
	// administrators price a gift apart from its collection. Empty from a
	// user service that does not know them.
	Prices []int64
	// Prime: only those with Freenet Prime send its gifts.
	Prime bool
	// Supplies are how many of each of Gifts may ever be sent, 0 for no
	// end, and Sold how many have been, in their order.
	Supplies []int
	Sold     []int
}

// GiftCatalog is what there is to send, and what collectibles may be:
// their traits, commonest first, and what upgrading a gift and giving a
// collectible away cost.
type GiftCatalog struct {
	Collections   []GiftCollection
	Models        []GiftTrait
	Backdrops     []GiftTrait
	Symbols       []GiftTrait
	UpgradePrice  int64
	TransferPrice int64
}

// GiftTrait is a model, a backdrop or a symbol of collectibles; Weight is
// its chance in tenths of a percent.
type GiftTrait struct {
	Name   string
	Weight int
}

// GiftDraft is a gift to send: its name in the catalogue, the words with
// it and whether only the recipient is told who sent it ("Анонимно").
type GiftDraft struct {
	Gift      string
	Caption   string
	Anonymous bool
}

// Gift is a gift sent to a page, as the caller sees it. Sender is nil when
// the caller is not told who sent it: it is anonymous, or the page of the
// sender is deleted; the words of an anonymous gift are not told either.
type Gift struct {
	ID        string
	Gift      string
	Caption   string
	Anonymous bool
	SentAt    time.Time
	Sender    *PublicProfile
	// Number is above zero for a collectible, with its traits; SalePrice
	// is above zero while it is on sale, Auctioned while it is at an
	// auction.
	Number    int
	Model     string
	Backdrop  string
	Symbol    string
	SalePrice int64
	Auctioned bool
}

// Collectible is a collectible gift as the caller sees it: Owner is nil
// when the site holds it (Site) or the page of the owner is gone; History
// comes only with one gift, newest first.
type Collectible struct {
	Gift    Gift
	Owner   *PublicProfile
	Site    bool
	Mine    bool
	Auction *GiftAuction
	History []GiftHistoryLine
}

// GiftAuction is a collectible up for the highest bid: Bidder is nil
// while nobody has bid, Seller when the site sells.
type GiftAuction struct {
	Start  int64
	TopBid int64
	MinBid int64
	Bids   int
	EndsAt time.Time
	Site   bool
	Seller *PublicProfile
	Bidder *PublicProfile
	Mine   bool
}

// GiftHistoryLine is where a collectible has been: From and To are nil for
// the site and for erased pages.
type GiftHistoryLine struct {
	Kind  string
	At    time.Time
	Price int64
	From  *PublicProfile
	To    *PublicProfile
}

// CollectiblePage is one screen of the market or the auctions; NextCursor
// is empty on the last.
type CollectiblePage struct {
	Collectibles []Collectible
	NextCursor   string
}

// MarketFilter is what the market shows: a gift and traits, empty for
// any, in an order ("price", "price_desc", "number").
type MarketFilter struct {
	Gift     string
	Model    string
	Backdrop string
	Symbol   string
	Sort     string
}

// GiftEdition is a collectible the site mints to auction: a gift of the
// catalogue with the traits the administrators chose.
type GiftEdition struct {
	Gift     string
	Model    string
	Backdrop string
	Symbol   string
	Start    int64
	Hours    int
}

// GiftPage is one screen of the gifts of a page, newest first; NextCursor
// is empty on the last.
type GiftPage struct {
	Gifts      []Gift
	NextCursor string
	Total      int
}

// VoteOperation is a line of the statement of votes: Amount is what came
// in (above zero) or went out (below), Balance — the votes after it,
// PeerID the other person of a transfer, a gift or a sticker pack.
type VoteOperation struct {
	ID        string
	Kind      string
	Amount    int64
	Balance   int64
	PeerID    string
	Note      string
	CreatedAt time.Time
}

// Statement is one screen of the statement of votes, newest first, with
// what came in and went out over the last 30 days; NextCursor is empty on
// the last.
type Statement struct {
	Operations []VoteOperation
	NextCursor string
	Income     int64
	Expense    int64
}

// Redeemed is what a voucher gave the caller and the votes they have now.
type Redeemed struct {
	Votes   int64
	Rating  int
	Balance int64
}

// Treasury is the treasury of the site: every vote given comes out of it,
// every vote spent goes in. It has no bottom: Votes is what it has taken
// more than paid, below zero when it paid more.
type Treasury struct {
	Votes       int64
	Outstanding int64
	Bonus       int64
	// Held is the votes on the accounts of the pages.
	Held int64
	// The fees in percent: of a transfer, of a sale of a sticker pack and
	// of the same with Prime.
	TransferFee     int
	StickerTax      int
	PrimeStickerTax int
}

// Economy is what the administrators set of the votes: the daily bonus and
// the fees in percent.
type Economy struct {
	Bonus           int64
	TransferFee     int
	StickerTax      int
	PrimeStickerTax int
}

// Transferred is what a transfer left the sender and the fee it took on
// top of the votes.
type Transferred struct {
	Votes int64
	Fee   int64
}

// RatingRaised is the votes and the rating the caller has after
// "Повысить рейтинг".
type RatingRaised struct {
	Balance int64
	Rating  int
}

// Bonus is the daily bonus of the caller: what it gives, whether it is
// taken today and whether the caller may take it.
type Bonus struct {
	Votes    int64
	Taken    bool
	Eligible bool
}

// VoucherDraft is a voucher to make: Token empty for a generated serial
// number, Usages -1 for any number of accounts.
type VoucherDraft struct {
	Token  string
	Votes  int64
	Rating int
	Usages int
}

// Voucher is a voucher as the administrators see it: what it gives, how
// many more accounts may use it (-1: any number) and how many have.
type Voucher struct {
	ID          string
	Token       string
	Votes       int64
	Rating      int
	UsagesLeft  int
	Activations int
}

// VoucherPage is one screen of the vouchers, newest first; NextCursor is
// empty on the last.
type VoucherPage struct {
	Vouchers   []Voucher
	NextCursor string
}

// StickerPack is a pack of the "Магазин стикеров": Price votes once, 0
// for a free one. Bought: the caller has taken it; Added: it is among
// their stickers. A pack of the site has no AuthorID — the site names it
// and draws it; one a person made of their photos carries its Name and
// About, and the shop shows it when Published. Holders is how many people
// have taken it, its author left out.
type StickerPack struct {
	ID        string
	Price     int64
	Stickers  []string
	Bought    bool
	Added     bool
	AuthorID  string
	Name      string
	About     string
	Published bool
	Holders   int
}

// StickerPackDraft is what the author of a pack says of it, and the
// photos added to it or taken out of it.
type StickerPackDraft struct {
	Name      string
	About     string
	Price     int64
	Published bool
	Added     []string
	Removed   []string
}

// Document is a file of "Мои Документы": its bytes are in the media
// service, the rest in the post service. Ext is its format, the extension
// of its file in lower case.
type Document struct {
	ID        string
	OwnerID   string
	Name      string
	Ext       string
	Size      int64
	CreatedAt time.Time
}

// DocumentPage is one screen of the caller's documents, newest first.
type DocumentPage struct {
	Documents  []Document
	NextCursor string
	Total      int
}

// Story is a photo with a few words shown for a day, VK's "история".
type Story struct {
	ID        string
	AuthorID  string
	PhotoID   string
	Text      string
	CreatedAt time.Time
	ExpiresAt time.Time
	// Viewed: the viewer viewed it; Mine: the viewer told it, and Views
	// is how many others viewed it.
	Viewed bool
	Mine   bool
	Views  int
	// Liked: the viewer liked it; Likes and Comments count those of the
	// pages that are not deleted.
	Liked    bool
	Likes    int
	Comments int
}

// StoryComment is a few words under a story; Mine: the viewer wrote it.
type StoryComment struct {
	ID        string
	StoryID   string
	AuthorID  string
	Text      string
	CreatedAt time.Time
	Mine      bool
}

// StoryCommentPage is one screen of the comments of a story, the oldest
// first.
type StoryCommentPage struct {
	Comments   []StoryComment
	NextCursor string
	Total      int
}

// PrimePrices are what each period of Freenet Prime costs, in votes.
type PrimePrices struct {
	Month   int64
	Quarter int64
	Year    int64
}

// PrimePurchase is what a purchase of Freenet Prime did: until when the
// recipient has it and the votes the buyer has left.
type PrimePurchase struct {
	Until   time.Time
	Balance int64
}

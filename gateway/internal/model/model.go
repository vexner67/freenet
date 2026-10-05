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

type Privacy struct {
	ShowBirthday       bool
	ShowRelationship   bool
	ShowWorldview      bool
	ShowPoliticalViews bool
}

// User is the caller's own account. AvatarID is empty without a photo.
type User struct {
	ID               string
	AvatarID         string
	Email            string
	Profile          Profile
	ProfileCompleted bool
	Privacy          Privacy
}

// PublicProfile is what anyone may see; fields hidden by the owner are empty.
type PublicProfile struct {
	ID       string
	AvatarID string
	Profile  Profile
}

// Author is who wrote a post, as shown next to it.
type Author struct {
	ID       string
	AvatarID string
	Nickname string
	Name     string
	Surname  string
	Gender   string
}

type Post struct {
	ID        string
	Author    Author
	Text      string
	CreatedAt time.Time
}

// PostPage is one screen of a wall; NextCursor is empty on the last one.
type PostPage struct {
	Posts      []Post
	NextCursor string
	Total      int
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

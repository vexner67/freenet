package service

import (
	"time"

	"github.com/google/uuid"
)

// Liker is who liked a post or a comment, and when.
type Liker struct {
	UserID  uuid.UUID
	LikedAt time.Time
}

// LikerCursor is where a page of likers ends: they are read newest first,
// by the time of the like and then the user.
type LikerCursor struct {
	LikedAt time.Time
	UserID  uuid.UUID
}

// LikerPage is one screen of openvk's "Понравилось N людям": Total is the
// counter of the post or the comment; Next is nil on the last page.
type LikerPage struct {
	Likers []Liker
	Next   *LikerCursor
	Total  int
}

// likerPage cuts found, read with one extra liker, to size.
func likerPage(found []Liker, size, total int) LikerPage {
	page := LikerPage{Likers: found, Total: total}
	if len(found) > size {
		page.Likers = found[:size]
		last := found[size-1]
		page.Next = &LikerCursor{LikedAt: last.LikedAt, UserID: last.UserID}
	}

	return page
}

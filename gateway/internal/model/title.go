package model

import "time"

// Title is an award the administrators gave a page: its name, its
// picture and when it was given.
type Title struct {
	ID        string
	Name      string
	Icon      string
	GrantedAt time.Time
}

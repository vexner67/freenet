package model

import "time"

// World is one generated map of "Мир": Status is "generating", "ready",
// "open", "ended" or "failed"; one world at a time is open, and the site
// shows its map.
type World struct {
	ID        string
	Seed      uint64
	Grid      int
	Status    string
	CreatedAt time.Time
	OpenedAt  time.Time
	EndedAt   time.Time
}

// Atlas is what the map knows of its provinces: their names, the land
// and its riches, and who borders whom.
type Atlas struct {
	Width     int
	Height    int
	Provinces []Province
	// Zooms is how many zooms of tiles there are, from 0.
	Zooms int
}

type Province struct {
	ID         int
	Sea        bool
	Name       string
	Terrain    string
	Biome      string
	Resources  []string
	CenterX    int
	CenterY    int
	Area       int
	Coastal    bool
	Culture    int
	Population int
	FreeCity   bool
	Links      []ProvinceLink
}

// ProvinceLink is a border: over a river, or a crossing by sea.
type ProvinceLink struct {
	To    int
	River bool
	Sea   bool
}

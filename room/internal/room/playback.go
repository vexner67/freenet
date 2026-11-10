package room

import (
	"math"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxQueue = 50

var youtubeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)

type PlaybackKind string

const (
	PlaybackNone    PlaybackKind = ""
	PlaybackYouTube PlaybackKind = "youtube"
	PlaybackAudio   PlaybackKind = "audio"
)

type QueueItem struct {
	kind           PlaybackKind
	mediaID, title string
}

func NewQueueItem(kind PlaybackKind, mediaID, title string) (QueueItem, error) {
	if !validMedia(kind, mediaID) || kind == PlaybackNone || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 256 {
		return QueueItem{}, ErrInvalidPlayback
	}
	return QueueItem{kind: kind, mediaID: mediaID, title: title}, nil
}
func (i QueueItem) Kind() PlaybackKind { return i.kind }
func (i QueueItem) MediaID() string    { return i.mediaID }
func (i QueueItem) Title() string      { return i.title }

type Playback struct {
	kind      PlaybackKind
	mediaID   string
	playing   bool
	position  float64
	updatedAt time.Time
	version   uint64
	queue     []QueueItem
}

type PlaybackInput struct {
	Kind     PlaybackKind
	MediaID  string
	Playing  bool
	Position float64
	Queue    []QueueItem
}

func NewPlayback(in PlaybackInput, version uint64, now time.Time) (Playback, error) {
	if !validMedia(in.Kind, in.MediaID) || math.IsNaN(in.Position) || math.IsInf(in.Position, 0) || in.Position < 0 || len(in.Queue) > MaxQueue {
		return Playback{}, ErrInvalidPlayback
	}
	if in.Kind == PlaybackNone && (in.Playing || in.Position != 0 || len(in.Queue) != 0) {
		return Playback{}, ErrInvalidPlayback
	}
	for _, item := range in.Queue {
		if _, err := NewQueueItem(item.kind, item.mediaID, item.title); err != nil {
			return Playback{}, err
		}
	}
	return Playback{kind: in.Kind, mediaID: in.MediaID, playing: in.Playing, position: in.Position, updatedAt: now.UTC(), version: version, queue: append([]QueueItem(nil), in.Queue...)}, nil
}

func validMedia(kind PlaybackKind, id string) bool {
	switch kind {
	case PlaybackNone:
		return id == ""
	case PlaybackYouTube:
		return youtubeID.MatchString(id)
	case PlaybackAudio:
		parsed, err := uuid.Parse(id)
		return err == nil && parsed != uuid.Nil
	}
	return false
}

func (p Playback) Kind() PlaybackKind   { return p.kind }
func (p Playback) MediaID() string      { return p.mediaID }
func (p Playback) Playing() bool        { return p.playing }
func (p Playback) Position() float64    { return p.position }
func (p Playback) UpdatedAt() time.Time { return p.updatedAt }
func (p Playback) Version() uint64      { return p.version }
func (p Playback) Queue() []QueueItem   { return append([]QueueItem(nil), p.queue...) }

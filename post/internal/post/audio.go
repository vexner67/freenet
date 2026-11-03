package post

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxAudioNameLen is how long an artist or a title may be.
	maxAudioNameLen = 128
	// MaxAudioSeconds is the longest song, three hours.
	MaxAudioSeconds = 3 * 60 * 60
	// maxAudios is how many songs a post or a message attaches.
	maxAudios = 10
	// maxCoverLen is how long the address of a cover may be.
	maxCoverLen = 512
)

var (
	ErrAudioNotFound     = errors.New("audio not found")
	ErrAudioNameTooLong  = errors.New("artist or title is too long")
	ErrInvalidDuration   = errors.New("invalid audio duration")
	ErrInvalidAudio      = errors.New("invalid audio id")
	ErrAudioKind         = errors.New("unknown audio source")
	ErrAudioLink         = errors.New("this is not a link to a track")
	ErrAudioCover        = errors.New("invalid audio cover")
	ErrCannotChangeAudio = errors.New("only the owner can change the audio")
	ErrTooManyAudios     = errors.New("a post has at most 10 audios")
	ErrDuplicateAudio    = errors.New("the same audio is attached twice")
)

// AudioKind is where a song plays from: a file uploaded to the media
// service, or a track of another site played by its own player.
type AudioKind string

const (
	AudioFile       AudioKind = "file"
	AudioSoundCloud AudioKind = "soundcloud"
	AudioYouTube    AudioKind = "youtube"
)

func NewAudioKind(s string) (AudioKind, error) {
	switch k := AudioKind(s); k {
	case AudioFile, AudioSoundCloud, AudioYouTube:
		return k, nil
	default:
		return "", ErrAudioKind
	}
}

func (k AudioKind) String() string {
	return string(k)
}

// AudioName is the artist or the title of a song: one line, trimmed, maybe
// empty, as the site then says "Неизвестный" or "Без названия" (openvk).
type AudioName string

func NewAudioName(s string) (AudioName, error) {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxAudioNameLen {
		return "", ErrAudioNameTooLong
	}

	return AudioName(s), nil
}

func (n AudioName) String() string {
	return string(n)
}

// AudioDuration is how long a song plays, in seconds; 0 when unknown, as
// with a track of another site.
type AudioDuration int

func NewAudioDuration(seconds int) (AudioDuration, error) {
	if seconds < 0 || seconds > MaxAudioSeconds {
		return 0, ErrInvalidDuration
	}

	return AudioDuration(seconds), nil
}

func (d AudioDuration) Seconds() int {
	return int(d)
}

var (
	// youTubeID is the ID of a video, eleven characters.
	youTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	// soundCloudPath is /artist/track, maybe with /s-secret of a private
	// track; the pages of an artist, a set or a search are not tracks.
	soundCloudPath = regexp.MustCompile(`^/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+(/s-[A-Za-z0-9]+)?$`)
)

// AudioSource is a track of another site: the address it is known by,
// one per track, so that everyone who adds it shares it, and its cover.
type AudioSource struct {
	kind  AudioKind
	link  string
	cover string
}

// NewAudioSource reads the address of a track of kind as people paste it:
// https://soundcloud.com/artist/track, youtube.com/watch?v=…, youtu.be/…,
// music.youtube.com, shorts. The cover, from the site, may be empty.
func NewAudioSource(kind AudioKind, link, cover string) (AudioSource, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return AudioSource{}, ErrAudioLink
	}

	canonical, err := canonicalLink(kind, u)
	if err != nil {
		return AudioSource{}, err
	}

	if cover != "" {
		c, err := url.Parse(cover)
		if err != nil || c.Scheme != "https" || c.Host == "" || len(cover) > maxCoverLen {
			return AudioSource{}, ErrAudioCover
		}
	}

	return AudioSource{kind: kind, link: canonical, cover: cover}, nil
}

func canonicalLink(kind AudioKind, u *url.URL) (string, error) {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.TrimSuffix(u.Path, "/")

	switch kind {
	case AudioSoundCloud:
		if (host != "soundcloud.com" && host != "m.soundcloud.com") || !soundCloudPath.MatchString(path) {
			return "", ErrAudioLink
		}

		return "https://soundcloud.com" + path, nil
	case AudioYouTube:
		var id string
		switch host {
		case "youtu.be":
			id = strings.TrimPrefix(path, "/")
		case "youtube.com", "m.youtube.com", "music.youtube.com":
			id = u.Query().Get("v")
			if rest, ok := strings.CutPrefix(path, "/shorts/"); ok {
				id = rest
			}
		}
		if !youTubeID.MatchString(id) {
			return "", ErrAudioLink
		}

		return "https://www.youtube.com/watch?v=" + id, nil
	default:
		return "", ErrAudioLink
	}
}

// RestoreAudioSource is a source from storage, validated on write.
func RestoreAudioSource(kind AudioKind, link, cover string) AudioSource {
	return AudioSource{kind: kind, link: link, cover: cover}
}

func (s AudioSource) Kind() AudioKind { return s.kind }
func (s AudioSource) Link() string    { return s.link }
func (s AudioSource) Cover() string   { return s.cover }

// Audio is a song of "Аудиозаписи" (openvk's audio): an MP3 its owner
// uploaded, its bytes in the media service under the same ID, or a track
// of SoundCloud or YouTube someone added first. People keep songs in their
// libraries and playlists; posts and messages attach them by ID.
type Audio struct {
	id        uuid.UUID
	ownerID   uuid.UUID
	kind      AudioKind
	link      string
	cover     string
	artist    AudioName
	title     AudioName
	duration  AudioDuration
	createdAt time.Time
}

// AudioDraft is what a new song is.
type AudioDraft struct {
	OwnerID  uuid.UUID
	Artist   AudioName
	Title    AudioName
	Duration AudioDuration
}

// NewAudioFile adds the MP3 id the media service keeps for the owner.
func NewAudioFile(id uuid.UUID, d AudioDraft, now time.Time) (*Audio, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidAudio
	}

	return newAudio(id, d, AudioSource{kind: AudioFile}, now), nil
}

// NewAudioTrack adds a track of another site; its UUIDv7 grows with time.
func NewAudioTrack(id uuid.UUID, d AudioDraft, source AudioSource, now time.Time) (*Audio, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidAudio
	}

	if source.kind == AudioFile || source.link == "" {
		return nil, ErrAudioLink
	}

	return newAudio(id, d, source, now), nil
}

func newAudio(id uuid.UUID, d AudioDraft, source AudioSource, now time.Time) *Audio {
	return &Audio{
		id: id, ownerID: d.OwnerID, kind: source.kind, link: source.link, cover: source.cover,
		artist: d.Artist, title: d.Title, duration: d.Duration,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}
}

// StoredAudio is a song as storage keeps it.
type StoredAudio struct {
	ID        uuid.UUID
	OwnerID   uuid.UUID
	Source    AudioSource
	Artist    AudioName
	Title     AudioName
	Duration  AudioDuration
	CreatedAt time.Time
}

func RestoreAudio(s StoredAudio) *Audio {
	return &Audio{
		id: s.ID, ownerID: s.OwnerID, kind: s.Source.kind, link: s.Source.link, cover: s.Source.cover,
		artist: s.Artist, title: s.Title, duration: s.Duration, createdAt: s.CreatedAt,
	}
}

// Edited is a's artist and title as its owner writes them; only an
// uploaded song: a track of another site is shared by all who add it.
func (a *Audio) Edited(requesterID uuid.UUID, artist, title AudioName) (*Audio, error) {
	if !a.CanBeChangedBy(requesterID) {
		return nil, ErrCannotChangeAudio
	}

	edited := *a
	edited.artist, edited.title = artist, title

	return &edited, nil
}

// CanBeChangedBy: only the owner edits or deletes an uploaded song.
func (a *Audio) CanBeChangedBy(userID uuid.UUID) bool {
	return a.kind == AudioFile && userID == a.ownerID
}

func (a *Audio) ID() uuid.UUID           { return a.id }
func (a *Audio) OwnerID() uuid.UUID      { return a.ownerID }
func (a *Audio) Source() AudioSource     { return AudioSource{kind: a.kind, link: a.link, cover: a.cover} }
func (a *Audio) Artist() AudioName       { return a.artist }
func (a *Audio) Title() AudioName        { return a.title }
func (a *Audio) Duration() AudioDuration { return a.duration }
func (a *Audio) CreatedAt() time.Time    { return a.createdAt }

// Audios are the IDs of the songs a post or a message attaches, in the
// order they are shown. Anyone's songs may be attached, as on openvk.
type Audios []uuid.UUID

func NewAudios(ids []uuid.UUID) (Audios, error) {
	if len(ids) > maxAudios {
		return nil, ErrTooManyAudios
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidAudio
		}
		if seen[id] {
			return nil, ErrDuplicateAudio
		}
		seen[id] = true
	}

	return Audios(ids), nil
}

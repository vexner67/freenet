package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

func playbackKey(id uuid.UUID) string { return "room:playback:" + id.String() }
func chatKey(id uuid.UUID) string     { return "room:chat:" + id.String() }

type queueItem struct {
	Kind           room.PlaybackKind
	MediaID, Title string
}
type playback struct {
	Kind      room.PlaybackKind
	MediaID   string
	Playing   bool
	Position  float64
	UpdatedAt time.Time
	Version   uint64
	Queue     []queueItem
}

func encodePlayback(p room.Playback) playback {
	result := playback{Kind: p.Kind(), MediaID: p.MediaID(), Playing: p.Playing(), Position: p.Position(), UpdatedAt: p.UpdatedAt(), Version: p.Version()}
	for _, q := range p.Queue() {
		result.Queue = append(result.Queue, queueItem{q.Kind(), q.MediaID(), q.Title()})
	}
	return result
}
func (p playback) domain() (room.Playback, error) {
	in := room.PlaybackInput{Kind: p.Kind, MediaID: p.MediaID, Playing: p.Playing, Position: p.Position}
	for _, q := range p.Queue {
		item, err := room.NewQueueItem(q.Kind, q.MediaID, q.Title)
		if err != nil {
			return room.Playback{}, errs.Wrap("room.NewQueueItem", err)
		}
		in.Queue = append(in.Queue, item)
	}
	result, err := room.NewPlayback(in, p.Version, p.UpdatedAt)
	return result, errs.Wrap("room.NewPlayback", err)
}

type chat struct {
	ID, RoomID, AuthorID, StickerID uuid.UUID
	Text                            room.ChatText
	CreatedAt                       time.Time
}

func encodeChat(c room.Chat) chat {
	return chat{c.ID(), c.RoomID(), c.AuthorID(), c.StickerID(), c.Text(), c.CreatedAt()}
}
func (c chat) domain() room.Chat {
	return room.RestoreChat(c.ID, c.RoomID, c.AuthorID, c.StickerID, c.Text, c.CreatedAt)
}

func (s *State) Playback(ctx context.Context, id uuid.UUID) (room.Playback, error) {
	raw, err := s.client.HGet(ctx, playbackKey(id), "state").Bytes()
	if errors.Is(err, redis.Nil) {
		return room.Playback{}, nil
	}
	if err != nil {
		return room.Playback{}, errs.Wrap("s.client.HGet", err)
	}
	var p playback
	if err = json.Unmarshal(raw, &p); err != nil {
		return room.Playback{}, errs.Wrap("json.Unmarshal", err)
	}
	return p.domain()
}

const setPlaybackScript = `
 if redis.call('EXISTS',KEYS[4])==1 then return -2 end
 if ARGV[8]~='reset' then
  if redis.call('GET',KEYS[2])~=ARGV[1] then return -4 end
  local score=redis.call('ZSCORE',KEYS[3],ARGV[2])
  if not score or tonumber(score)<=tonumber(ARGV[7])-45000 then return -4 end
 end
 local version=redis.call('HGET',KEYS[1],'version') or '0'
 if version~=ARGV[3] then return -5 end
 redis.call('HSET',KEYS[1],'version',ARGV[4],'state',ARGV[5])
 redis.call('PEXPIRE',KEYS[1],ARGV[6])
 return 1
`

func (s *State) SetPlayback(ctx context.Context, id, actor uuid.UUID, version uint64, p room.Playback) error {
	raw, err := json.Marshal(encodePlayback(p))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}
	code, err := s.client.Eval(ctx, setPlaybackScript, []string{playbackKey(id), current(actor), members(id), closed(id)}, id.String(), actor.String(), strconv.FormatUint(version, 10), strconv.FormatUint(p.Version(), 10), raw, s.ttlMS(), s.clock.Now().UnixMilli(), "member").Int64()
	if err != nil {
		return errs.Wrap("s.client.Eval", err)
	}
	return scriptStatus(code)
}

const appendChatScript = `
 if redis.call('EXISTS',KEYS[4])==1 then return -2 end
 if redis.call('GET',KEYS[2])~=ARGV[1] then return -4 end
 local score=redis.call('ZSCORE',KEYS[3],ARGV[2])
 if not score or tonumber(score)<=tonumber(ARGV[5])-45000 then return -4 end
 redis.call('RPUSH',KEYS[1],ARGV[3])
 redis.call('LTRIM',KEYS[1],-50,-1)
 redis.call('PEXPIRE',KEYS[1],ARGV[4])
 return 1
`

func (s *State) AppendChat(ctx context.Context, id uuid.UUID, c room.Chat) error {
	raw, err := json.Marshal(encodeChat(c))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}
	code, err := s.client.Eval(ctx, appendChatScript, []string{chatKey(id), current(c.AuthorID()), members(id), closed(id)}, id.String(), c.AuthorID().String(), raw, s.ttlMS(), s.clock.Now().UnixMilli()).Int64()
	if err != nil {
		return errs.Wrap("s.client.Eval", err)
	}
	return scriptStatus(code)
}
func (s *State) Chat(ctx context.Context, id uuid.UUID) ([]room.Chat, error) {
	rows, err := s.client.LRange(ctx, chatKey(id), 0, -1).Result()
	if err != nil {
		return nil, errs.Wrap("s.client.LRange", err)
	}
	result := make([]room.Chat, 0, len(rows))
	for _, raw := range rows {
		var c chat
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, errs.Wrap("json.Unmarshal", err)
		}
		result = append(result, c.domain())
	}
	return result, nil
}

const closeScript = `
 local ids=redis.call('ZRANGE',KEYS[1],0,-1,'WITHSCORES')
 local result={}
 for i=1,#ids,2 do
  local user=ids[i]
  table.insert(result,user)
  table.insert(result,redis.call('HGET',KEYS[2],user) or ids[i+1])
  table.insert(result,ids[i+1])
  local key='room:current:'..user
  if redis.call('GET',key)==ARGV[1] then redis.call('DEL',key) end
 end
 redis.call('SET',KEYS[3],'1','PX',ARGV[3])
 redis.call('DEL',KEYS[1],KEYS[2],KEYS[5],KEYS[6])
 redis.call('SREM',KEYS[4],ARGV[1])
 redis.call('SET',KEYS[7],ARGV[2],'PX',ARGV[3])
 return result
`

func (s *State) Close(ctx context.Context, id uuid.UUID, now time.Time) ([]service.Member, error) {
	rows, err := s.client.Eval(ctx, closeScript, []string{members(id), joined(id), closed(id), liveKey, playbackKey(id), chatKey(id), empty(id)}, id.String(), now.UnixMilli(), s.ttlMS()).StringSlice()
	if err != nil {
		return nil, errs.Wrap("s.client.Eval", err)
	}
	result := make([]service.Member, 0, len(rows)/3)
	for i := 0; i < len(rows); i += 3 {
		id, err := parseID(rows[i])
		if err != nil {
			return nil, errs.Wrap("parseID", err)
		}
		at, err := millis(rows[i+1])
		if err != nil {
			return nil, errs.Wrap("millis", err)
		}
		beat, err := millis(rows[i+2])
		if err != nil {
			return nil, errs.Wrap("millis", err)
		}
		result = append(result, service.Member{UserID: id, JoinedAt: at, HeartbeatAt: beat})
	}
	return result, nil
}

const unlockScript = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`

func (s *State) SweepLocked(ctx context.Context, fn func(context.Context) error) (result error) {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}
	locked, err := s.client.SetNX(ctx, "room:sweep-lock", id.String(), 9*time.Second).Result()
	if err != nil {
		return errs.Wrap("s.client.SetNX", err)
	}
	if !locked {
		return nil
	}
	defer func() {
		release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := s.client.Eval(release, unlockScript, []string{"room:sweep-lock"}, id.String()).Err(); err != nil {
			result = errors.Join(result, errs.Wrap("s.client.Eval", err))
		}
	}()
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return fn(bounded)
}

// ResetPlayback is used only after the owner's settings permission check.
func (s *State) ResetPlayback(ctx context.Context, id uuid.UUID, version uint64, p room.Playback) error {
	raw, err := json.Marshal(encodePlayback(p))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}
	code, err := s.client.Eval(ctx, setPlaybackScript, []string{playbackKey(id), current(uuid.Nil), members(id), closed(id)}, id.String(), "", strconv.FormatUint(version, 10), strconv.FormatUint(p.Version(), 10), raw, s.ttlMS(), s.clock.Now().UnixMilli(), "reset").Int64()
	if err != nil {
		return errs.Wrap("s.client.Eval", err)
	}
	return scriptStatus(code)
}

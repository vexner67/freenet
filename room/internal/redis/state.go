package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

type State struct {
	client *redis.Client
	clock  service.Clock
	ttl    time.Duration
}

func NewState(client *redis.Client, clock service.Clock, ttl time.Duration) *State {
	return &State{client: client, clock: clock, ttl: ttl}
}
func members(id uuid.UUID) string { return "room:members:" + id.String() }
func joined(id uuid.UUID) string  { return "room:joined:" + id.String() }
func current(id uuid.UUID) string { return "room:current:" + id.String() }
func empty(id uuid.UUID) string   { return "room:empty:" + id.String() }
func closed(id uuid.UUID) string  { return "room:closed:" + id.String() }

const liveKey = "room:live"

func (s *State) ttlMS() int64 { return s.ttl.Milliseconds() }
func parseID(value string) (uuid.UUID, error) {
	if value == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(value)
	return id, errs.Wrap("uuid.Parse", err)
}
func millis(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, errs.Wrap("strconv.ParseInt", err)
	}
	return time.UnixMilli(n).UTC(), nil
}
func scriptStatus(code int64) error {
	switch code {
	case 1:
		return nil
	case -1:
		return room.ErrFull
	case -2:
		return room.ErrClosed
	case -3:
		return room.ErrTooManyRooms
	case -4:
		return room.ErrNotMember
	case -5:
		return room.ErrStalePlayback
	}
	return errors.New("unexpected room storage status")
}

const joinScript = `
 if redis.call('EXISTS',KEYS[4])==1 then return {'-2'} end
 local old=redis.call('GET',KEYS[1]) or ''
 local uid=ARGV[1]
 local rid=ARGV[2]
 local now=ARGV[3]
 local ttl=ARGV[6]
 if old==rid and redis.call('ZSCORE',KEYS[2],uid) then
  redis.call('ZADD',KEYS[2],now,uid)
  redis.call('PEXPIRE',KEYS[2],ttl)
  redis.call('PEXPIRE',KEYS[3],ttl)
  redis.call('PEXPIRE',KEYS[1],ttl)
  redis.call('PEXPIRE',KEYS[5],ttl)
  return {'','','','0'}
 end
 if redis.call('ZCARD',KEYS[2])>=tonumber(ARGV[4]) then return {'-1'} end
 if redis.call('ZCARD',KEYS[2])==0 and redis.call('SCARD',KEYS[5])>=tonumber(ARGV[5]) then
  if old=='' or redis.call('ZCARD','room:members:'..old)~=1 then return {'-3'} end
 end
 local oldJoined=''
 local oldHeartbeat=''
 if old~='' then
  local oldMembers='room:members:'..old
  local oldTimes='room:joined:'..old
  oldJoined=redis.call('HGET',oldTimes,uid) or ''
  oldHeartbeat=redis.call('ZSCORE',oldMembers,uid) or ''
  redis.call('ZREM',oldMembers,uid)
  redis.call('HDEL',oldTimes,uid)
  if redis.call('ZCARD',oldMembers)==0 then
   redis.call('SREM',KEYS[5],old)
   redis.call('SET','room:empty:'..old,now,'PX',ttl)
  end
 end
 redis.call('SET',KEYS[1],rid,'PX',ttl)
 redis.call('ZADD',KEYS[2],now,uid)
 redis.call('HSET',KEYS[3],uid,now)
 redis.call('PEXPIRE',KEYS[2],ttl)
 redis.call('PEXPIRE',KEYS[3],ttl)
 redis.call('SADD',KEYS[5],rid)
 redis.call('PEXPIRE',KEYS[5],ttl)
 redis.call('DEL',KEYS[6])
 return {old,oldJoined,oldHeartbeat,'1'}
`

func (s *State) Join(ctx context.Context, id, user uuid.UUID, now time.Time, maxMembers, maxLive int) (service.Transition, error) {
	rows, err := s.client.Eval(ctx, joinScript, []string{current(user), members(id), joined(id), closed(id), liveKey, empty(id)}, user.String(), id.String(), now.UnixMilli(), maxMembers, maxLive, s.ttlMS()).StringSlice()
	if err != nil {
		return service.Transition{}, errs.Wrap("s.client.Eval", err)
	}
	if len(rows) == 1 {
		code, err := strconv.ParseInt(rows[0], 10, 64)
		if err != nil {
			return service.Transition{}, errs.Wrap("strconv.ParseInt", err)
		}
		return service.Transition{}, scriptStatus(code)
	}
	old, err := parseID(rows[0])
	if err != nil {
		return service.Transition{}, errs.Wrap("parseID", err)
	}
	at, err := millis(rows[1])
	if err != nil {
		return service.Transition{}, errs.Wrap("millis", err)
	}
	heartbeat, err := millis(rows[2])
	if err != nil {
		return service.Transition{}, errs.Wrap("millis", err)
	}
	return service.Transition{PreviousRoom: old, PreviousMember: service.Member{UserID: user, JoinedAt: at, HeartbeatAt: heartbeat}, Joined: rows[3] == "1"}, nil
}

const leaveScript = `
 local beat=redis.call('ZSCORE',KEYS[1],ARGV[1])
 if not beat then return {} end
 if ARGV[5]~='' and tonumber(beat)>tonumber(ARGV[5]) then return {} end
 local at=redis.call('HGET',KEYS[2],ARGV[1]) or beat
 redis.call('ZREM',KEYS[1],ARGV[1])
 redis.call('HDEL',KEYS[2],ARGV[1])
 if redis.call('GET',KEYS[3])==ARGV[2] then redis.call('DEL',KEYS[3]) end
 if redis.call('ZCARD',KEYS[1])==0 then
  redis.call('SREM',KEYS[4],ARGV[2])
  redis.call('SET',KEYS[5],ARGV[3],'PX',ARGV[4])
 end
 return {at,beat}
`

func (s *State) leave(ctx context.Context, id, user uuid.UUID, now time.Time, before string) (service.Member, bool, error) {
	rows, err := s.client.Eval(ctx, leaveScript, []string{members(id), joined(id), current(user), liveKey, empty(id)}, user.String(), id.String(), now.UnixMilli(), s.ttlMS(), before).StringSlice()
	if err != nil {
		return service.Member{}, false, errs.Wrap("s.client.Eval", err)
	}
	if len(rows) == 0 {
		return service.Member{}, false, nil
	}
	at, err := millis(rows[0])
	if err != nil {
		return service.Member{}, false, errs.Wrap("millis", err)
	}
	beat, err := millis(rows[1])
	if err != nil {
		return service.Member{}, false, errs.Wrap("millis", err)
	}
	return service.Member{UserID: user, JoinedAt: at, HeartbeatAt: beat}, true, nil
}
func (s *State) Leave(ctx context.Context, id, user uuid.UUID, now time.Time) (service.Member, bool, error) {
	return s.leave(ctx, id, user, now, "")
}
func (s *State) Expire(ctx context.Context, id, user uuid.UUID, before, now time.Time) (service.Member, bool, error) {
	return s.leave(ctx, id, user, now, strconv.FormatInt(before.UnixMilli(), 10))
}

const heartbeatScript = `
 local score=redis.call('ZSCORE',KEYS[1],ARGV[1])
 if redis.call('GET',KEYS[2])~=ARGV[2] or not score or tonumber(score)<=tonumber(ARGV[3])-45000 then return -4 end
 redis.call('ZADD',KEYS[1],ARGV[3],ARGV[1])
 for i=1,#KEYS do redis.call('PEXPIRE',KEYS[i],ARGV[4]) end
 return 1
`

func (s *State) Heartbeat(ctx context.Context, id, user uuid.UUID, now time.Time) error {
	code, err := s.client.Eval(ctx, heartbeatScript, []string{members(id), current(user), joined(id), liveKey}, user.String(), id.String(), now.UnixMilli(), s.ttlMS()).Int64()
	if err != nil {
		return errs.Wrap("s.client.Eval", err)
	}
	return scriptStatus(code)
}
func (s *State) Members(ctx context.Context, id uuid.UUID) ([]service.Member, error) {
	values, err := s.client.ZRangeWithScores(ctx, members(id), 0, -1).Result()
	if err != nil {
		return nil, errs.Wrap("s.client.ZRangeWithScores", err)
	}
	if len(values) == 0 {
		return []service.Member{}, nil
	}
	ids := make([]string, 0, len(values))
	for _, v := range values {
		ids = append(ids, v.Member.(string))
	}
	times, err := s.client.HMGet(ctx, joined(id), ids...).Result()
	if err != nil {
		return nil, errs.Wrap("s.client.HMGet", err)
	}
	result := make([]service.Member, 0, len(values))
	for i, v := range values {
		uid, err := parseID(ids[i])
		if err != nil {
			return nil, errs.Wrap("parseID", err)
		}
		beat := time.UnixMilli(int64(v.Score)).UTC()
		at := beat
		if times[i] != nil {
			at, err = millis(times[i].(string))
			if err != nil {
				return nil, errs.Wrap("millis", err)
			}
		}
		result = append(result, service.Member{UserID: uid, JoinedAt: at, HeartbeatAt: beat})
	}
	return result, nil
}

const currentScript = `
 local result={}
 for i=1,#KEYS do
  local rid=redis.call('GET',KEYS[i])
  if rid then
   local score=redis.call('ZSCORE','room:members:'..rid,ARGV[i])
   if score and tonumber(score)>tonumber(ARGV[#KEYS+1])-45000 then
    table.insert(result,ARGV[i]);table.insert(result,rid)
   end
  end
 end
 return result
`

func (s *State) Current(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	result := make(map[uuid.UUID]uuid.UUID)
	if len(ids) == 0 {
		return result, nil
	}
	keys := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		keys = append(keys, current(id))
		args = append(args, id.String())
	}
	args = append(args, s.clock.Now().UnixMilli())
	rows, err := s.client.Eval(ctx, currentScript, keys, args...).StringSlice()
	if err != nil {
		return nil, errs.Wrap("s.client.Eval", err)
	}
	for i := 0; i < len(rows); i += 2 {
		user, err := parseID(rows[i])
		if err != nil {
			return nil, errs.Wrap("parseID", err)
		}
		id, err := parseID(rows[i+1])
		if err != nil {
			return nil, errs.Wrap("parseID", err)
		}
		result[user] = id
	}
	return result, nil
}
func (s *State) Live(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.client.SMembers(ctx, liveKey).Result()
	if err != nil {
		return nil, errs.Wrap("s.client.SMembers", err)
	}
	result := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		id, err := parseID(row)
		if err != nil {
			return nil, errs.Wrap("parseID", err)
		}
		result = append(result, id)
	}
	return result, nil
}
func (s *State) EmptySince(ctx context.Context, id uuid.UUID) (time.Time, error) {
	value, err := s.client.Get(ctx, empty(id)).Result()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errs.Wrap("s.client.Get", err)
	}
	return millis(value)
}

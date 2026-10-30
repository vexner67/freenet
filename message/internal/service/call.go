package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // coturn's use-auth-secret is HMAC-SHA-1 by its protocol.
	"encoding/base64"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

// callServersLast is how long the credentials of a TURN server hold: a
// call goes on with its allocation refreshed under them.
const callServersLast = 12 * time.Hour

// CallSignal is a sealed signal of the call with PeerID, for whoever
// hears it. It is not kept.
type CallSignal struct {
	PeerID uuid.UUID
	Sealed string
}

// TurnConfig are the servers a call goes through when two browsers cannot
// reach each other; Secret is shared with the TURN server.
type TurnConfig struct {
	URLs   []string
	Secret string
}

// CallServers are the servers of a call and the credentials to them.
type CallServers struct {
	URLs       []string
	Username   string
	Credential string
}

// CallSignal hands sealed to the pages of peerID and the other pages of
// userID. Whether userID may write to peerID the caller has checked.
func (s *MessageService) CallSignal(ctx context.Context, userID, peerID uuid.UUID, sealed string) error {
	if userID == peerID {
		return message.ErrToYourself
	}
	if sealed == "" || len(sealed) > message.MaxCallSignal {
		return message.ErrCallSignal
	}

	s.events.Publish(ctx, peerID, Event{Call: &CallSignal{PeerID: userID, Sealed: sealed}})
	s.events.Publish(ctx, userID, Event{Call: &CallSignal{PeerID: peerID, Sealed: sealed}})

	return nil
}

// CallServers gives userID the servers of a call: the time-limited
// credentials coturn checks with its use-auth-secret.
func (s *MessageService) CallServers(userID uuid.UUID, now time.Time) CallServers {
	if len(s.turn.URLs) == 0 || s.turn.Secret == "" {
		return CallServers{}
	}

	username := strconv.FormatInt(now.Add(callServersLast).Unix(), 10) + ":" + userID.String()
	mac := hmac.New(sha1.New, []byte(s.turn.Secret))
	mac.Write([]byte(username))

	return CallServers{URLs: s.turn.URLs, Username: username, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))}
}

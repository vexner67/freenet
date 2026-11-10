// Package livekit adapts the internal HTTP API of the media server. No browser
// token can administer a room or publish a camera/screen track.
package livekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

type Config struct {
	URL, PublicURL, APIKey, APISecret string
	Timeout, TokenTTL, EmptyTTL       time.Duration
	MaxMembers                        int
}
type Client struct {
	cfg   Config
	http  *http.Client
	clock service.Clock
}

func New(cfg Config, clock service.Clock) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, clock: clock}
}

type videoGrant struct {
	Room                 string   `json:"room,omitempty"`
	RoomJoin             bool     `json:"roomJoin,omitempty"`
	RoomAdmin            bool     `json:"roomAdmin,omitempty"`
	RoomCreate           bool     `json:"roomCreate,omitempty"`
	CanPublish           *bool    `json:"canPublish,omitempty"`
	CanSubscribe         *bool    `json:"canSubscribe,omitempty"`
	CanPublishData       *bool    `json:"canPublishData,omitempty"`
	CanUpdateOwnMetadata *bool    `json:"canUpdateOwnMetadata,omitempty"`
	CanPublishSources    []string `json:"canPublishSources,omitempty"`
}
type claims struct {
	jwt.RegisteredClaims
	Video videoGrant `json:"video"`
}

func (c *Client) sign(subject string, grant videoGrant, ttl time.Duration) (string, error) {
	now := c.clock.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: c.cfg.APIKey, Subject: subject, NotBefore: jwt.NewNumericDate(now.Add(-time.Second)), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)), IssuedAt: jwt.NewNumericDate(now)}, Video: grant})
	signed, err := token.SignedString([]byte(c.cfg.APISecret))
	return signed, errs.Wrap("token.SignedString", err)
}
func (c *Client) Token(ctx context.Context, id, user uuid.UUID) (service.MediaToken, error) {
	request := struct {
		Name            string `json:"name"`
		MaxParticipants int    `json:"max_participants"`
		EmptyTimeout    int64  `json:"empty_timeout"`
	}{id.String(), c.cfg.MaxMembers, int64(c.cfg.EmptyTTL / time.Second)}
	if err := c.call(ctx, "CreateRoom", request, &struct{}{}, videoGrant{RoomCreate: true}, false); err != nil {
		return service.MediaToken{}, errors.Join(room.ErrUnavailable, errs.Wrap("c.call", err))
	}
	yes, no := true, false
	token, err := c.sign(user.String(), videoGrant{Room: id.String(), RoomJoin: true, CanPublish: &yes, CanSubscribe: &yes, CanPublishData: &no, CanUpdateOwnMetadata: &no, CanPublishSources: []string{"microphone"}}, c.cfg.TokenTTL)
	if err != nil {
		return service.MediaToken{}, errs.Wrap("c.sign", err)
	}
	return service.MediaToken{URL: c.cfg.PublicURL, Token: token, ExpiresAt: c.clock.Now().Add(c.cfg.TokenTTL)}, nil
}

type participantRequest struct {
	Room     string `json:"room"`
	Identity string `json:"identity"`
}

func (c *Client) Remove(ctx context.Context, id, user uuid.UUID) error {
	return errs.Wrap("c.call", c.call(ctx, "RemoveParticipant", participantRequest{id.String(), user.String()}, &struct{}{}, videoGrant{Room: id.String(), RoomAdmin: true}, true))
}
func (c *Client) Delete(ctx context.Context, id uuid.UUID) error {
	request := struct {
		Room string `json:"room"`
	}{id.String()}
	return errs.Wrap("c.call", c.call(ctx, "DeleteRoom", request, &struct{}{}, videoGrant{Room: id.String(), RoomAdmin: true, RoomCreate: true}, true))
}
func (c *Client) Mute(ctx context.Context, id, user uuid.UUID) error {
	var participant struct {
		Tracks []struct {
			SID  string          `json:"sid"`
			Type json.RawMessage `json:"type"`
		} `json:"tracks"`
	}
	if err := c.call(ctx, "GetParticipant", participantRequest{id.String(), user.String()}, &participant, videoGrant{Room: id.String(), RoomAdmin: true}, false); err != nil {
		return errs.Wrap("c.call", err)
	}
	for _, track := range participant.Tracks {
		if string(track.Type) != "\"AUDIO\"" && string(track.Type) != "0" && len(track.Type) != 0 {
			continue
		}
		request := struct {
			Room     string `json:"room"`
			Identity string `json:"identity"`
			TrackSID string `json:"track_sid"`
			Muted    bool   `json:"muted"`
		}{id.String(), user.String(), track.SID, true}
		if err := c.call(ctx, "MutePublishedTrack", request, &struct{}{}, videoGrant{Room: id.String(), RoomAdmin: true}, false); err != nil {
			return errs.Wrap("c.call", err)
		}
	}
	return nil
}

func (c *Client) call(ctx context.Context, method string, in, out any, grant videoGrant, missingOK bool) (resultErr error) {
	token, err := c.sign("", grant, c.cfg.Timeout+time.Minute)
	if err != nil {
		return errs.Wrap("c.sign", err)
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}
	endpoint, err := url.JoinPath(c.cfg.URL, "twirp/livekit.RoomService", method)
	if err != nil {
		return errs.Wrap("url.JoinPath", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errs.Wrap("http.NewRequestWithContext", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return errs.Wrap("c.http.Do", err)
	}
	defer func() { resultErr = errors.Join(resultErr, errs.Wrap("response.Body.Close", response.Body.Close())) }()
	if missingOK && response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("livekit %s: HTTP %d", method, response.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(out); err != nil {
		return errs.Wrap("json.Decoder.Decode", err)
	}
	return nil
}

func (c *Client) Verify(raw string) (uuid.UUID, uuid.UUID, error) {
	var claims claims
	token, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) { return []byte(c.cfg.APISecret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(c.cfg.APIKey), jwt.WithExpirationRequired(), jwt.WithTimeFunc(c.clock.Now))
	if err != nil || !token.Valid || !claims.Video.RoomJoin {
		return uuid.Nil, uuid.Nil, room.ErrForbidden
	}
	id, err := uuid.Parse(claims.Video.Room)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, uuid.Nil, room.ErrForbidden
	}
	user, err := uuid.Parse(claims.Subject)
	if err != nil || user == uuid.Nil {
		return uuid.Nil, uuid.Nil, room.ErrForbidden
	}
	return id, user, nil
}

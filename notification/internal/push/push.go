// Package push is the domain of push notifications: the browsers a user
// let show what happens while no page of the site is open, and what they
// are told.
package push

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxSubscriptions a user keeps: a browser more pushes the oldest out.
	MaxSubscriptions = 10
	// MaxRecipients of one message: everyone else in the biggest chat.
	MaxRecipients = 49

	maxEndpointLen = 2048
	// keyLen is an uncompressed point of P-256, authLen the secret the
	// browser shares with whoever pushes (RFC 8291).
	keyLen  = 65
	authLen = 16
)

var (
	ErrEndpointInvalid   = errors.New("invalid push endpoint")
	ErrKeysInvalid       = errors.New("invalid push keys")
	ErrNoUser            = errors.New("push subscription without a user")
	ErrNoRecipients      = errors.New("message push without recipients")
	ErrTooManyRecipients = errors.New("too many recipients of a message push")
)

// Endpoint is where the push service of a browser takes its messages: an
// https URL, the secret address of that browser.
type Endpoint string

func NewEndpoint(s string) (Endpoint, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxEndpointLen {
		return "", ErrEndpointInvalid
	}

	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", ErrEndpointInvalid
	}

	return Endpoint(s), nil
}

func (e Endpoint) String() string {
	return string(e)
}

// Origin is the audience of the VAPID token for the endpoint (RFC 8292).
func (e Endpoint) Origin() string {
	u, err := url.Parse(string(e))
	if err != nil {
		return ""
	}

	return u.Scheme + "://" + u.Host
}

// Keys encrypt what is pushed to a browser: its P-256 public key and the
// secret it shares (PushSubscription.getKey of the browser).
type Keys struct {
	P256DH []byte
	Auth   []byte
}

// NewKeys reads the keys in base64url, padded or not, as browsers give
// them.
func NewKeys(p256dh, auth string) (Keys, error) {
	key, err := decode(p256dh)
	if err != nil || len(key) != keyLen || key[0] != 4 {
		return Keys{}, ErrKeysInvalid
	}
	// A point off the curve would fail every push: refused at once.
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return Keys{}, ErrKeysInvalid
	}

	secret, err := decode(auth)
	if err != nil || len(secret) != authLen {
		return Keys{}, ErrKeysInvalid
	}

	return Keys{P256DH: key, Auth: secret}, nil
}

func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(s), "="))
}

// Subscription is a browser of a user that takes pushes. One browser
// pushes to one user: whoever subscribed it last.
type Subscription struct {
	id        uuid.UUID
	userID    uuid.UUID
	endpoint  Endpoint
	keys      Keys
	createdAt time.Time
}

func NewSubscription(userID uuid.UUID, endpoint Endpoint, keys Keys, now time.Time) (*Subscription, error) {
	if userID == uuid.Nil {
		return nil, ErrNoUser
	}

	if endpoint == "" {
		return nil, ErrEndpointInvalid
	}

	if len(keys.P256DH) != keyLen || len(keys.Auth) != authLen {
		return nil, ErrKeysInvalid
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Subscription{
		id:        id,
		userID:    userID,
		endpoint:  endpoint,
		keys:      keys,
		createdAt: now.UTC(),
	}, nil
}

// RestoreSubscription is a subscription read back from storage.
func RestoreSubscription(id, userID uuid.UUID, endpoint Endpoint, keys Keys, createdAt time.Time) *Subscription {
	return &Subscription{
		id:        id,
		userID:    userID,
		endpoint:  endpoint,
		keys:      keys,
		createdAt: createdAt,
	}
}

func (s *Subscription) ID() uuid.UUID        { return s.id }
func (s *Subscription) UserID() uuid.UUID    { return s.userID }
func (s *Subscription) Endpoint() Endpoint   { return s.endpoint }
func (s *Subscription) Keys() Keys           { return s.keys }
func (s *Subscription) CreatedAt() time.Time { return s.createdAt }

// Type is what a push tells of.
type Type string

const (
	// TypeNotification: a new notification ("Мои Ответы"), of Kind.
	TypeNotification Type = "notification"
	// TypeMessage: a new message in the dialog with the actor.
	TypeMessage Type = "message"
	// TypeChat: the actor wrote in the chat ChatID.
	TypeChat Type = "chat"
)

// Actor is who did it, as the push names them: a person or a community.
// Empty when not known; the browser then says it without a name.
type Actor struct {
	ID       uuid.UUID
	Name     string
	Gender   string
	Nickname string
	// Club is a community; Nickname is then its address, if it has one.
	Club     bool
	AvatarID string
}

// Payload is what one push says. It carries no words of messages: those
// of dialogs are sealed, and the push service should not learn the rest.
type Payload struct {
	Type Type
	// Kind of a notification, as the notification domain names it.
	Kind  string
	Actor Actor
	// ID of the notification or message, which collapses repeats.
	ID        uuid.UUID
	ChatID    uuid.UUID
	ChatTitle string
}

// Urgent pushes are delivered at once even to a phone saving power.
func (p Payload) Urgent() bool {
	return p.Type != TypeNotification
}

// Message is a new message: senderID wrote to recipientIDs, in a dialog
// or, when ChatID is set, in that chat.
type Message struct {
	ID           uuid.UUID
	SenderID     uuid.UUID
	RecipientIDs []uuid.UUID
	ChatID       uuid.UUID
	ChatTitle    string
}

// Validate checks a message told by the message service.
func (m Message) Validate() error {
	if m.SenderID == uuid.Nil {
		return ErrNoUser
	}

	if len(m.RecipientIDs) == 0 {
		return ErrNoRecipients
	}

	if len(m.RecipientIDs) > MaxRecipients {
		return ErrTooManyRecipients
	}

	return nil
}

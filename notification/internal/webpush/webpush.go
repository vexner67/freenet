// Package webpush pushes to browsers through their push services: the
// payload encrypted for the browser (RFC 8291, aes128gcm of RFC 8188) and
// the server named by its VAPID key (RFC 8292). Nothing of the payload is
// readable on the way: the push service only learns that something came.
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	// ttl is how long a push service keeps a push for a browser that is
	// away: a day-old message is still news.
	ttl = 24 * time.Hour
	// tokenLife is how long a VAPID token is good for; at most a day.
	tokenLife = 12 * time.Hour
	// recordSize of the encrypted content: one record holds the payload.
	recordSize = 4096
	saltLen    = 16
	keyLen     = 65
)

var errNotP256 = errors.New("the VAPID key is not of P-256")

// Sender is the service.Sender of Web Push.
type Sender struct {
	key       *ecdsa.PrivateKey
	publicKey []byte
	subject   string
	client    *http.Client
	now       func() time.Time
}

// Load reads the VAPID private key: P-256 in PEM, SEC 1 or PKCS #8.
// subject is whom the push services may contact: a mailto: or https URL.
func Load(path, subject string, timeout time.Duration) (*Sender, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path is configuration
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	key, err := parseKey(data)
	if err != nil {
		return nil, errs.Wrap("parseKey", err)
	}

	return New(key, subject, timeout)
}

func New(key *ecdsa.PrivateKey, subject string, timeout time.Duration) (*Sender, error) {
	pub, err := key.PublicKey.ECDH()
	if err != nil {
		return nil, errs.Wrap("key.PublicKey.ECDH", err)
	}

	if pub.Curve() != ecdh.P256() {
		return nil, errNotP256
	}

	return &Sender{
		key:       key,
		publicKey: pub.Bytes(),
		subject:   subject,
		client:    &http.Client{Timeout: timeout},
		now:       time.Now,
	}, nil
}

func parseKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block")
	}

	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errs.Wrap("x509.ParsePKCS8PrivateKey", err)
	}

	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errNotP256
	}

	return key, nil
}

func (s *Sender) PublicKey() string {
	return base64.RawURLEncoding.EncodeToString(s.publicKey)
}

func (s *Sender) Send(ctx context.Context, sub *push.Subscription, p push.Payload) error {
	plain, err := json.Marshal(toPayloadJSON(p))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	body, err := encrypt(plain, sub.Keys(), rand.Reader)
	if err != nil {
		return errs.Wrap("encrypt", err)
	}

	auth, err := s.authorization(sub.Endpoint())
	if err != nil {
		return errs.Wrap("s.authorization", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint().String(), bytes.NewReader(body))
	if err != nil {
		return errs.Wrap("http.NewRequestWithContext", err)
	}

	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", strconv.Itoa(int(ttl.Seconds())))
	req.Header.Set("Urgency", "normal")
	if p.Urgent() {
		req.Header.Set("Urgency", "high")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		// Without the URL: the endpoint is the browser's secret address.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return errs.Wrap("s.client.Do", err)
	}
	// Read and closed so the connection serves the next push.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusGone:
		return service.ErrSubscriptionGone
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	default:
		// The endpoint is a secret of the browser: only its host is told.
		return fmt.Errorf("push service %s answered %d", hostOf(sub.Endpoint()), resp.StatusCode)
	}
}

func hostOf(e push.Endpoint) string {
	u, err := url.Parse(e.String())
	if err != nil {
		return "?"
	}

	return u.Host
}

// authorization names this server to the push service of the endpoint:
// a VAPID token signed by the key whose public half browsers subscribed
// with.
func (s *Sender) authorization(e push.Endpoint) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"aud": e.Origin(),
		"exp": s.now().Add(tokenLife).Unix(),
		"sub": s.subject,
	})

	signed, err := token.SignedString(s.key)
	if err != nil {
		return "", errs.Wrap("token.SignedString", err)
	}

	return "vapid t=" + signed + ", k=" + s.PublicKey(), nil
}

// encrypt makes the body of a push for a browser with keys (RFC 8291): a
// fresh key of this server agrees a secret with the browser's key, mixed
// with the browser's auth secret and a random salt, and one aes128gcm
// record carries the payload.
func encrypt(plain []byte, keys push.Keys, random io.Reader) ([]byte, error) {
	server, err := ecdh.P256().GenerateKey(random)
	if err != nil {
		return nil, errs.Wrap("ecdh.P256().GenerateKey", err)
	}

	salt := make([]byte, saltLen)
	if _, err = io.ReadFull(random, salt); err != nil {
		return nil, errs.Wrap("io.ReadFull", err)
	}

	return seal(plain, keys, server, salt)
}

func seal(plain []byte, keys push.Keys, server *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	browser, err := ecdh.P256().NewPublicKey(keys.P256DH)
	if err != nil {
		return nil, errs.Wrap("ecdh.P256().NewPublicKey", err)
	}

	secret, err := server.ECDH(browser)
	if err != nil {
		return nil, errs.Wrap("server.ECDH", err)
	}

	serverPublic := server.PublicKey().Bytes()

	info := append([]byte("WebPush: info\x00"), keys.P256DH...)
	info = append(info, serverPublic...)

	ikm, err := hkdf.Key(sha256.New, secret, keys.Auth, string(info), 32)
	if err != nil {
		return nil, errs.Wrap("hkdf.Key", err)
	}

	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, errs.Wrap("hkdf.Extract", err)
	}

	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, errs.Wrap("hkdf.Expand", err)
	}

	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, errs.Wrap("hkdf.Expand", err)
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, errs.Wrap("aes.NewCipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errs.Wrap("cipher.NewGCM", err)
	}

	// The last (and only) record ends with the delimiter 2, unpadded.
	record := append(append([]byte{}, plain...), 2)
	if len(record)+gcm.Overhead() > recordSize {
		return nil, errors.New("push payload is too long")
	}

	header := make([]byte, 0, saltLen+4+1+keyLen)
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, recordSize)
	header = append(header, byte(len(serverPublic)))
	header = append(header, serverPublic...)

	return gcm.Seal(header, nonce, record, nil), nil
}

type actorJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Gender   string `json:"gender,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Club     bool   `json:"club,omitempty"`
	// Avatar is the small photo, as the site serves it.
	Avatar string `json:"avatar,omitempty"`
}

type chatJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// payloadJSON is what the service worker of the site (web/static/sw.js)
// reads.
type payloadJSON struct {
	Type  string    `json:"type"`
	Kind  string    `json:"kind,omitempty"`
	ID    string    `json:"id"`
	Actor actorJSON `json:"actor"`
	Chat  *chatJSON `json:"chat,omitempty"`
}

func toPayloadJSON(p push.Payload) payloadJSON {
	out := payloadJSON{
		Type: string(p.Type),
		Kind: p.Kind,
		ID:   p.ID.String(),
		Actor: actorJSON{
			ID:       p.Actor.ID.String(),
			Name:     p.Actor.Name,
			Gender:   p.Actor.Gender,
			Nickname: p.Actor.Nickname,
			Club:     p.Actor.Club,
		},
	}

	if p.Actor.AvatarID != "" {
		out.Actor.Avatar = "/media/avatars/" + url.PathEscape(p.Actor.ID.String()) + "/" + url.PathEscape(p.Actor.AvatarID) + "/small.jpg"
	}

	if p.Type == push.TypeChat {
		out.Chat = &chatJSON{ID: p.ChatID.String(), Title: p.ChatTitle}
	}

	return out
}

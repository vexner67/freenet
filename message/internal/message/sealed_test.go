package message_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

var (
	annKey = key("ann")
	bobKey = key("bob")
)

// key is a public key made up of a name.
func key(name string) message.Key {
	sum := sha256.Sum256([]byte(name))
	return message.Key(sum[:])
}

// sealedBy is text as a browser of from would seal it for to: the server
// never opens it, so the bytes only have to look right.
func sealedBy(from, to message.Key, text string) message.Sealed {
	b := []byte{1}
	b = append(b, from.Bytes()...)
	b = append(b, to.Bytes()...)
	b = append(b, make([]byte, 12)...)
	b = append(b, text...)
	b = append(b, make([]byte, 16)...)

	return message.Sealed(b)
}

func seal(text string) message.Sealed {
	return sealedBy(annKey, bobKey, text)
}

// said is a message of a dialog saying text, sealed.
func said(text message.Text, photos message.Photos, post uuid.UUID, forwarded message.Forwards) message.Content {
	var sealed message.Sealed
	if text != "" {
		sealed = seal(text.String())
	}

	content, err := message.NewSealedContent(sealed, photos, post, forwarded)
	if err != nil {
		panic(err)
	}

	return content
}

func TestNewKey(t *testing.T) {
	t.Parallel()

	if k, err := message.NewKey(annKey.Bytes()); err != nil || k != annKey {
		t.Fatalf("NewKey = %x, %v", k, err)
	}

	for name, b := range map[string][]byte{
		"none":      nil,
		"too short": make([]byte, 31),
		"too long":  make([]byte, 33),
		"all zero":  make([]byte, 32),
	} {
		if _, err := message.NewKey(b); !errors.Is(err, message.ErrKeyInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNewSealed(t *testing.T) {
	t.Parallel()

	good := seal("привет")
	if s, err := message.NewSealed(good.Bytes()); err != nil || s != good || s.SenderKey() != annKey || s.RecipientKey() != bobKey {
		t.Fatalf("NewSealed = %x, %v", s, err)
	}
	if s, err := message.NewSealed(nil); err != nil || s != "" || s.Bytes() != nil || s.SenderKey() != "" {
		t.Fatalf("none = %q, %v", s, err)
	}

	empty := sealedBy(annKey, bobKey, "")
	if _, err := message.NewSealed(empty.Bytes()); err != nil {
		t.Errorf("the shortest: %v", err)
	}

	version := []byte(good)
	version[0] = 2
	zero := sealedBy(annKey, message.Key(make([]byte, 32)), "x")

	for name, b := range map[string][]byte{
		"cut":          empty.Bytes()[:len(empty)-1],
		"of a version": version,
		"a zero key":   zero.Bytes(),
	} {
		if _, err := message.NewSealed(b); !errors.Is(err, message.ErrSealedInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if _, err := message.NewSealed(bytes.Repeat([]byte{1}, 640<<10+1)); !errors.Is(err, message.ErrSealedTooLong) {
		t.Errorf("too long: %v", err)
	}
}

func TestSealed_CheckKeys(t *testing.T) {
	t.Parallel()

	s := seal("x")
	if err := s.CheckKeys(annKey, bobKey); err != nil {
		t.Fatalf("the keys it is sealed with: %v", err)
	}
	if err := message.Sealed("").CheckKeys("", ""); err != nil {
		t.Fatalf("nothing sealed: %v", err)
	}

	for name, keys := range map[string][2]message.Key{
		"swapped":               {bobKey, annKey},
		"a new sender's key":    {key("ann again"), bobKey},
		"a new recipient's":     {annKey, key("bob again")},
		"a recipient with none": {annKey, ""},
	} {
		if err := s.CheckKeys(keys[0], keys[1]); !errors.Is(err, message.ErrKeyChanged) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNew_Sealed(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	if _, err := message.New(ann, bob, message.RestoreContent("привет", nil, uuid.Nil, nil), now); !errors.Is(err, message.ErrPlainText) {
		t.Errorf("plain words: %v", err)
	}

	carried := message.Forwards{{SenderID: bob, Text: "секрет", CreatedAt: now}}
	if _, err := message.New(ann, bob, message.RestoreContent("", nil, uuid.Nil, carried), now); !errors.Is(err, message.ErrPlainText) {
		t.Errorf("plain words carried on: %v", err)
	}

	// The words of what a message carries on travel sealed with its own.
	content, err := message.NewSealedContent(seal("смотри"), nil, uuid.Nil, carried)
	if err != nil {
		t.Fatal(err)
	}
	m, err := message.New(ann, bob, content, now)
	if err != nil || m.Sealed() != seal("смотри") || m.Text() != "" || m.Forwarded()[0].Text != "" || m.Forwarded()[0].SenderID != bob {
		t.Fatalf("New = %+v, %v", m, err)
	}
	if carried[0].Text != "секрет" {
		t.Error("the forwards given must not change")
	}

	if _, err = message.NewSealedContent("", nil, uuid.Nil, nil); !errors.Is(err, message.ErrTextEmpty) {
		t.Errorf("nothing: %v", err)
	}
	if _, err = m.Edited(ann, message.Revision{Text: "открыто"}, now); !errors.Is(err, message.ErrPlainText) {
		t.Errorf("an edit in plain words: %v", err)
	}

	was, other := seal("смотри"), seal("гляди")
	if edited, err := m.Edited(ann, message.Revision{Sealed: seal("вот"), BaseSealed: &was}, now); err != nil || edited.Sealed() != seal("вот") {
		t.Errorf("edited = %+v, %v", edited, err)
	}
	if _, err = m.Edited(ann, message.Revision{Sealed: seal("вот"), BaseSealed: &other}, now); !errors.Is(err, message.ErrEditConflict) {
		t.Errorf("edited from other words: %v", err)
	}
}

package postgres_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func keyOf(name string) message.Key {
	sum := sha256.Sum256([]byte(name))
	return message.Key(sum[:])
}

// seal is text as a browser would seal it: the server never opens it, so
// the bytes only have to look right.
func seal(text string) message.Sealed {
	b := []byte{1}
	b = append(b, keyOf("ann").Bytes()...)
	b = append(b, keyOf("bob").Bytes()...)
	b = append(b, make([]byte, 12)...)
	b = append(b, text...)

	return message.Sealed(append(b, make([]byte, 16)...))
}

// said is a message of a dialog saying text, sealed.
func said(text string, photos message.Photos, post uuid.UUID, forwarded message.Forwards) message.Content {
	var sealed message.Sealed
	if text != "" {
		sealed = seal(text)
	}

	content, err := message.NewSealedContent(sealed, photos, post, forwarded)
	if err != nil {
		panic(err)
	}

	return content
}

// opened is what a sealed message says, as the browser opens it.
func opened(m *message.Message) string {
	s := m.Sealed()
	if s == "" {
		return ""
	}

	return string(s[1+2*message.KeySize+12 : len(s)-16])
}

func TestKeyRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	keys := postgres.NewKeyRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	if _, err = keys.Get(ctx, ann); !errors.Is(err, message.ErrNoKey) {
		t.Fatalf("no key yet: %v", err)
	}

	for _, key := range []message.Key{keyOf("ann"), keyOf("ann anew")} {
		if err = keys.Set(ctx, ann, key); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if got, err := keys.Get(ctx, ann); err != nil || got != key {
			t.Fatalf("Get = %x, %v; want %x", got, err, key)
		}
	}
	if err = keys.Set(ctx, bob, keyOf("bob")); err != nil {
		t.Fatal(err)
	}

	if err = keys.Delete(ctx, ann); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err = keys.Delete(ctx, ann); err != nil {
		t.Fatalf("Delete twice: %v", err)
	}
	if _, err = keys.Get(ctx, ann); !errors.Is(err, message.ErrNoKey) {
		t.Fatalf("a deleted key: %v", err)
	}
	if got, err := keys.Get(ctx, bob); err != nil || got != keyOf("bob") {
		t.Fatalf("another's key = %x, %v", got, err)
	}
}

// What a message says is kept sealed as it came, byte for byte.
func TestMessageRepository_Sealed(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	sealed := seal(string([]byte{0, 0xff, 'x', 0}))
	content, err := message.NewSealedContent(sealed, nil, uuid.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := message.New(ann, bob, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got, err := messages.Get(ctx, m.ID()); err != nil || got.Sealed() != sealed || got.Text() != "" {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	photos, _ := message.New(ann, bob, said("", message.Photos{uuid.New()}, uuid.Nil, nil), time.Now())
	if err = messages.Save(ctx, photos); err != nil {
		t.Fatalf("Save photos alone: %v", err)
	}
	if got, err := messages.Get(ctx, photos.ID()); err != nil || got.Sealed() != "" {
		t.Fatalf("photos alone = %+v, %v", got, err)
	}
}

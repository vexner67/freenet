package redis_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/redis"
	"github.com/vexner67/freenet/notification/internal/service"
)

func TestChangeCodec(t *testing.T) {
	t.Parallel()

	var codec redis.ChangeCodec
	ann := uuid.New()

	for _, c := range []service.Change{service.ChangeNew, service.ChangeGone, service.ChangeRead} {
		data, err := codec.Encode(ann, c)
		if err != nil {
			t.Fatalf("Encode(%s): %v", c, err)
		}

		key, got, err := codec.Decode(data)
		if err != nil || key != ann || got != c {
			t.Errorf("round trip of %s = %v, %s, %v", c, key, got, err)
		}
	}

	for _, bad := range []string{`not json`, `{"user_id":"` + ann.String() + `","change":"exploded"}`} {
		if _, _, err := codec.Decode([]byte(bad)); err == nil {
			t.Errorf("Decode(%s) = nil, want an error", bad)
		}
	}
}

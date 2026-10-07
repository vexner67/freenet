// Package redis carries the live events of notification between replicas:
// the JSON that pkg/pubsub.Redis sends through a Redis channel.
package redis

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// Channel is the Redis channel of the events.
const Channel = "notification:events"

// ChangeCodec is a pubsub.Codec for service.Change.
type ChangeCodec struct{}

type changeJSON struct {
	UserID uuid.UUID      `json:"user_id"`
	Change service.Change `json:"change"`
}

func (ChangeCodec) Encode(userID uuid.UUID, c service.Change) ([]byte, error) {
	data, err := json.Marshal(changeJSON{UserID: userID, Change: c})
	if err != nil {
		return nil, errs.Wrap("json.Marshal", err)
	}

	return data, nil
}

func (ChangeCodec) Decode(data []byte) (uuid.UUID, service.Change, error) {
	var cj changeJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return uuid.Nil, "", errs.Wrap("json.Unmarshal", err)
	}

	switch cj.Change {
	case service.ChangeNew, service.ChangeGone, service.ChangeRead:
		return cj.UserID, cj.Change, nil
	default:
		return uuid.Nil, "", fmt.Errorf("unknown change %q", cj.Change)
	}
}

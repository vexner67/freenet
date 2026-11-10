package rest

import (
	"context"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type roomEventJSON struct {
	Type     string            `json:"type"`
	RoomID   string            `json:"roomId"`
	UserID   string            `json:"userId,omitempty"`
	User     *authorJSON       `json:"user,omitempty"`
	Room     *roomJSON         `json:"room,omitempty"`
	Message  *roomChatJSON     `json:"message,omitempty"`
	Playback *roomPlaybackJSON `json:"playback,omitempty"`
}

func (s *eventSession) forwardRooms(ctx context.Context, next model.RoomEvents) error {
	for {
		event, err := next()
		if err != nil {
			return err
		}
		out := roomEventJSON{Type: event.Kind, RoomID: event.RoomID, UserID: event.UserID}
		if event.UserID != "" {
			user, ok, err := s.peer(event.UserID)
			if err != nil {
				return err
			}
			if ok {
				out.User = &user
			}
		}
		if event.Room != nil {
			r := toRoomJSON(*event.Room, nil)
			out.Room = &r
		}
		if event.Playback != nil {
			p := toRoomPlaybackJSON(*event.Playback)
			out.Playback = &p
		}
		if event.Chat != nil {
			author, ok, err := s.peer(event.Chat.AuthorID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			chat := toRoomChatJSON(*event.Chat, author)
			out.Message = &chat
		}
		if err = s.send(ctx, out); err != nil {
			return err
		}
	}
}

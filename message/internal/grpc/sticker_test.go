package grpc_test

import (
	"testing"

	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStickers(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	sent, err := s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), Sticker: "cat_hi"})
	if err != nil || sent.GetMessage().GetSticker() != "cat_hi" {
		t.Fatalf("SendMessage a sticker = %v, %v", sent, err)
	}

	for name, req := range map[string]*messagev1.SendMessageRequest{
		"an unknown shape": {RecipientId: bob.String(), Sticker: "Кот"},
		"with a post":      {RecipientId: bob.String(), Sticker: "cat_hi", PostId: uuid.NewString()},
	} {
		if _, err = s.client.SendMessage(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("a sticker %s: %v", name, err)
		}
	}

	list, err := s.client.ListMessages(s.as(t, bob), &messagev1.ListMessagesRequest{PeerId: ann.String()})
	if err != nil || len(list.GetMessages()) != 1 || list.GetMessages()[0].GetSticker() != "cat_hi" {
		t.Fatalf("ListMessages = %v, %v", list, err)
	}

	chat, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: "Горы", MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatal(err)
	}
	inChat, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chat.GetChat().GetId(), Sticker: "smile_wink"})
	if err != nil || inChat.GetMessage().GetSticker() != "smile_wink" {
		t.Fatalf("SendChatMessage a sticker = %v, %v", inChat, err)
	}

	carried, err := s.client.SendMessage(s.as(t, cat), &messagev1.SendMessageRequest{RecipientId: ann.String(), ForwardIds: []string{inChat.GetMessage().GetId()}})
	if err != nil || len(carried.GetMessage().GetForwarded()) != 1 || carried.GetMessage().GetForwarded()[0].GetSticker() != "smile_wink" {
		t.Fatalf("a forwarded sticker = %v, %v", carried, err)
	}
}

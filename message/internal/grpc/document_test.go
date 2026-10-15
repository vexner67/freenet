package grpc_test

import (
	"testing"

	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDocuments(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	doc := uuid.NewString()

	sent, err := s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), DocumentIds: []string{doc}})
	if err != nil || len(sent.GetMessage().GetDocumentIds()) != 1 || sent.GetMessage().GetDocumentIds()[0] != doc {
		t.Fatalf("SendMessage documents = %v, %v", sent, err)
	}

	for name, req := range map[string]*messagev1.SendMessageRequest{
		"a bad id":       {RecipientId: bob.String(), DocumentIds: []string{"x"}},
		"twice":          {RecipientId: bob.String(), DocumentIds: []string{doc, doc}},
		"with a sticker": {RecipientId: bob.String(), DocumentIds: []string{doc}, Sticker: "cat_hi"},
	} {
		if _, err = s.client.SendMessage(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("documents %s: %v", name, err)
		}
	}

	chat, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: "Горы", MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatal(err)
	}
	inChat, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chat.GetChat().GetId(), DocumentIds: []string{doc}})
	if err != nil || len(inChat.GetMessage().GetDocumentIds()) != 1 {
		t.Fatalf("SendChatMessage documents = %v, %v", inChat, err)
	}

	carried, err := s.client.SendMessage(s.as(t, cat), &messagev1.SendMessageRequest{RecipientId: ann.String(), ForwardIds: []string{inChat.GetMessage().GetId()}})
	if err != nil || len(carried.GetMessage().GetForwarded()) != 1 || len(carried.GetMessage().GetForwarded()[0].GetDocumentIds()) != 1 {
		t.Fatalf("forwarded documents = %v, %v", carried, err)
	}
}

package grpc_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRepliesAndReactions(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	asked, err := s.client.SendMessage(s.as(t, bob), &messagev1.SendMessageRequest{RecipientId: ann.String(), Sealed: s.sealed(t, "идёшь?", bob, ann)})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	answer, err := s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{
		RecipientId: bob.String(), Sealed: s.sealed(t, "иду", ann, bob), ReplyToId: asked.GetMessage().GetId(),
	})
	if err != nil {
		t.Fatalf("SendMessage answering: %v", err)
	}
	if q := answer.GetMessage().GetReplyTo(); q.GetId() != asked.GetMessage().GetId() || q.GetSenderId() != bob.String() || !q.GetFound() || q.GetText() != "" {
		t.Fatalf("the answer quotes %v", q)
	}

	_, err = s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), Sealed: s.sealed(t, "?", ann, bob), ReplyToId: "nope"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an answer to a bad ID: %v", err)
	}

	for _, tt := range []struct {
		name     string
		user     uuid.UUID
		reaction string
		code     codes.Code
	}{
		{"a heart", ann, "heart", codes.OK},
		{"of the sender", bob, "fire", codes.OK},
		{"not one of those", ann, "angry", codes.InvalidArgument},
		{"of a stranger", cat, "like", codes.NotFound},
	} {
		_, err := s.client.ReactToMessage(s.as(t, tt.user), &messagev1.ReactToMessageRequest{MessageId: asked.GetMessage().GetId(), Reaction: tt.reaction})
		if status.Code(err) != tt.code {
			t.Fatalf("%s: %v", tt.name, err)
		}
	}

	history, err := s.client.ListMessages(s.as(t, ann), &messagev1.ListMessagesRequest{PeerId: bob.String()})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, m := range history.GetMessages() {
		if m.GetId() == asked.GetMessage().GetId() && len(m.GetReactions()) != 2 {
			t.Fatalf("the reactions read = %v", m.GetReactions())
		}
	}

	// A chat's quote says the beginning of the words.
	chat, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: "Поход", MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	long, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chat.GetChat().GetId(), Text: strings.Repeat("слово ", 100)})
	if err != nil {
		t.Fatalf("SendChatMessage: %v", err)
	}

	reply, err := s.client.SendChatMessage(s.as(t, cat), &messagev1.SendChatMessageRequest{
		ChatId: chat.GetChat().GetId(), Text: "да", ReplyToId: long.GetMessage().GetId(),
	})
	if err != nil {
		t.Fatalf("SendChatMessage answering: %v", err)
	}
	if q := reply.GetMessage().GetReplyTo(); !q.GetTextCut() || !strings.HasPrefix(q.GetText(), "слово") || len([]rune(q.GetText())) > 200 {
		t.Fatalf("the chat's quote = %v", q)
	}

	if _, err = s.client.ReactToChatMessage(s.as(t, ann), &messagev1.ReactToChatMessageRequest{MessageId: long.GetMessage().GetId(), Reaction: "laugh"}); err != nil {
		t.Fatalf("ReactToChatMessage: %v", err)
	}
}

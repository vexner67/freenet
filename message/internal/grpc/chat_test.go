package grpc_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestChat(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: "  Поход\nв горы ", MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	chat := created.GetChat()
	if chat.GetTitle() != "Поход в горы" || chat.GetOwnerId() != ann.String() || chat.GetMemberCount() != 3 || chat.GetUnread() ||
		chat.GetState() != messagev1.ChatMemberState_CHAT_MEMBER_STATE_ACTIVE || chat.GetLastMessage().GetAction() != messagev1.ChatAction_CHAT_ACTION_CREATE {
		t.Fatalf("created = %v", chat)
	}

	sent, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chat.GetId(), Text: strings.Repeat("я", 30)})
	if err != nil {
		t.Fatalf("SendChatMessage: %v", err)
	}
	if m := sent.GetMessage(); m.GetSenderId() != bob.String() || m.GetChatId() != chat.GetId() || m.GetUnread() || m.GetAction() != messagev1.ChatAction_CHAT_ACTION_NONE ||
		!m.GetEditableUntil().AsTime().Equal(m.GetCreatedAt().AsTime().Add(24*time.Hour)) || m.GetDeletableUntil() == nil {
		t.Fatalf("sent = %v", m)
	}

	t.Run("the counter of the menu counts chats with dialogs", func(t *testing.T) {
		if _, err := s.client.SendMessage(s.as(t, dan), &messagev1.SendMessageRequest{RecipientId: ann.String(), Sealed: s.sealed(t, "привет", dan, ann)}); err != nil {
			t.Fatal(err)
		}

		unread, err := s.client.CountUnreadDialogs(s.as(t, ann), &messagev1.CountUnreadDialogsRequest{})
		if err != nil || unread.GetCount() != 2 {
			t.Fatalf("CountUnreadDialogs = %v, %v; want a dialog and a chat", unread, err)
		}
	})

	t.Run("lists carry the beginnings of texts", func(t *testing.T) {
		chats, err := s.client.ListChats(s.as(t, ann), &messagev1.ListChatsRequest{TextLimit: 10})
		if err != nil || len(chats.GetChats()) != 1 || chats.GetNextCursor() != "" {
			t.Fatalf("ListChats = %v, %v", chats, err)
		}
		if c := chats.GetChats()[0]; !c.GetUnread() || c.GetLastMessage().GetText() != strings.Repeat("я", 10) || !c.GetLastMessage().GetTextCut() || !c.GetLastMessage().GetUnread() {
			t.Fatalf("chat = %v", c)
		}

		history, err := s.client.ListChatMessages(s.as(t, ann), &messagev1.ListChatMessagesRequest{ChatId: chat.GetId(), TextLimit: 10})
		if err != nil || len(history.GetMessages()) != 2 || !history.GetMessages()[1].GetTextCut() || history.GetMessages()[0].GetUnread() {
			t.Fatalf("ListChatMessages = %v, %v", history, err)
		}

		whole, err := s.client.GetChatMessage(s.as(t, ann), &messagev1.GetChatMessageRequest{Id: sent.GetMessage().GetId()})
		if err != nil || whole.GetMessage().GetTextCut() || whole.GetMessage().GetText() != strings.Repeat("я", 30) {
			t.Fatalf("GetChatMessage = %v, %v", whole, err)
		}

		after, err := s.client.ListChatMessages(s.as(t, ann), &messagev1.ListChatMessagesRequest{ChatId: chat.GetId(), After: history.GetMessages()[0].GetId()})
		if err != nil || len(after.GetMessages()) != 1 {
			t.Fatalf("ListChatMessages after = %v, %v", after, err)
		}

		_, err = s.client.ListChatMessages(s.as(t, ann), &messagev1.ListChatMessagesRequest{ChatId: chat.GetId(), After: sent.GetMessage().GetId(), Cursor: sent.GetMessage().GetId()})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("cursor with after: %v", err)
		}
	})

	t.Run("reading", func(t *testing.T) {
		if _, err := s.client.MarkChatRead(s.as(t, ann), &messagev1.MarkChatReadRequest{ChatId: chat.GetId(), UpToId: sent.GetMessage().GetId()}); err != nil {
			t.Fatalf("MarkChatRead: %v", err)
		}

		got, err := s.client.GetChat(s.as(t, ann), &messagev1.GetChatRequest{ChatId: chat.GetId()})
		if err != nil || got.GetChat().GetUnread() || len(got.GetMembers()) != 3 || got.GetMembers()[0].GetUserId() != ann.String() {
			t.Fatalf("GetChat = %v, %v", got, err)
		}
	})

	t.Run("people", func(t *testing.T) {
		if _, err := s.client.AddChatMembers(s.as(t, bob), &messagev1.AddChatMembersRequest{ChatId: chat.GetId(), UserIds: []string{dan.String()}}); err != nil {
			t.Fatalf("AddChatMembers: %v", err)
		}
		if _, err := s.client.RenameChat(s.as(t, dan), &messagev1.RenameChatRequest{ChatId: chat.GetId(), Title: "Горы"}); err != nil {
			t.Fatalf("RenameChat: %v", err)
		}

		_, err := s.client.RemoveChatMember(s.as(t, bob), &messagev1.RemoveChatMemberRequest{ChatId: chat.GetId(), UserId: dan.String()})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("a member removes: %v", err)
		}
		if _, err = s.client.RemoveChatMember(s.as(t, ann), &messagev1.RemoveChatMemberRequest{ChatId: chat.GetId(), UserId: dan.String()}); err != nil {
			t.Fatalf("RemoveChatMember: %v", err)
		}

		got, err := s.client.GetChat(s.as(t, dan), &messagev1.GetChatRequest{ChatId: chat.GetId()})
		if err != nil || got.GetChat().GetState() != messagev1.ChatMemberState_CHAT_MEMBER_STATE_KICKED || len(got.GetMembers()) != 0 ||
			got.GetChat().GetTitle() != "Горы" || got.GetChat().GetLastMessage().GetTargetId() != dan.String() {
			t.Fatalf("GetChat of who was removed = %v, %v", got, err)
		}

		if _, err = s.client.LeaveChat(s.as(t, cat), &messagev1.LeaveChatRequest{ChatId: chat.GetId()}); err != nil {
			t.Fatalf("LeaveChat: %v", err)
		}
		if _, err = s.client.ReturnToChat(s.as(t, cat), &messagev1.ReturnToChatRequest{ChatId: chat.GetId()}); err != nil {
			t.Fatalf("ReturnToChat: %v", err)
		}
	})

	t.Run("edit and take back", func(t *testing.T) {
		id := sent.GetMessage().GetId()

		edited, err := s.client.EditChatMessage(s.as(t, bob), &messagev1.EditChatMessageRequest{Id: id, Text: "иначе"})
		if err != nil || edited.GetMessage().GetText() != "иначе" || edited.GetMessage().GetEditedAt() == nil {
			t.Fatalf("EditChatMessage = %v, %v", edited, err)
		}

		// Another window still shows the text it loaded before that edit.
		_, err = s.client.EditChatMessage(s.as(t, bob), &messagev1.EditChatMessageRequest{Id: id, Text: "поздно", BaseText: proto.String(sent.GetMessage().GetText())})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("EditChatMessage of an older text: %v", err)
		}
		if _, err = s.client.EditChatMessage(s.as(t, bob), &messagev1.EditChatMessageRequest{Id: id, Text: "иначе", BaseText: proto.String("иначе")}); err != nil {
			t.Fatalf("EditChatMessage of the text it saw: %v", err)
		}

		if _, err = s.client.DeleteChatMessage(s.as(t, bob), &messagev1.DeleteChatMessageRequest{Id: id}); err != nil {
			t.Fatalf("DeleteChatMessage: %v", err)
		}

		gone, err := s.client.GetChatMessage(s.as(t, ann), &messagev1.GetChatMessageRequest{Id: id})
		if err != nil || gone.GetMessage().GetDeletedAt() == nil || gone.GetMessage().GetText() != "" || gone.GetMessage().GetEditableUntil() != nil {
			t.Fatalf("the message taken back = %v, %v", gone, err)
		}
	})

	t.Run("statuses", func(t *testing.T) {
		stranger := uuid.New()

		for _, tt := range []struct {
			name string
			call func() error
			want codes.Code
		}{
			{"a chat of two", func() error {
				_, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{MemberIds: []string{bob.String()}})
				return err
			}, codes.InvalidArgument},
			{"a long title", func() error {
				_, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: strings.Repeat("я", 65), MemberIds: []string{bob.String(), cat.String()}})
				return err
			}, codes.InvalidArgument},
			{"a member who is no user id", func() error {
				_, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{MemberIds: []string{"bob", cat.String()}})
				return err
			}, codes.InvalidArgument},
			{"a stranger opens", func() error {
				_, err := s.client.GetChat(s.as(t, stranger), &messagev1.GetChatRequest{ChatId: chat.GetId()})
				return err
			}, codes.NotFound},
			{"a stranger writes", func() error {
				_, err := s.client.SendChatMessage(s.as(t, stranger), &messagev1.SendChatMessageRequest{ChatId: chat.GetId(), Text: "я тут"})
				return err
			}, codes.NotFound},
			{"who was removed writes", func() error {
				_, err := s.client.SendChatMessage(s.as(t, dan), &messagev1.SendChatMessageRequest{ChatId: chat.GetId(), Text: "я тут"})
				return err
			}, codes.FailedPrecondition},
			{"who was removed comes back", func() error {
				_, err := s.client.ReturnToChat(s.as(t, dan), &messagev1.ReturnToChatRequest{ChatId: chat.GetId()})
				return err
			}, codes.FailedPrecondition},
			{"an empty message", func() error {
				_, err := s.client.SendChatMessage(s.as(t, ann), &messagev1.SendChatMessageRequest{ChatId: chat.GetId(), Text: "  "})
				return err
			}, codes.InvalidArgument},
			{"someone else edits", func() error {
				_, err := s.client.EditChatMessage(s.as(t, ann), &messagev1.EditChatMessageRequest{Id: created.GetChat().GetLastMessage().GetId(), Text: "не было"})
				return err
			}, codes.PermissionDenied},
			{"no token", func() error {
				_, err := s.client.ListChats(context.Background(), &messagev1.ListChatsRequest{})
				return err
			}, codes.Unauthenticated},
		} {
			if got := status.Code(tt.call()); got != tt.want {
				t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
			}
		}
	})

	t.Run("a page that is gone leaves the chat", func(t *testing.T) {
		if _, err := s.client.PurgeUser(context.Background(), &messagev1.PurgeUserRequest{UserId: ann.String()}); err != nil {
			t.Fatalf("PurgeUser: %v", err)
		}

		got, err := s.client.GetChat(s.as(t, bob), &messagev1.GetChatRequest{ChatId: chat.GetId()})
		if err != nil || got.GetChat().GetOwnerId() != bob.String() || got.GetChat().GetMemberCount() != 2 {
			t.Fatalf("GetChat after the creator went = %v, %v", got, err)
		}
	})
}

func TestSubscribe_Chat(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatal(err)
	}
	chatID := created.GetChat().GetId()

	ctx, cancel := context.WithCancel(s.as(t, cat))
	defer cancel()

	stream, err := s.client.Subscribe(ctx, &messagev1.SubscribeRequest{})
	if err != nil {
		t.Fatal(err)
	}

	received := make(chan *messagev1.SubscribeResponse, 16)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				return
			}
			received <- resp
		}
	}()

	// The stream is open once the server has the subscription; until then
	// a message could be missed, so send until one arrives.
	var got *messagev1.SubscribeResponse
	for got == nil {
		if _, err = s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chatID, Text: "привет"}); err != nil {
			t.Fatal(err)
		}

		select {
		case got = <-received:
		case <-time.After(50 * time.Millisecond):
		}
	}

	m := got.GetChatMessage()
	if m == nil || m.GetSenderId() != bob.String() || m.GetChatId() != chatID || m.GetText() != "привет" {
		t.Fatalf("event = %v, want bob's message", got)
	}

	if _, err = s.client.MarkChatRead(s.as(t, cat), &messagev1.MarkChatReadRequest{ChatId: chatID, UpToId: m.GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RemoveChatMember(s.as(t, ann), &messagev1.RemoveChatMemberRequest{ChatId: chatID, UserId: cat.String()}); err != nil {
		t.Fatal(err)
	}

	// Earlier sends of the loop may come first; then the read mark and
	// the news that cat was removed.
	var read, kicked bool
	for !read || !kicked {
		select {
		case e := <-received:
			if r := e.GetChatRead(); r != nil && r.GetChatId() == chatID && r.GetUpToId() == m.GetId() {
				read = true
			}
			if a := e.GetChatMessage(); a.GetAction() == messagev1.ChatAction_CHAT_ACTION_KICK && a.GetTargetId() == cat.String() {
				kicked = true
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("read mark heard: %v, removal heard: %v", read, kicked)
		}
	}
}

func TestChat_ForwardShareAndSearch(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: "Поход", MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chatID := created.GetChat().GetId()

	post, albumPhoto, photo := uuid.New().String(), uuid.New().String(), uuid.New().String()

	// What bob wrote in another chat, without cat.
	elsewhere, err := s.client.CreateChat(s.as(t, bob), &messagev1.CreateChatRequest{Title: "Без Кати", MemberIds: []string{ann.String(), uuid.NewString()}})
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{
		ChatId: elsewhere.GetChat().GetId(), Text: strings.Repeat("я", 30) + " поход", PhotoIds: []string{photo},
	})
	if err != nil {
		t.Fatal(err)
	}
	dialog, err := s.client.SendMessage(s.as(t, bob), &messagev1.SendMessageRequest{RecipientId: ann.String(), Sealed: s.sealed(t, "только тебе", bob, ann)})
	if err != nil {
		t.Fatal(err)
	}

	sent, err := s.client.SendChatMessage(s.as(t, ann), &messagev1.SendChatMessageRequest{
		ChatId: chatID, Text: "гляньте", PostId: post, ForwardIds: []string{private.GetMessage().GetId()},
	})
	if err != nil {
		t.Fatalf("SendChatMessage: %v", err)
	}

	m := sent.GetMessage()
	if m.GetPostId() != post || len(m.GetForwarded()) != 1 || m.GetForwarded()[0].GetSenderId() != bob.String() ||
		len(m.GetForwarded()[0].GetPhotoIds()) != 1 || m.GetForwarded()[0].GetPhotoIds()[0] == photo {
		t.Fatalf("sent = %v", m)
	}

	shared, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chatID, PhotoId: albumPhoto})
	if err != nil || shared.GetMessage().GetPhotoId() != albumPhoto {
		t.Fatalf("a photo of an album = %v, %v", shared, err)
	}

	t.Run("a chat message goes on into a dialog", func(t *testing.T) {
		on, err := s.client.SendMessage(s.as(t, cat), &messagev1.SendMessageRequest{RecipientId: uuid.NewString(), ForwardIds: []string{m.GetId()}})
		if err != nil || len(on.GetMessage().GetForwarded()) != 2 || on.GetMessage().GetForwarded()[1].GetDepth() != 1 {
			t.Fatalf("SendMessage = %v, %v", on, err)
		}
	})

	t.Run("lists cut what is carried on", func(t *testing.T) {
		history, err := s.client.ListChatMessages(s.as(t, cat), &messagev1.ListChatMessagesRequest{ChatId: chatID, TextLimit: 10})
		if err != nil || len(history.GetMessages()) != 3 {
			t.Fatalf("ListChatMessages = %v, %v", history, err)
		}
		if got := history.GetMessages()[1]; !got.GetTextCut() || got.GetText() != "гляньте" || got.GetForwarded()[0].GetText() != strings.Repeat("я", 10) {
			t.Fatalf("the message = %v", got)
		}

		around, err := s.client.ListChatMessages(s.as(t, cat), &messagev1.ListChatMessagesRequest{ChatId: chatID, Around: m.GetId(), Limit: 2})
		if err != nil || len(around.GetMessages()) != 3 || around.GetMessages()[1].GetId() != m.GetId() || around.GetNextCursor() != "" {
			t.Fatalf("around = %v, %v", around, err)
		}
	})

	t.Run("search", func(t *testing.T) {
		found, err := s.client.SearchChatMessages(s.as(t, cat), &messagev1.SearchChatMessagesRequest{Query: "ПОХОД", TextLimit: 10})
		if err != nil || len(found.GetMessages()) != 1 || found.GetMessages()[0].GetId() != m.GetId() || found.GetNextCursor() != "" {
			t.Fatalf("SearchChatMessages = %v, %v", found, err)
		}
		// The text carried on is cut to where it says the query.
		if got := found.GetMessages()[0]; !got.GetTextCut() || !strings.Contains(got.GetForwarded()[0].GetText(), "поход") {
			t.Fatalf("found = %v", got)
		}

		if found, err = s.client.SearchChatMessages(s.as(t, cat), &messagev1.SearchChatMessagesRequest{Query: "поход", ChatId: chatID}); err != nil || len(found.GetMessages()) != 1 {
			t.Fatalf("one chat = %v, %v", found, err)
		}
		if found, err = s.client.SearchChatMessages(s.as(t, uuid.New()), &messagev1.SearchChatMessagesRequest{Query: "поход"}); err != nil || len(found.GetMessages()) != 0 {
			t.Fatalf("a stranger finds %v, %v", found, err)
		}
	})

	t.Run("edit photos", func(t *testing.T) {
		withPhoto, err := s.client.SendChatMessage(s.as(t, ann), &messagev1.SendChatMessageRequest{ChatId: chatID, PhotoIds: []string{photo}})
		if err != nil {
			t.Fatal(err)
		}

		added := uuid.NewString()

		edited, err := s.client.EditChatMessage(s.as(t, ann), &messagev1.EditChatMessageRequest{
			Id: withPhoto.GetMessage().GetId(), RemovePhotoIds: []string{photo}, AddPhotoIds: []string{added},
		})
		if err != nil || len(edited.GetMessage().GetPhotoIds()) != 1 || edited.GetMessage().GetPhotoIds()[0] != added {
			t.Fatalf("EditChatMessage = %v, %v", edited, err)
		}
	})

	t.Run("statuses", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			call func() error
			want codes.Code
		}{
			{"a message one does not have is carried on", func() error {
				_, err := s.client.SendChatMessage(s.as(t, cat), &messagev1.SendChatMessageRequest{ChatId: chatID, ForwardIds: []string{private.GetMessage().GetId()}})
				return err
			}, codes.InvalidArgument},
			{"a message of a dialog is carried into a chat", func() error {
				_, err := s.client.SendChatMessage(s.as(t, ann), &messagev1.SendChatMessageRequest{ChatId: chatID, ForwardIds: []string{dialog.GetMessage().GetId()}})
				return err
			}, codes.InvalidArgument},
			{"a photo of an album with a post", func() error {
				_, err := s.client.SendChatMessage(s.as(t, ann), &messagev1.SendChatMessageRequest{ChatId: chatID, PhotoId: albumPhoto, PostId: post})
				return err
			}, codes.InvalidArgument},
			{"an empty query", func() error {
				_, err := s.client.SearchChatMessages(s.as(t, ann), &messagev1.SearchChatMessagesRequest{Query: " "})
				return err
			}, codes.InvalidArgument},
			{"a search of a chat one is not in", func() error {
				_, err := s.client.SearchChatMessages(s.as(t, uuid.New()), &messagev1.SearchChatMessagesRequest{Query: "поход", ChatId: chatID})
				return err
			}, codes.NotFound},
			{"around with a cursor", func() error {
				_, err := s.client.ListChatMessages(s.as(t, ann), &messagev1.ListChatMessagesRequest{ChatId: chatID, Around: m.GetId(), Cursor: m.GetId()})
				return err
			}, codes.InvalidArgument},
			{"around a message that is not there", func() error {
				_, err := s.client.ListChatMessages(s.as(t, ann), &messagev1.ListChatMessagesRequest{ChatId: chatID, Around: uuid.NewString()})
				return err
			}, codes.NotFound},
		} {
			if got := status.Code(tt.call()); got != tt.want {
				t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
			}
		}
	})
}

func TestChat_PinAndPhoto(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateChat(s.as(t, ann), &messagev1.CreateChatRequest{Title: "Поход", MemberIds: []string{bob.String(), cat.String()}})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	chat := created.GetChat()

	sent, err := s.client.SendChatMessage(s.as(t, bob), &messagev1.SendChatMessageRequest{ChatId: chat.GetId(), Text: strings.Repeat("я", 30)})
	if err != nil {
		t.Fatalf("SendChatMessage: %v", err)
	}

	if _, err = s.client.PinChatMessage(s.as(t, cat), &messagev1.PinChatMessageRequest{ChatId: chat.GetId(), MessageId: sent.GetMessage().GetId()}); err != nil {
		t.Fatalf("PinChatMessage: %v", err)
	}

	got, err := s.client.GetChat(s.as(t, ann), &messagev1.GetChatRequest{ChatId: chat.GetId()})
	if err != nil || got.GetPinned().GetId() != sent.GetMessage().GetId() || got.GetPinned().GetText() != strings.Repeat("я", 30) ||
		got.GetChat().GetLastMessage().GetAction() != messagev1.ChatAction_CHAT_ACTION_PIN {
		t.Fatalf("GetChat pinned = %v, %v", got, err)
	}

	if _, err = s.client.PinChatMessage(s.as(t, cat), &messagev1.PinChatMessageRequest{ChatId: chat.GetId(), MessageId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("pinned nothing: %v", err)
	}
	if _, err = s.client.PinChatMessage(s.as(t, cat), &messagev1.PinChatMessageRequest{ChatId: chat.GetId(), MessageId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("pinned a bad id: %v", err)
	}

	if _, err = s.client.UnpinChatMessage(s.as(t, ann), &messagev1.UnpinChatMessageRequest{ChatId: chat.GetId()}); err != nil {
		t.Fatalf("UnpinChatMessage: %v", err)
	}

	photo := uuid.NewString()
	if _, err = s.client.SetChatPhoto(s.as(t, bob), &messagev1.SetChatPhotoRequest{ChatId: chat.GetId(), PhotoId: photo}); err != nil {
		t.Fatalf("SetChatPhoto: %v", err)
	}

	got, err = s.client.GetChat(s.as(t, ann), &messagev1.GetChatRequest{ChatId: chat.GetId()})
	if err != nil || got.GetPinned() != nil || got.GetChat().GetPhotoId() != photo || got.GetChat().GetLastMessage().GetAction() != messagev1.ChatAction_CHAT_ACTION_PHOTO {
		t.Fatalf("GetChat with a photo = %v, %v", got, err)
	}

	if _, err = s.client.SetChatPhoto(s.as(t, bob), &messagev1.SetChatPhotoRequest{ChatId: chat.GetId()}); err != nil {
		t.Fatalf("SetChatPhoto removing: %v", err)
	}
	if _, err = s.client.SetChatPhoto(s.as(t, dan), &messagev1.SetChatPhotoRequest{ChatId: chat.GetId(), PhotoId: photo}); status.Code(err) != codes.NotFound {
		t.Fatalf("a stranger sets the photo: %v", err)
	}
}

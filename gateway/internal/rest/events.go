package rest

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	eventsPath = "/api/v1/events"

	// authWait is how long a new connection may take to send its token.
	authWait  = 10 * time.Second
	pingEvery = 30 * time.Second
	// onlineEvery is how often an open page tells that its user is on the
	// site: well within the minutes the user service counts them online.
	onlineEvery = 2 * time.Minute
	writeWait   = 10 * time.Second

	// closeTokenExpired asks the page to refresh its token and connect
	// again; 4000-4999 are the codes of the application.
	closeTokenExpired websocket.StatusCode = 4001
)

// authMessage is what a connection sends first: the token rides in the
// stream, not in the URL, which would put it in logs.
type authMessage struct {
	Token string `json:"token"`
}

// The events of the stream, told apart by type. Counters carry what
// changed: a nil one stays as it was.
type (
	countersEventJSON struct {
		Type          string `json:"type"` // "ready" first, then "counters"
		Messages      *int   `json:"messages,omitempty"`
		Notifications *int   `json:"notifications,omitempty"`
	}

	// messageEventJSON: a new message of the dialog with Peer, or one its
	// sender edited ("edit": it says something else now).
	messageEventJSON struct {
		Type    string      `json:"type"` // "message" or "edit"
		Peer    authorJSON  `json:"peer"`
		Message messageJSON `json:"message"`
	}

	// readEventJSON: the dialog with Peer was read up to UpTo, by the
	// caller on another page (ByMe) or by the peer.
	readEventJSON struct {
		Type string     `json:"type"` // "read"
		Peer authorJSON `json:"peer"`
		ByMe bool       `json:"byMe"`
		UpTo string     `json:"upTo"`
	}

	// deleteEventJSON: message ID of the dialog with Peer is gone, taken
	// back by its sender or deleted by the caller for themselves.
	deleteEventJSON struct {
		Type string     `json:"type"` // "delete"
		Peer authorJSON `json:"peer"`
		ID   string     `json:"id"`
	}

	// chatMessageEventJSON: a new message of a chat of the caller, or one
	// its sender edited or took back ("chat_edit").
	chatMessageEventJSON struct {
		Type    string          `json:"type"` // "chat_message" or "chat_edit"
		Chat    string          `json:"chat"`
		Message chatMessageJSON `json:"message"`
	}

	// chatReadEventJSON: the caller has read the chat up to UpTo, on
	// another page.
	chatReadEventJSON struct {
		Type string `json:"type"` // "chat_read"
		Chat string `json:"chat"`
		UpTo string `json:"upTo"`
	}

	notificationEventJSON struct {
		Type   string `json:"type"`   // "notification"
		Change string `json:"change"` // "new", "gone" or "read"
	}
)

// events is the live stream of a page (WebSocket): new messages and read
// marks of the caller's dialogs, changes of their notifications and the
// counters of both. The connection lives until the access token expires
// (closed with 4001: refresh and connect again), the page leaves or the
// server stops (1001); 1013 asks to connect again a bit later.
func (a *api) events(w http.ResponseWriter, r *http.Request) {
	// The server's read and write deadlines would cut a connection that
	// lives long; a hijacked one keeps them unless cleared.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: a.eventOrigins})
	if err != nil {
		return // Accept has answered
	}
	defer conn.CloseNow() //nolint:errcheck // the connection may be gone already

	a.streams.Add(1)
	defer a.streams.Done()

	a.eventStreams.Inc()
	defer a.eventStreams.Dec()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	stop := context.AfterFunc(a.stopping, cancel)
	defer stop()

	err = a.streamEvents(ctx, conn, r.WithContext(ctx))
	a.closeEvents(r.Context(), conn, err)
}

// streamEvents authenticates the connection and forwards the caller's
// events until the first error.
func (a *api) streamEvents(ctx context.Context, conn *websocket.Conn, r *http.Request) error {
	token, err := readToken(ctx, conn)
	if err != nil {
		return err
	}

	me, err := a.users.GetMe(ctx, token)
	if err != nil {
		return err
	}

	messages, err := a.messages.Subscribe(ctx, token)
	if err != nil {
		return err
	}

	notifications, err := a.notifications.Subscribe(ctx, token)
	if err != nil {
		return err
	}

	// The page sends nothing more: this reads control frames and tells
	// when the page goes. It gets a context of its own, as cancelling it
	// would drop the connection without the close frame that says why.
	gone := conn.CloseRead(context.Background())

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(gone, cancel)()

	out := make(chan any, 16)
	done := make(chan error, 3)

	s := &eventSession{api: a, r: r, token: token, me: me.ID, out: out, peers: make(map[string]authorJSON)}

	go func() { done <- s.forwardMessages(ctx, messages) }()
	go func() { done <- s.forwardNotifications(ctx, notifications) }()
	go func() { done <- ping(ctx, conn) }()

	s.markOnline(ctx)
	go s.keepOnline(ctx)

	ready, err := s.counters(ctx, "ready", true, true)
	if err != nil {
		return err
	}
	out <- ready

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err = <-done:
			return err
		case event := <-out:
			writeCtx, cancel := context.WithTimeout(ctx, writeWait)
			err = wsjson.Write(writeCtx, conn, event)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

// markOnline tells the user service the user is on the site ("Онлайн").
// A failure is only logged: the events matter more.
func (s *eventSession) markOnline(ctx context.Context) {
	if err := s.api.users.MarkOnline(ctx, s.token); err != nil && ctx.Err() == nil {
		s.api.logger.WarnContext(ctx, "mark online", "error", err)
	}
}

// keepOnline marks the user online while the page is open.
func (s *eventSession) keepOnline(ctx context.Context) {
	ticker := time.NewTicker(onlineEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.markOnline(ctx)
		}
	}
}

func readToken(ctx context.Context, conn *websocket.Conn) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()

	var msg authMessage
	if err := wsjson.Read(ctx, conn, &msg); err != nil || msg.Token == "" {
		return "", failure.New(failure.Unauthenticated, "missing access token")
	}

	return msg.Token, nil
}

func ping(ctx context.Context, conn *websocket.Conn) error {
	ticker := time.NewTicker(pingEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

// closeEvents tells the page why the stream ended, so it knows whether to
// refresh the token, wait or give up.
func (a *api) closeEvents(ctx context.Context, conn *websocket.Conn, err error) {
	switch kind := failure.From(err).Kind; {
	case a.stopping.Err() != nil:
		_ = conn.Close(websocket.StatusGoingAway, "server is stopping")
	case errors.Is(err, context.Canceled) || websocket.CloseStatus(err) != -1:
		// The page left.
	case kind == failure.Unauthenticated:
		_ = conn.Close(closeTokenExpired, "access token expired")
	case kind == failure.TooManyRequests || kind == failure.Unavailable:
		_ = conn.Close(websocket.StatusTryAgainLater, "connect again later")
	default:
		a.logger.ErrorContext(ctx, "event stream", "error", err)
		_ = conn.Close(websocket.StatusInternalError, "internal error")
	}
}

// eventSession turns one caller's service events into events of the page.
type eventSession struct {
	api   *api
	r     *http.Request
	token string
	me    string
	out   chan<- any
	// peers are the profiles already looked up, by ID.
	peers map[string]authorJSON
}

func (s *eventSession) send(ctx context.Context, event any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.out <- event:
		return nil
	}
}

func (s *eventSession) forwardMessages(ctx context.Context, next model.MessageEvents) error {
	for {
		e, err := next()
		if err != nil {
			return err
		}

		event, err := s.messageEvent(e)
		if err != nil {
			return err
		}
		if event == nil {
			continue // a peer who deleted the account
		}

		if err = s.send(ctx, event); err != nil {
			return err
		}

		// Unread dialogs change with incoming messages, own reads and
		// unread messages taken back.
		counters, err := s.counters(ctx, "counters", true, false)
		if err != nil {
			return err
		}
		if err = s.send(ctx, counters); err != nil {
			return err
		}
	}
}

func (s *eventSession) messageEvent(e model.MessageEvent) (any, error) {
	if e.Read != nil {
		byMe := e.Read.ReaderID == s.me
		peerID := e.Read.ReaderID
		if byMe {
			peerID = e.Read.PeerID
		}

		peer, ok, err := s.peer(peerID)
		if err != nil || !ok {
			return nil, err
		}

		return readEventJSON{Type: "read", Peer: peer, ByMe: byMe, UpTo: e.Read.UpTo}, nil
	}

	if d := e.Deleted; d != nil {
		peerID := d.SenderID
		if peerID == s.me {
			peerID = d.RecipientID
		}

		peer, ok, err := s.peer(peerID)
		if err != nil || !ok {
			return nil, err
		}

		return deleteEventJSON{Type: "delete", Peer: peer, ID: d.ID}, nil
	}

	if e.ChatRead != nil {
		return chatReadEventJSON{Type: "chat_read", Chat: e.ChatRead.ChatID, UpTo: e.ChatRead.UpTo}, nil
	}

	if e.ChatMessage != nil {
		return s.shownChatMessage("chat_message", *e.ChatMessage)
	}

	if e.ChatEdited != nil {
		return s.shownChatMessage("chat_edit", *e.ChatEdited)
	}

	kind := "message"
	m := e.Message
	if e.Edited != nil {
		kind, m = "edit", e.Edited
	}

	return s.shownMessage(kind, *m)
}

// shownMessage is a message of the caller's dialog as an event of kind.
func (s *eventSession) shownMessage(kind string, m model.Message) (any, error) {
	peerID := m.SenderID
	if peerID == s.me {
		peerID = m.RecipientID
	}

	peer, ok, err := s.peer(peerID)
	if err != nil || !ok {
		return nil, err
	}

	j := []messageJSON{toMessageJSON(m, peerID)}
	if err = s.api.showAttached(s.r, j); err != nil {
		return nil, err
	}

	return messageEventJSON{Type: kind, Peer: peer, Message: j[0]}, nil
}

// shownChatMessage is a message of a chat of the caller as an event of
// kind; nil when the page of someone it names is gone.
func (s *eventSession) shownChatMessage(kind string, m model.ChatMessage) (any, error) {
	shown, err := s.api.shownChatMessages(s.r, s.me, []model.ChatMessage{m})
	if err != nil || len(shown) == 0 {
		return nil, err
	}

	return chatMessageEventJSON{Type: kind, Chat: m.ChatID, Message: shown[0]}, nil
}

// peer is the profile of the user with id; false when the account is gone.
func (s *eventSession) peer(id string) (authorJSON, bool, error) {
	if p, ok := s.peers[id]; ok {
		return p, true, nil
	}

	found, err := s.api.people(s.r, []string{id})
	if err != nil {
		return authorJSON{}, false, err
	}

	author, ok := found[id]
	if !ok {
		return authorJSON{}, false, nil
	}

	s.peers[id] = toAuthorJSON(author)

	return s.peers[id], true, nil
}

func (s *eventSession) forwardNotifications(ctx context.Context, next model.NotificationChanges) error {
	for {
		change, err := next()
		if err != nil {
			return err
		}

		if err = s.send(ctx, notificationEventJSON{Type: "notification", Change: change}); err != nil {
			return err
		}

		counters, err := s.counters(ctx, "counters", false, true)
		if err != nil {
			return err
		}
		if err = s.send(ctx, counters); err != nil {
			return err
		}
	}
}

// counters counts what is asked for afresh: the source of truth is the
// services, events only say when to look.
func (s *eventSession) counters(ctx context.Context, typ string, messages, notifications bool) (countersEventJSON, error) {
	event := countersEventJSON{Type: typ}

	if messages {
		n, err := s.api.messages.CountUnread(ctx, s.token)
		if err != nil {
			return countersEventJSON{}, err
		}
		event.Messages = &n
	}

	if notifications {
		n, err := s.api.notifications.CountUnread(ctx, s.token)
		if err != nil {
			return countersEventJSON{}, err
		}
		event.Notifications = &n
	}

	return event, nil
}

// eventOrigins are the hosts of allowed cross-origin pages, as the
// WebSocket handshake checks them; the API's own origin always may.
func eventOrigins(allowedOrigins []string) []string {
	hosts := make([]string, 0, len(allowedOrigins))

	for _, origin := range allowedOrigins {
		if u, err := url.Parse(origin); err == nil && u.Host != "" {
			hosts = append(hosts, u.Host)
		}
	}

	return hosts
}

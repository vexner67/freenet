package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeVotes struct {
	token, userID, code, cursor, deleted, note, flow string
	votes                                            int64
	limit                                            int
	draft                                            model.VoucherDraft
	err                                              error
}

func (f *fakeVotes) Transfer(_ context.Context, token, userID string, votes int64, note string) (model.Transferred, error) {
	f.token, f.userID, f.votes, f.note = token, userID, votes, note
	return model.Transferred{Votes: 6, Fee: 1}, f.err
}

func (f *fakeVotes) RaiseRating(_ context.Context, token string, percent int) (model.RatingRaised, error) {
	f.token, f.limit = token, percent
	return model.RatingRaised{Balance: 85, Rating: percent}, f.err
}

func (f *fakeVotes) Statement(_ context.Context, token, cursor, flow string, limit int) (model.Statement, error) {
	f.token, f.cursor, f.flow, f.limit = token, cursor, flow, limit
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	return model.Statement{
		Operations: []model.VoteOperation{
			{ID: "o3", Kind: "transfer", Amount: -4, Balance: 6, PeerID: "u2", Note: "За пиццу", CreatedAt: at},
			{ID: "o2", Kind: "gift", Amount: -1, Balance: 10, PeerID: "gone", Note: "cake", CreatedAt: at},
			{ID: "o1", Kind: "voucher", Amount: 11, Balance: 11, CreatedAt: at},
		},
		NextCursor: "o1",
		Income:     11,
		Expense:    5,
	}, f.err
}

func (f *fakeVotes) Redeem(_ context.Context, token, code string) (model.Redeemed, error) {
	f.token, f.code = token, code
	return model.Redeemed{Votes: 10, Rating: 5, Balance: 12}, f.err
}

func (f *fakeVotes) CreateVoucher(_ context.Context, token string, d model.VoucherDraft) (model.Voucher, error) {
	f.token, f.draft = token, d
	return model.Voucher{ID: "v9", Token: "ABCDEFGH12345678JKLMNPQR", Votes: d.Votes, Rating: d.Rating, UsagesLeft: d.Usages}, f.err
}

func (f *fakeVotes) ListVouchers(_ context.Context, token, cursor string, limit int) (model.VoucherPage, error) {
	f.token, f.cursor, f.limit = token, cursor, limit
	return model.VoucherPage{
		Vouchers: []model.Voucher{
			{ID: "v2", Token: "WELCOME00000000000000000", Rating: 20, UsagesLeft: -1, Activations: 3},
			{ID: "v1", Token: "ABCDEFGH12345678JKLMNPQR", Votes: 10, Activations: 2},
		},
		NextCursor: "v1",
	}, f.err
}

func (f *fakeVotes) DeleteVoucher(_ context.Context, token, voucherID string) error {
	f.token, f.deleted = token, voucherID
	return f.err
}

func (f *fakeVotes) Rate(context.Context) (int64, error) {
	return 700, f.err
}

func (f *fakeVotes) SetRate(_ context.Context, token string, kopecks int64) error {
	f.token, f.votes = token, kopecks
	return f.err
}

func (f *fakeVotes) Treasury(context.Context) (model.Treasury, error) {
	return model.Treasury{Votes: 26000, Reserve: 20000, Free: 5800, Minted: 1000, Outstanding: 200, Bonus: 10, MaxWin: 1000}, f.err
}

func (f *fakeVotes) Bonus(_ context.Context, token string) (model.Bonus, error) {
	f.token = token
	return model.Bonus{Votes: 10, Eligible: true}, f.err
}

func (f *fakeVotes) ClaimBonus(_ context.Context, token string) (model.Redeemed, error) {
	f.token = token
	return model.Redeemed{Votes: 10, Balance: 15}, f.err
}

type voucherBody struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Votes       int64  `json:"votes"`
	Rating      int    `json:"rating"`
	UsagesLeft  int    `json:"usagesLeft"`
	Activations int    `json:"activations"`
}

func TestRedeemVoucher(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/vouchers", `{"code":"abcd-efgh"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Votes   int64 `json:"votes"`
		Rating  int   `json:"rating"`
		Balance int64 `json:"balance"`
	}](t, w)
	if w.Code != http.StatusOK || got.Votes != 10 || got.Rating != 5 || got.Balance != 12 || h.votes.code != "abcd-efgh" || h.votes.token != "tok" {
		t.Fatalf("%d %+v, asked %+v", w.Code, got, h.votes)
	}

	h = newHarness(rest.Config{})
	h.votes.err = failure.New(failure.Conflict, "voucher is not valid")
	w = h.do("POST", "/api/v1/me/vouchers", `{"code":"nope"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusConflict || decode[errorBody](t, w).Error.Message != "voucher is not valid" {
		t.Fatalf("a wrong voucher: %d %s", w.Code, w.Body)
	}

	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/vouchers", `{"code":"abcd"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/vouchers", `{"token":"abcd"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown field: %d", w.Code)
	}
}

func TestTransferVotes(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/users/bob/votes", `{"votes":4,"note":"За пиццу"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Votes int64 `json:"votes"`
		Fee   int64 `json:"fee"`
	}](t, w)
	if w.Code != http.StatusOK || got.Votes != 6 || got.Fee != 1 || h.votes.userID != "u2" || h.votes.votes != 4 || h.votes.token != "tok" || h.votes.note != "За пиццу" {
		t.Fatalf("%d %+v, asked %+v", w.Code, got, h.votes)
	}

	// What the user service refuses comes with its words.
	for message, kind := range map[string]failure.Kind{
		"you cannot transfer votes to this user": failure.PermissionDenied,
		"not enough votes":                       failure.Conflict,
		"you cannot transfer votes to yourself":  failure.InvalidArgument,
	} {
		h = newHarness(rest.Config{})
		h.votes.err = failure.New(kind, message)
		w = h.do("POST", "/api/v1/users/bob/votes", `{"votes":4}`, "Authorization", "Bearer tok")
		if w.Code < 400 || decode[errorBody](t, w).Error.Message != message {
			t.Fatalf("%s: %d %s", message, w.Code, w.Body)
		}
	}

	h = newHarness(rest.Config{})
	if w = h.do("POST", "/api/v1/users/nobody/votes", `{"votes":4}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound || h.votes.userID != "" {
		t.Fatalf("to nobody: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/users/bob/votes", `{"votes":"four"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("a broken body: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/users/bob/votes", `{"votes":4}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestVoteStatement(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/me/votes?cursor=o9&flow=out&limit=3", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Operations []struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Amount  int64  `json:"amount"`
			Balance int64  `json:"balance"`
			Peer    *struct {
				Nickname string `json:"nickname"`
			} `json:"peer"`
			Note      string `json:"note"`
			CreatedAt string `json:"createdAt"`
		} `json:"operations"`
		NextCursor string `json:"nextCursor"`
		Income     int64  `json:"income"`
		Expense    int64  `json:"expense"`
	}](t, w)
	if w.Code != http.StatusOK || h.votes.token != "tok" || h.votes.cursor != "o9" || h.votes.flow != "out" || h.votes.limit != 3 {
		t.Fatalf("%d, asked %+v", w.Code, h.votes)
	}
	if got.NextCursor != "o1" || got.Income != 11 || got.Expense != 5 || len(got.Operations) != 3 {
		t.Fatalf("statement = %+v", got)
	}
	first := got.Operations[0]
	if first.Kind != "transfer" || first.Amount != -4 || first.Balance != 6 || first.Peer == nil || first.Peer.Nickname != "bob" ||
		first.Note != "За пиццу" || first.CreatedAt != "2026-10-21T12:00:00Z" {
		t.Fatalf("the transfer = %+v", first)
	}
	// A peer who is gone and no peer at all are both null.
	if got.Operations[1].Peer != nil || got.Operations[2].Peer != nil {
		t.Fatalf("peers = %+v", got.Operations)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/me/votes", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/me/votes?limit=x", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("a wrong limit: %d", w.Code)
	}
}

func TestVouchers(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/vouchers?cursor=v5&limit=2", "", "Authorization", "Bearer tok")
	list := decode[struct {
		Vouchers   []voucherBody `json:"vouchers"`
		NextCursor string        `json:"nextCursor"`
	}](t, w)
	if w.Code != http.StatusOK || list.NextCursor != "v1" || len(list.Vouchers) != 2 || h.votes.cursor != "v5" || h.votes.limit != 2 || h.votes.token != "tok" {
		t.Fatalf("%d %+v, asked %+v", w.Code, list, h.votes)
	}
	if want := (voucherBody{ID: "v2", Code: "WELCOME00000000000000000", Rating: 20, UsagesLeft: -1, Activations: 3}); list.Vouchers[0] != want {
		t.Fatalf("the first voucher = %+v", list.Vouchers[0])
	}

	w = h.do("POST", "/api/v1/vouchers", `{"code":"abc","votes":10,"rating":5,"usages":-1}`, "Authorization", "Bearer tok")
	created := decode[voucherBody](t, w)
	if w.Code != http.StatusCreated || created.ID != "v9" || created.Code != "ABCDEFGH12345678JKLMNPQR" || created.Votes != 10 || created.UsagesLeft != -1 {
		t.Fatalf("%d %+v", w.Code, created)
	}
	if want := (model.VoucherDraft{Token: "abc", Votes: 10, Rating: 5, Usages: -1}); h.votes.draft != want {
		t.Fatalf("created %+v", h.votes.draft)
	}

	if w = h.do("DELETE", "/api/v1/vouchers/v1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.votes.deleted != "v1" {
		t.Fatalf("%d, deleted %q", w.Code, h.votes.deleted)
	}

	// Vouchers are the administrators'.
	h = newHarness(rest.Config{})
	h.votes.err = failure.New(failure.PermissionDenied, "permission denied")
	for _, call := range [][3]string{
		{"GET", "/api/v1/vouchers", ""},
		{"POST", "/api/v1/vouchers", `{"votes":1,"usages":1}`},
		{"DELETE", "/api/v1/vouchers/v1", ""},
	} {
		if w = h.do(call[0], call[1], call[2], "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s by a stranger: %d", call[0], call[1], w.Code)
		}
		if w = newHarness(rest.Config{}).do(call[0], call[1], call[2]); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: %d", call[0], call[1], w.Code)
		}
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/vouchers?limit=x", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("a broken limit: %d", w.Code)
	}
}

func TestVoteRate(t *testing.T) {
	t.Parallel()

	// Anyone reads the rate, without signing in.
	w := newHarness(rest.Config{}).do("GET", "/api/v1/vote-rate", "")
	if got := decode[map[string]int64](t, w); w.Code != http.StatusOK || got["kopecks"] != 700 {
		t.Fatalf("%d %v", w.Code, got)
	}

	h := newHarness(rest.Config{})
	if w = h.do("PUT", "/api/v1/vote-rate", `{"kopecks":1250}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent ||
		h.votes.token != "tok" || h.votes.votes != 1250 {
		t.Fatalf("set: %d, %+v", w.Code, h.votes)
	}

	if w = newHarness(rest.Config{}).do("PUT", "/api/v1/vote-rate", `{"kopecks":1250}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	h.votes.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("PUT", "/api/v1/vote-rate", `{"kopecks":1250}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("not an administrator: %d", w.Code)
	}
}

func TestTreasury(t *testing.T) {
	t.Parallel()

	w := newHarness(rest.Config{}).do("GET", "/api/v1/treasury", "")
	got := decode[struct {
		Votes  int64 `json:"votes"`
		Free   int64 `json:"free"`
		Bonus  int64 `json:"bonus"`
		MaxWin int64 `json:"maxWin"`
	}](t, w)
	if w.Code != http.StatusOK || got.Votes != 26000 || got.Free != 5800 || got.Bonus != 10 || got.MaxWin != 1000 {
		t.Fatalf("%d %+v", w.Code, got)
	}
}

func TestBonus(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/me/bonus", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Votes    int64 `json:"votes"`
		Taken    bool  `json:"taken"`
		Eligible bool  `json:"eligible"`
	}](t, w)
	if w.Code != http.StatusOK || got.Votes != 10 || got.Taken || !got.Eligible || h.votes.token != "tok" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/bonus", "", "Authorization", "Bearer tok")
	claimed := decode[struct {
		Votes   int64 `json:"votes"`
		Balance int64 `json:"balance"`
	}](t, w)
	if w.Code != http.StatusOK || claimed.Votes != 10 || claimed.Balance != 15 {
		t.Fatalf("%d %+v", w.Code, claimed)
	}

	h = newHarness(rest.Config{})
	h.votes.err = failure.New(failure.Conflict, "the bonus is taken today")
	if w = h.do("POST", "/api/v1/me/bonus", "", "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("a bonus taken: %d %s", w.Code, w.Body)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/me/bonus", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestRaiseRating(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/rating", `{"percent":5}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Balance int64 `json:"balance"`
		Rating  int   `json:"rating"`
	}](t, w)
	if w.Code != http.StatusOK || got.Balance != 85 || got.Rating != 5 || h.votes.limit != 5 || h.votes.token != "tok" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	h = newHarness(rest.Config{})
	h.votes.err = failure.New(failure.Conflict, "the rating cannot go above 1000 percent")
	if w = h.do("POST", "/api/v1/me/rating", `{"percent":5}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("past the limit: %d %s", w.Code, w.Body)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/rating", `{"percent":5}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

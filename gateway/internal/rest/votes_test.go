package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeVotes struct {
	token, userID, code, cursor, deleted string
	votes                                int64
	limit                                int
	draft                                model.VoucherDraft
	err                                  error
}

func (f *fakeVotes) Transfer(_ context.Context, token, userID string, votes int64) (int64, error) {
	f.token, f.userID, f.votes = token, userID, votes
	return 6, f.err
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
	w := h.do("POST", "/api/v1/users/bob/votes", `{"votes":4}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Votes int64 `json:"votes"`
	}](t, w)
	if w.Code != http.StatusOK || got.Votes != 6 || h.votes.userID != "u2" || h.votes.votes != 4 || h.votes.token != "tok" {
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

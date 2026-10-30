package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

type redeemVoucherRequest struct {
	// Code is the serial number, as typed: in any case, with dashes or
	// without.
	Code string `json:"code"`
}

// redeemedResponse is what a voucher gave and the votes the caller has
// now.
type redeemedResponse struct {
	Votes   int64 `json:"votes"`
	Rating  int   `json:"rating"`
	Balance int64 `json:"balance"`
}

type transferVotesRequest struct {
	Votes int64 `json:"votes"`
	// Note is the message with the transfer, may be empty.
	Note string `json:"note"`
}

// voteOperationJSON is a line of the statement of votes. Peer is the
// other person of a transfer, a gift or a sticker pack; null for none or
// a page that is gone.
type voteOperationJSON struct {
	ID        string      `json:"id"`
	Kind      string      `json:"kind"`
	Amount    int64       `json:"amount"`
	Balance   int64       `json:"balance"`
	Peer      *authorJSON `json:"peer"`
	Note      string      `json:"note"`
	CreatedAt string      `json:"createdAt"`
}

type statementResponse struct {
	Operations []voteOperationJSON `json:"operations"`
	NextCursor string              `json:"nextCursor"`
	Income     int64               `json:"income"`
	Expense    int64               `json:"expense"`
}

// transferredResponse is the votes the caller has left and the fee the
// transfer took on top.
type transferredResponse struct {
	Votes int64 `json:"votes"`
	Fee   int64 `json:"fee"`
}

type createVoucherRequest struct {
	// Code is the serial number; empty for a generated one.
	Code   string `json:"code"`
	Votes  int64  `json:"votes"`
	Rating int    `json:"rating"`
	// Usages is how many accounts may use it; -1 for any number.
	Usages int `json:"usages"`
}

type voucherJSON struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Votes  int64  `json:"votes"`
	Rating int    `json:"rating"`
	// UsagesLeft is -1 for a voucher any number of accounts may use.
	UsagesLeft  int `json:"usagesLeft"`
	Activations int `json:"activations"`
}

type voucherPageResponse struct {
	Vouchers   []voucherJSON `json:"vouchers"`
	NextCursor string        `json:"nextCursor"`
}

func toVoucherJSON(v model.Voucher) voucherJSON {
	return voucherJSON{
		ID:          v.ID,
		Code:        v.Token,
		Votes:       v.Votes,
		Rating:      v.Rating,
		UsagesLeft:  v.UsagesLeft,
		Activations: v.Activations,
	}
}

// redeemVoucher is "Активировать ваучер": the caller gets what the voucher
// carries, once.
func (a *api) redeemVoucher(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req redeemVoucherRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	redeemed, err := a.votes.Redeem(r.Context(), token, req.Code)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, redeemedResponse(redeemed))
}

// transferVotes is "Перевод голосов": the caller hands some of their votes
// to a person. Whether that person takes them is the user service's to
// say.
func (a *api) transferVotes(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req transferVotesRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	recipient, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	sent, err := a.votes.Transfer(r.Context(), token, recipient.ID, req.Votes, req.Note)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, transferredResponse(sent))
}

// voteStatement is the statement of the caller's votes ("Голоса"): every
// change of them, newest first, with the people they were with.
func (a *api) voteStatement(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	q := r.URL.Query()

	statement, err := a.votes.Statement(r.Context(), token, q.Get("cursor"), q.Get("flow"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	peerIDs := make([]string, 0, len(statement.Operations))
	for _, op := range statement.Operations {
		if op.PeerID != "" {
			peerIDs = append(peerIDs, op.PeerID)
		}
	}

	peers, err := a.people(r, peerIDs)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := statementResponse{
		Operations: make([]voteOperationJSON, 0, len(statement.Operations)),
		NextCursor: statement.NextCursor,
		Income:     statement.Income,
		Expense:    statement.Expense,
	}

	for _, op := range statement.Operations {
		line := voteOperationJSON{
			ID:        op.ID,
			Kind:      op.Kind,
			Amount:    op.Amount,
			Balance:   op.Balance,
			Note:      op.Note,
			CreatedAt: op.CreatedAt.UTC().Format(time.RFC3339),
		}

		if peer, ok := peers[op.PeerID]; ok {
			author := toAuthorJSON(peer)
			line.Peer = &author
		}

		resp.Operations = append(resp.Operations, line)
	}

	writeJSON(w, http.StatusOK, resp)
}

// listVouchers is "Ваучеры" of the administrators: every voucher, newest
// first.
func (a *api) listVouchers(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.votes.ListVouchers(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := voucherPageResponse{Vouchers: make([]voucherJSON, 0, len(page.Vouchers)), NextCursor: page.NextCursor}
	for _, v := range page.Vouchers {
		resp.Vouchers = append(resp.Vouchers, toVoucherJSON(v))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *api) createVoucher(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req createVoucherRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	voucher, err := a.votes.CreateVoucher(r.Context(), token, model.VoucherDraft{
		Token:  req.Code,
		Votes:  req.Votes,
		Rating: req.Rating,
		Usages: req.Usages,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toVoucherJSON(voucher))
}

func (a *api) deleteVoucher(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.votes.DeleteVoucher(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// voteRateJSON is what a vote is worth in rubles, in kopecks.
type voteRateJSON struct {
	Kopecks int64 `json:"kopecks"`
}

// voteRate is public: the site tells everyone what a vote is worth.
func (a *api) voteRate(w http.ResponseWriter, r *http.Request) {
	kopecks, err := a.votes.Rate(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, voteRateJSON{Kopecks: kopecks})
}

// setVoteRate is "Подарки" of the admin panel: an administrator says what
// a vote is worth from now on.
func (a *api) setVoteRate(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req voteRateJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.votes.SetRate(r.Context(), token, req.Kopecks); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// treasuryJSON is the treasury of the site.
type treasuryJSON struct {
	Votes           int64 `json:"votes"`
	Outstanding     int64 `json:"outstanding"`
	Bonus           int64 `json:"bonus"`
	Held            int64 `json:"held"`
	TransferFee     int   `json:"transferFee"`
	StickerTax      int   `json:"stickerTax"`
	PrimeStickerTax int   `json:"primeStickerTax"`
}

// treasury is public: everyone may see how many votes the site holds.
func (a *api) treasury(w http.ResponseWriter, r *http.Request) {
	t, err := a.votes.Treasury(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, treasuryJSON(t))
}

// economyJSON is the bonus and the fees an administrator sets.
type economyJSON struct {
	Bonus           int64 `json:"bonus"`
	TransferFee     int   `json:"transferFee"`
	StickerTax      int   `json:"stickerTax"`
	PrimeStickerTax int   `json:"primeStickerTax"`
}

// setEconomy is "Экономика" of the admin panel: the daily bonus and the
// fees of transfers and sticker packs.
func (a *api) setEconomy(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req economyJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.votes.SetEconomy(r.Context(), token, model.Economy(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// bonusJSON is the daily bonus of the caller.
type bonusJSON struct {
	Votes    int64 `json:"votes"`
	Taken    bool  `json:"taken"`
	Eligible bool  `json:"eligible"`
	Streak   int   `json:"streak"`
}

func (a *api) bonus(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	b, err := a.votes.Bonus(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, bonusJSON(b))
}

// claimedBonusJSON is what the bonus gave and the votes the caller has
// now.
type claimedBonusJSON struct {
	Votes   int64 `json:"votes"`
	Balance int64 `json:"balance"`
}

// claimBonus is "Получить бонус": the votes of the day, once.
func (a *api) claimBonus(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	claimed, err := a.votes.ClaimBonus(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, claimedBonusJSON{Votes: claimed.Votes, Balance: claimed.Balance})
}

// raiseRating is "Повысить рейтинг": votes of the caller into the
// treasury, percent of rating in return.
func (a *api) raiseRating(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Percent int `json:"percent"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	raised, err := a.votes.RaiseRating(r.Context(), token, req.Percent)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Balance int64 `json:"balance"`
		Rating  int   `json:"rating"`
	}{raised.Balance, raised.Rating})
}

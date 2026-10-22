package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Votes is rest.Votes over the user service, which keeps the votes of
// people and the vouchers that give them.
type Votes struct {
	api userv1.UserServiceClient
}

func NewVotes(api userv1.UserServiceClient) *Votes {
	return &Votes{
		api: api,
	}
}

func (v *Votes) Transfer(ctx context.Context, accessToken, userID string, votes int64, note string) (model.Transferred, error) {
	resp, err := v.api.TransferVotes(withToken(ctx, accessToken), &userv1.TransferVotesRequest{UserId: userID, Votes: votes, Note: note})
	if err != nil {
		// Why the user service refuses a transfer is its to say, as with
		// gifts.
		return model.Transferred{}, errs.Wrap("v.api.TransferVotes", giftRefusal(err))
	}

	return model.Transferred{Votes: resp.GetVotes(), Fee: resp.GetFee()}, nil
}

func (v *Votes) RaiseRating(ctx context.Context, accessToken string, percent int) (model.RatingRaised, error) {
	if percent < 0 || percent > math.MaxInt32 {
		return model.RatingRaised{}, failure.New(failure.InvalidArgument, "the rating is raised by 1 to 1000 percent")
	}

	resp, err := v.api.RaiseRating(withToken(ctx, accessToken), &userv1.RaiseRatingRequest{Percent: int32(percent)})
	if err != nil {
		return model.RatingRaised{}, errs.Wrap("v.api.RaiseRating", fromStatus(err))
	}

	return model.RatingRaised{Balance: resp.GetBalance(), Rating: int(resp.GetRating())}, nil
}

func (v *Votes) Statement(ctx context.Context, accessToken, cursor, flow string, limit int) (model.Statement, error) {
	resp, err := v.api.ListVoteOperations(withToken(ctx, accessToken), &userv1.ListVoteOperationsRequest{
		Cursor: cursor,
		Limit:  toLimit(limit),
		Flow:   flow,
	})
	if err != nil {
		return model.Statement{}, errs.Wrap("v.api.ListVoteOperations", fromStatus(err))
	}

	s := model.Statement{
		Operations: make([]model.VoteOperation, 0, len(resp.GetOperations())),
		NextCursor: resp.GetNextCursor(),
		Income:     resp.GetIncome(),
		Expense:    resp.GetExpense(),
	}

	for _, op := range resp.GetOperations() {
		s.Operations = append(s.Operations, model.VoteOperation{
			ID:        op.GetId(),
			Kind:      op.GetKind(),
			Amount:    op.GetAmount(),
			Balance:   op.GetBalance(),
			PeerID:    op.GetPeerId(),
			Note:      op.GetNote(),
			CreatedAt: op.GetCreatedAt().AsTime(),
		})
	}

	return s, nil
}

func (v *Votes) Redeem(ctx context.Context, accessToken, token string) (model.Redeemed, error) {
	resp, err := v.api.RedeemVoucher(withToken(ctx, accessToken), &userv1.RedeemVoucherRequest{Token: token})
	if err != nil {
		return model.Redeemed{}, errs.Wrap("v.api.RedeemVoucher", fromStatus(err))
	}

	return model.Redeemed{Votes: resp.GetVotes(), Rating: int(resp.GetRating()), Balance: resp.GetBalance()}, nil
}

func (v *Votes) CreateVoucher(ctx context.Context, accessToken string, d model.VoucherDraft) (model.Voucher, error) {
	resp, err := v.api.CreateVoucher(withToken(ctx, accessToken), &userv1.CreateVoucherRequest{
		Token:  d.Token,
		Votes:  d.Votes,
		Rating: clampInt32(d.Rating),
		Usages: clampInt32(d.Usages),
	})
	if err != nil {
		return model.Voucher{}, errs.Wrap("v.api.CreateVoucher", fromStatus(err))
	}

	return toVoucher(resp.GetVoucher()), nil
}

func (v *Votes) ListVouchers(ctx context.Context, accessToken, cursor string, limit int) (model.VoucherPage, error) {
	resp, err := v.api.ListVouchers(withToken(ctx, accessToken), &userv1.ListVouchersRequest{Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.VoucherPage{}, errs.Wrap("v.api.ListVouchers", fromStatus(err))
	}

	page := model.VoucherPage{
		Vouchers:   make([]model.Voucher, 0, len(resp.GetVouchers())),
		NextCursor: resp.GetNextCursor(),
	}

	for _, pv := range resp.GetVouchers() {
		page.Vouchers = append(page.Vouchers, toVoucher(pv))
	}

	return page, nil
}

func (v *Votes) DeleteVoucher(ctx context.Context, accessToken, voucherID string) error {
	if _, err := v.api.DeleteVoucher(withToken(ctx, accessToken), &userv1.DeleteVoucherRequest{VoucherId: voucherID}); err != nil {
		return errs.Wrap("v.api.DeleteVoucher", fromStatus(err))
	}

	return nil
}

func (v *Votes) Rate(ctx context.Context) (int64, error) {
	resp, err := v.api.GetVoteRate(ctx, &userv1.GetVoteRateRequest{})
	if err != nil {
		return 0, errs.Wrap("v.api.GetVoteRate", fromStatus(err))
	}

	return resp.GetKopecks(), nil
}

func (v *Votes) SetRate(ctx context.Context, accessToken string, kopecks int64) error {
	if _, err := v.api.SetVoteRate(withToken(ctx, accessToken), &userv1.SetVoteRateRequest{Kopecks: kopecks}); err != nil {
		return errs.Wrap("v.api.SetVoteRate", fromStatus(err))
	}

	return nil
}

func (v *Votes) Treasury(ctx context.Context) (model.Treasury, error) {
	resp, err := v.api.GetTreasury(ctx, &userv1.GetTreasuryRequest{})
	if err != nil {
		return model.Treasury{}, errs.Wrap("v.api.GetTreasury", fromStatus(err))
	}

	return model.Treasury{
		Votes:           resp.GetVotes(),
		Outstanding:     resp.GetOutstanding(),
		Bonus:           resp.GetBonus(),
		Held:            resp.GetHeld(),
		TransferFee:     int(resp.GetTransferFee()),
		StickerTax:      int(resp.GetStickerTax()),
		PrimeStickerTax: int(resp.GetPrimeStickerTax()),
	}, nil
}

func (v *Votes) SetEconomy(ctx context.Context, accessToken string, e model.Economy) error {
	req := &userv1.SetEconomyRequest{Bonus: e.Bonus}

	fees := []struct {
		n    int
		what string
		to   *int32
	}{
		{e.TransferFee, "the fee of a transfer", &req.TransferFee},
		{e.StickerTax, "the tax of a sticker pack", &req.StickerTax},
		{e.PrimeStickerTax, "the tax of a sticker pack with prime", &req.PrimeStickerTax},
	}
	for _, f := range fees {
		n, err := int32Of(f.n, f.what)
		if err != nil {
			return err
		}

		*f.to = n
	}

	if _, err := v.api.SetEconomy(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("v.api.SetEconomy", fromStatus(err))
	}

	return nil
}

func (v *Votes) Bonus(ctx context.Context, accessToken string) (model.Bonus, error) {
	resp, err := v.api.GetBonus(withToken(ctx, accessToken), &userv1.GetBonusRequest{})
	if err != nil {
		return model.Bonus{}, errs.Wrap("v.api.GetBonus", fromStatus(err))
	}

	return model.Bonus{Votes: resp.GetVotes(), Taken: resp.GetTaken(), Eligible: resp.GetEligible()}, nil
}

func (v *Votes) ClaimBonus(ctx context.Context, accessToken string) (model.Redeemed, error) {
	resp, err := v.api.ClaimBonus(withToken(ctx, accessToken), &userv1.ClaimBonusRequest{})
	if err != nil {
		return model.Redeemed{}, errs.Wrap("v.api.ClaimBonus", fromStatus(err))
	}

	return model.Redeemed{Votes: resp.GetVotes(), Balance: resp.GetBalance()}, nil
}

func toVoucher(pv *userv1.Voucher) model.Voucher {
	return model.Voucher{
		ID:          pv.GetId(),
		Token:       pv.GetToken(),
		Votes:       pv.GetVotes(),
		Rating:      int(pv.GetRating()),
		UsagesLeft:  int(pv.GetUsagesLeft()),
		Activations: int(pv.GetActivations()),
	}
}

// clampInt32 keeps n within int32: what is out of range stays out of the
// range the user service accepts.
func clampInt32(n int) int32 {
	return int32(min(max(n, math.MinInt32), math.MaxInt32)) //nolint:gosec // clamped
}

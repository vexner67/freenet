package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PurchaseRepository keeps the paid posts people opened and the messages
// paid for and not yet sent.
type PurchaseRepository interface {
	// Bought are those of postIDs buyerID has paid for.
	Bought(ctx context.Context, buyerID uuid.UUID, postIDs []uuid.UUID) ([]uuid.UUID, error)
	// AddPurchase writes postID down as bought by buyerID; false when it
	// was already.
	AddPurchase(ctx context.Context, buyerID, postID, authorID uuid.UUID, price int64) (bool, error)
	SavePayment(ctx context.Context, p user.MessagePayment) error
	// TakePayment removes the payment id of senderID and returns it;
	// user.ErrPaymentNotFound.
	TakePayment(ctx context.Context, id, senderID uuid.UUID) (user.MessagePayment, error)
}

// MaxBoughtBatch is the most posts asked of at once.
const MaxBoughtBatch = 100

var ErrTooManyPosts = errors.New("too many posts")

// MessageTerms are on what terms a sender may write to a recipient.
type MessageTerms struct {
	Allowed bool
	// PrimeOnly: the recipient refuses the sender only for having no
	// Freenet Prime.
	PrimeOnly bool
	// Price is what each message costs, 0 for free.
	Price int64
}

// PaidService sells what people put a price on: the photos and documents
// of their posts and messages to them. The votes go to the seller, who
// pays the treasury the tax of sticker packs (User.StickerTaxAt).
type PaidService struct {
	users       UserRepository
	wallets     WalletRepository
	purchases   PurchaseRepository
	economy     EconomyRepository
	permissions *PermissionService
	tx          TxManager
	now         func() time.Time
}

func NewPaidService(
	users UserRepository, wallets WalletRepository, purchases PurchaseRepository, economy EconomyRepository,
	permissions *PermissionService, tx TxManager,
) *PaidService {
	return &PaidService{
		users: users, wallets: wallets, purchases: purchases, economy: economy,
		permissions: permissions, tx: tx, now: time.Now,
	}
}

// Bought are those of postIDs buyerID has opened; at most MaxBoughtBatch
// (ErrTooManyPosts).
func (s *PaidService) Bought(ctx context.Context, buyerID uuid.UUID, postIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(postIDs) > MaxBoughtBatch {
		return nil, ErrTooManyPosts
	}

	if len(postIDs) == 0 {
		return nil, nil
	}

	bought, err := s.purchases.Bought(ctx, buyerID, postIDs)

	return bought, errs.Wrap("s.purchases.Bought", err)
}

// BuyPost opens the photos and documents of postID, written by authorID
// for price, to buyerID and returns their votes left. The post service
// knows the post: the caller tells its author and price. Opened before, it
// costs nothing. user.ErrNotEnoughVotes, user.ErrDeactivated for an author
// whose page is gone, user.ErrInvalidPostPrice for a free post or one's
// own.
func (s *PaidService) BuyPost(ctx context.Context, buyerID, postID, authorID uuid.UUID, price int64) (int64, error) {
	if price <= 0 || buyerID == authorID {
		return 0, user.ErrInvalidPostPrice
	}

	author, err := s.users.GetByID(ctx, authorID)
	if err != nil {
		return 0, errs.Wrap("s.users.GetByID", err)
	}

	if author.IsDeactivated() || author.IsBannedAt(s.now()) {
		return 0, user.ErrDeactivated
	}

	var left int64

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		bought, err := s.purchases.Bought(ctx, buyerID, []uuid.UUID{postID})
		if err != nil {
			return errs.Wrap("s.purchases.Bought", err)
		}

		if len(bought) > 0 {
			return errBoughtMeanwhile
		}

		// The votes go before the post is written down: refused, they
		// leave nothing bought even where no transaction rolls back.
		if left, err = s.sell(ctx, buyerID, author, price, user.Entry(user.VotePost, uuid.Nil, postID.String()), user.TaxNotePost); err != nil {
			return errs.Wrap("s.sell", err)
		}

		added, err := s.purchases.AddPurchase(ctx, buyerID, postID, authorID, price)
		if err != nil {
			return errs.Wrap("s.purchases.AddPurchase", err)
		}

		if !added {
			return errBoughtMeanwhile
		}

		return nil
	})
	if errors.Is(err, errBoughtMeanwhile) {
		return s.balance(ctx, buyerID)
	}
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return left, nil
}

// sell hands price of buyerID to the seller, who pays the tax of it into
// the treasury, and returns what the buyer has left. In a transaction.
func (s *PaidService) sell(ctx context.Context, buyerID uuid.UUID, seller *user.User, price int64, e user.VoteEntry, taxNote string) (int64, error) {
	left, err := s.wallets.Move(ctx, buyerID, seller.ID(), price, e)
	if err != nil {
		return 0, errs.Wrap("s.wallets.Move", err)
	}

	tax, err := s.taxOf(ctx, seller, price)
	if err != nil {
		return 0, errs.Wrap("s.taxOf", err)
	}

	// The seller pays the tax out of the price just received: it cannot
	// be short.
	if tax > 0 {
		if _, err = s.wallets.Spend(ctx, seller.ID(), tax, user.Entry(user.VoteTax, buyerID, taxNote)); err != nil {
			return 0, errs.Wrap("s.wallets.Spend", err)
		}
	}

	return left, nil
}

func (s *PaidService) taxOf(ctx context.Context, seller *user.User, price int64) (int64, error) {
	economy, err := s.economy.Economy(ctx)
	if err != nil {
		return 0, errs.Wrap("s.economy.Economy", err)
	}

	return seller.StickerTaxAt(economy, price, s.now()), nil
}

func (s *PaidService) balance(ctx context.Context, userID uuid.UUID) (int64, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return 0, errs.Wrap("s.users.GetByID", err)
	}

	return u.Wallet().Votes, nil
}

// Terms tells on what terms senderID may write to recipientID; wroteFirst
// is whether the recipient has written to the sender, which the message
// service knows. user.ErrNotFound for an unknown recipient.
func (s *PaidService) Terms(ctx context.Context, senderID, recipientID uuid.UUID, wroteFirst bool) (MessageTerms, error) {
	recipient, ties, err := s.permissions.messageTies(ctx, senderID, recipientID, wroteFirst)
	if err != nil {
		return MessageTerms{}, errs.Wrap("s.permissions.messageTies", err)
	}

	if !recipient.CanBeMessagedBy(senderID, ties) {
		return MessageTerms{PrimeOnly: recipient.WantsPrimeFrom(senderID, ties)}, nil
	}

	return MessageTerms{Allowed: true, Price: recipient.MessagePriceFor(senderID, ties)}, nil
}

// PayMessage pays for a message of senderID to recipientID at price, what
// the sender agreed to, and returns the payment and the votes left; the
// payment is zero for a message that costs nothing. The caller sends the
// message then and, when it is not sent, gives the votes back with
// RefundMessage. user.ErrCannotMessage, user.ErrMessagePriceChanged,
// user.ErrNotEnoughVotes.
func (s *PaidService) PayMessage(
	ctx context.Context, senderID, recipientID uuid.UUID, wroteFirst bool, price int64,
) (user.MessagePayment, int64, error) {
	recipient, ties, err := s.permissions.messageTies(ctx, senderID, recipientID, wroteFirst)
	if err != nil {
		return user.MessagePayment{}, 0, errs.Wrap("s.permissions.messageTies", err)
	}

	if !recipient.CanBeMessagedBy(senderID, ties) {
		return user.MessagePayment{}, 0, user.ErrCannotMessage
	}

	if want := recipient.MessagePriceFor(senderID, ties); want != price {
		return user.MessagePayment{}, 0, user.ErrMessagePriceChanged
	}

	if price == 0 {
		return user.MessagePayment{}, 0, nil
	}

	id, err := uuid.NewV7()
	if err != nil {
		return user.MessagePayment{}, 0, errs.Wrap("uuid.NewV7", err)
	}

	tax, err := s.taxOf(ctx, recipient, price)
	if err != nil {
		return user.MessagePayment{}, 0, errs.Wrap("s.taxOf", err)
	}

	payment := user.MessagePayment{ID: id, SenderID: senderID, RecipientID: recipientID, Price: price, Tax: tax}

	var left int64

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if left, err = s.sell(ctx, senderID, recipient, price, user.Entry(user.VoteMessage, uuid.Nil, ""), user.TaxNoteMessage); err != nil {
			return errs.Wrap("s.sell", err)
		}

		return errs.Wrap("s.purchases.SavePayment", s.purchases.SavePayment(ctx, payment))
	})
	if err != nil {
		return user.MessagePayment{}, 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return payment, left, nil
}

// RefundMessage gives senderID back the votes of a message that was not
// sent: the recipient returns what they got, the treasury the tax.
// user.ErrPaymentNotFound, user.ErrNotEnoughVotes when the recipient has
// spent them already.
func (s *PaidService) RefundMessage(ctx context.Context, senderID, paymentID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.purchases.TakePayment(ctx, paymentID, senderID)
		if err != nil {
			return errs.Wrap("s.purchases.TakePayment", err)
		}

		refund := user.Entry(user.VoteMessage, uuid.Nil, user.RefundNote)

		if _, err = s.wallets.Move(ctx, p.RecipientID, senderID, p.Price-p.Tax, refund); err != nil {
			return errs.Wrap("s.wallets.Move", err)
		}

		if p.Tax > 0 {
			if _, err = s.wallets.Credit(ctx, senderID, p.Tax, 0, user.Entry(user.VoteMessage, p.RecipientID, user.RefundNote)); err != nil {
				return errs.Wrap("s.wallets.Credit", err)
			}
		}

		return nil
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

package user

import (
	"errors"

	"github.com/google/uuid"
)

// The notes of the tax on what people sell besides sticker packs: the
// photos of a post and messages.
const (
	TaxNotePost    = "#post"
	TaxNoteMessage = "#message"
	// RefundNote marks the votes of a message given back.
	RefundNote = "refund"
)

var (
	ErrInvalidPostPrice = errors.New("the post is not paid")
	// ErrCannotMessage: the recipient does not let the sender write.
	ErrCannotMessage = errors.New("you cannot write to this user")
	// ErrMessagePriceChanged: the sender agreed to another price than
	// messages to the recipient cost now.
	ErrMessagePriceChanged = errors.New("the price of messages changed")
	ErrPaymentNotFound     = errors.New("payment not found")
)

// MessagePayment is what a sender paid for a message to a recipient, kept
// until the message is sent, so that the votes come back when it is not.
// Tax is what the recipient paid the treasury of Price.
type MessagePayment struct {
	ID          uuid.UUID
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	Price       int64
	Tax         int64
}

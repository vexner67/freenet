package user

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxVoteNoteLength is the most characters of the note of an operation:
// the message of a transfer, the question of an event.
const MaxVoteNoteLength = 200

// MaxTransferNoteLength is the most characters of the message a sender
// writes with a transfer.
const MaxTransferNoteLength = 140

var (
	ErrTransferNoteTooLong = errors.New("the message of a transfer is too long")
	ErrTransferNoteLines   = errors.New("the message of a transfer is one line")
)

// VoteKind is why the votes of a page changed: a line of its statement
// ("Выписка" of the bank of votes).
type VoteKind string

const (
	// VoteTransfer: votes handed to or by another person.
	VoteTransfer VoteKind = "transfer"
	VoteVoucher  VoteKind = "voucher"
	// VoteGift: a gift paid for; the peer is the recipient.
	VoteGift VoteKind = "gift"
	// VoteStickers: a sticker pack bought or, for its author, sold.
	VoteStickers VoteKind = "stickers"
	// VoteCasino: a spin of "Огонёк" and its prize.
	VoteCasino VoteKind = "casino"
	// VoteAim: a bet of "Прицел" and its win.
	VoteAim VoteKind = "aim"
	// VoteStake: a stake on an event and what the event paid back.
	VoteStake VoteKind = "stake"
	// VoteRocket: a bet of "Ракетка" and its win.
	VoteRocket VoteKind = "rocket"
	// VoteBonus: the daily bonus out of the treasury.
	VoteBonus VoteKind = "bonus"
	// VoteDenomination: the balance after the denomination of 1 to
	// 1 000 000, the first line of every statement kept.
	VoteDenomination VoteKind = "denomination"
	// VoteFee: the fee of a transfer, into the treasury; the peer is
	// the recipient.
	VoteFee VoteKind = "fee"
	// VoteTax: the tax on a sale of a sticker pack, paid by its author
	// into the treasury.
	VoteTax VoteKind = "tax"
	// VoteRating: rating bought; the note is how many percent.
	VoteRating VoteKind = "rating"
	// VotePrime: Freenet Prime bought; the note is its period, the peer
	// whom it is a present for.
	VotePrime VoteKind = "prime"
	// VoteGiftUpgrade: a gift made collectible; the note is the gift.
	VoteGiftUpgrade VoteKind = "gift_upgrade"
	// VoteGiftTransfer: a collectible given away; the peer is whom to.
	VoteGiftTransfer VoteKind = "gift_transfer"
	// VoteGiftSale: a collectible bought or sold; the peer is the other
	// side, none for the site.
	VoteGiftSale VoteKind = "gift_sale"
	// VoteGiftFee: what the treasury took of a sale; the peer is the
	// buyer.
	VoteGiftFee VoteKind = "gift_fee"
	// VoteGiftBid: votes held for a bid at an auction.
	VoteGiftBid VoteKind = "gift_bid"
	// VoteGiftRefund: the votes of a bid given back, when someone bid
	// more or the auction was called off.
	VoteGiftRefund VoteKind = "gift_refund"
	// VoteMine: ore sold in the mine; the note is the ore.
	VoteMine VoteKind = "mine"
	// VoteMineChest: a chest found in the mine; the note is the ore.
	VoteMineChest VoteKind = "mine_chest"
	// VotePost: the photos of a paid post opened or, for its author,
	// sold; the note is the post.
	VotePost VoteKind = "post"
	// VoteMessage: a paid message, or its votes given back when it was
	// not sent (the note is "refund").
	VoteMessage VoteKind = "message"
	// VoteBusiness: votes spent on a business or taken out of it; the
	// note is the business and the action, "kiosk:collect".
	VoteBusiness VoteKind = "business"
)

// VoteEntry is what a change of votes is written down as: why, with whom
// (uuid.Nil: nobody) and a note.
type VoteEntry struct {
	Kind   VoteKind
	PeerID uuid.UUID
	Note   string
}

// Entry is a VoteEntry of kind with peerID and note, the note cut to
// what a statement keeps.
func Entry(kind VoteKind, peerID uuid.UUID, note string) VoteEntry {
	if utf8.RuneCountInString(note) > MaxVoteNoteLength {
		note = string([]rune(note)[:MaxVoteNoteLength])
	}

	return VoteEntry{Kind: kind, PeerID: peerID, Note: note}
}

// VoteOperation is a line of the statement of a page: Amount is what came
// in (above zero) or went out (below), Balance — the votes after it.
type VoteOperation struct {
	ID      uuid.UUID
	Kind    VoteKind
	Amount  int64
	Balance int64
	PeerID  uuid.UUID
	Note    string
	At      time.Time
}

// VoteFlow picks the operations of a statement: all of them, what came in
// or what went out.
type VoteFlow string

const (
	VoteFlowAll VoteFlow = ""
	VoteFlowIn  VoteFlow = "in"
	VoteFlowOut VoteFlow = "out"
)

var ErrVoteFlowInvalid = errors.New("unknown flow of votes")

func NewVoteFlow(s string) (VoteFlow, error) {
	switch f := VoteFlow(s); f {
	case VoteFlowAll, VoteFlowIn, VoteFlowOut:
		return f, nil
	default:
		return "", ErrVoteFlowInvalid
	}
}

// Matches tells whether an operation of amount is of the flow.
func (f VoteFlow) Matches(amount int64) bool {
	switch f {
	case VoteFlowIn:
		return amount > 0
	case VoteFlowOut:
		return amount < 0
	default:
		return true
	}
}

// TransferNote is the message a sender writes with a transfer ("За
// пиццу"): one line, may be empty.
type TransferNote string

func NewTransferNote(s string) (TransferNote, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if strings.ContainsAny(s, "\r\n") {
		return "", ErrTransferNoteLines
	}

	if utf8.RuneCountInString(s) > MaxTransferNoteLength {
		return "", ErrTransferNoteTooLong
	}

	return TransferNote(s), nil
}

func (n TransferNote) String() string {
	return string(n)
}

// OperationsSince is the ID below every operation made at t or later and
// above every one made earlier: IDs are UUIDv7, which begin with the time.
func OperationsSince(t time.Time) uuid.UUID {
	var id uuid.UUID

	ms := max(t.UnixMilli(), 0)
	for i := 5; i >= 0; i-- {
		id[i] = byte(ms)
		ms >>= 8
	}

	return id
}

package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewVotes(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{1, 50, user.MaxVotes} {
		if v, err := user.NewVotes(n); err != nil || v.Int64() != n {
			t.Fatalf("NewVotes(%d) = %d, %v", n, v, err)
		}
	}

	for _, n := range []int64{0, -1, user.MaxVotes + 1} {
		if _, err := user.NewVotes(n); !errors.Is(err, user.ErrVotesInvalid) {
			t.Fatalf("NewVotes(%d): %v", n, err)
		}
	}
}

func TestUser_CheckTransferFrom(t *testing.T) {
	t.Parallel()

	ann, _ := user.New("ann@example.com")
	bob := uuid.New()

	if err := ann.CheckTransferFrom(bob, user.Ties{}); err != nil {
		t.Fatalf("a stranger: %v", err)
	}
	if err := ann.CheckTransferFrom(ann.ID(), user.Ties{}); !errors.Is(err, user.ErrTransferSelf) {
		t.Fatalf("oneself: %v", err)
	}
	if err := ann.CheckTransferFrom(bob, user.Ties{Blocked: true}); !errors.Is(err, user.ErrTransferRefused) {
		t.Fatalf("across a blacklist: %v", err)
	}

	if err := ann.Deactivate("", time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := ann.CheckTransferFrom(bob, user.Ties{}); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("a deleted page: %v", err)
	}
	if admins, _ := user.NewAdmins([]string{"ann@example.com"}); admins.Has(ann) {
		t.Fatal("a deleted page runs the site")
	}
}

func TestUser_Wallet(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	if u.Wallet() != (user.Wallet{}) {
		t.Fatalf("a new page has %+v", u.Wallet())
	}

	rich := u.WithWallet(user.Wallet{Votes: 5, Rating: 30})
	if rich.Wallet() != (user.Wallet{Votes: 5, Rating: 30}) || u.Wallet() != (user.Wallet{}) {
		t.Fatalf("WithWallet: %+v, the original %+v", rich.Wallet(), u.Wallet())
	}

	// The rating of vouchers is added to the fullness and may pass 100.
	if p, _ := rich.Completeness(); p != 40 {
		t.Fatalf("an empty page with 30 of rating: %d", p)
	}
	if p, _ := u.WithWallet(user.Wallet{Rating: 200}).Completeness(); p != 210 {
		t.Fatalf("with 200 of rating: %d", p)
	}
}

func TestNewVoucherToken(t *testing.T) {
	t.Parallel()

	const token = "ABCDEFGH12345678JKLMNPQR"

	for _, typed := range []string{
		token,
		strings.ToLower(token),
		"  ABCDEF-GH1234-5678JK-LMNPQR ",
		"ABCDEF GH1234 5678JK LMNPQR",
	} {
		if got, err := user.NewVoucherToken(typed); err != nil || got.String() != token {
			t.Fatalf("NewVoucherToken(%q) = %q, %v", typed, got, err)
		}
	}

	for _, typed := range []string{"", "ABC", token + "A", "ABCDEFGH12345678JKLMNPQ_", "АБВГДЕЖЗ12345678ИКЛМНОПР"} {
		if _, err := user.NewVoucherToken(typed); !errors.Is(err, user.ErrVoucherTokenInvalid) {
			t.Fatalf("NewVoucherToken(%q): %v", typed, err)
		}
	}

	a, err := user.GenerateVoucherToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := user.GenerateVoucherToken()
	if a == b {
		t.Fatal("two generated serial numbers are the same")
	}
	if again, err := user.NewVoucherToken(a.String()); err != nil || again != a {
		t.Fatalf("a generated serial number %q is not accepted: %v", a, err)
	}
}

func TestNewVoucher(t *testing.T) {
	t.Parallel()

	const token = user.VoucherToken("ABCDEFGH12345678JKLMNPQR")

	v, err := user.NewVoucher(token, 10, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID() == uuid.Nil || v.Token() != token || v.Votes() != 10 || v.Rating() != 5 || v.UsagesLeft() != 3 || v.Activations() != 0 || v.Expired() {
		t.Fatalf("voucher = %+v", v)
	}

	if v, err = user.NewVoucher(token, 0, 1, user.UnlimitedUsages); err != nil || v.Expired() {
		t.Fatalf("rating only, for everyone: %+v, %v", v, err)
	}

	if !user.RestoreVoucher(user.StoredVoucher{ID: uuid.New(), Token: token, Votes: 1}).Expired() {
		t.Fatal("a voucher with no usages left has not expired")
	}

	tests := []struct {
		name          string
		votes         int64
		rating, usage int
		want          error
	}{
		{"nothing", 0, 0, 1, user.ErrVoucherEmpty},
		{"negative votes", -1, 0, 1, user.ErrVoucherAmountInvalid},
		{"too many votes", user.MaxVotes + 1, 0, 1, user.ErrVoucherAmountInvalid},
		{"negative rating", 1, -1, 1, user.ErrVoucherAmountInvalid},
		{"too much rating", 1, user.MaxVoucherRating + 1, 1, user.ErrVoucherAmountInvalid},
		{"no usages", 1, 0, 0, user.ErrVoucherUsagesInvalid},
		{"negative usages", 1, 0, -2, user.ErrVoucherUsagesInvalid},
		{"too many usages", 1, 0, user.MaxVoucherUsages + 1, user.ErrVoucherUsagesInvalid},
	}
	for _, tt := range tests {
		if _, err := user.NewVoucher(token, tt.votes, tt.rating, tt.usage); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestAdmins(t *testing.T) {
	t.Parallel()

	admins, err := user.NewAdmins([]string{" Ann@Example.com ", "", "bob@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	ann, _ := user.New("ann@example.com")
	cat, _ := user.New("cat@example.com")
	if !admins.Has(ann) || admins.Has(cat) {
		t.Fatalf("ann: %v, cat: %v", admins.Has(ann), admins.Has(cat))
	}

	if none, err := user.NewAdmins(nil); err != nil || none.Has(ann) {
		t.Fatalf("no admins: %v, %v", none, err)
	}
	if _, err = user.NewAdmins([]string{"not an email"}); err == nil {
		t.Fatal("a wrong email is accepted")
	}
}

func TestGiftPrices(t *testing.T) {
	t.Parallel()

	free, paid := 0, 0
	for _, c := range user.GiftCatalog() {
		if c.Price < 0 {
			t.Fatalf("%s costs %d", c.ID, c.Price)
		}
		if c.Price == 0 {
			free++
		} else {
			paid++
		}
		for _, g := range c.Gifts {
			if g.DefaultPrice() != c.Price {
				t.Fatalf("%s costs %d in a collection of %d", g, g.DefaultPrice(), c.Price)
			}
		}
	}
	if free == 0 || paid == 0 {
		t.Fatalf("%d free collections and %d paid", free, paid)
	}
	if user.GiftKind("nothing").DefaultPrice() != 0 {
		t.Fatal("an unknown gift has a price")
	}
}

func TestNewGiftPrice(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{0, 1, user.MaxGiftPrice} {
		if p, err := user.NewGiftPrice(n); err != nil || p.Int64() != n {
			t.Errorf("NewGiftPrice(%d) = %d, %v", n, p, err)
		}
	}
	for _, n := range []int64{-1, user.MaxGiftPrice + 1} {
		if _, err := user.NewGiftPrice(n); !errors.Is(err, user.ErrGiftPriceInvalid) {
			t.Errorf("NewGiftPrice(%d): %v", n, err)
		}
	}
}

func TestGiftPrices_Of(t *testing.T) {
	t.Parallel()

	prices := user.GiftPrices{"cake": 7, "rose": 2}
	if got := prices.Of("cake"); got != 7 {
		t.Errorf("a priced gift costs %d", got)
	}
	if got := prices.Of("rose"); got != 2 {
		t.Errorf("a free gift priced by the administrators costs %d", got)
	}
	if got, want := prices.Of("present"), user.GiftKind("present").DefaultPrice(); got != want {
		t.Errorf("an unpriced gift costs %d, its collection %d", got, want)
	}
	if got, want := user.GiftPrices(nil).Of("candy"), user.GiftKind("candy").DefaultPrice(); got != want {
		t.Errorf("with no prices a gift costs %d, its collection %d", got, want)
	}
}

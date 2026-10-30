package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns user errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

// invalidStickerPack are what a pack of a person may not be.
var invalidStickerPack = []error{
	user.ErrStickerPackNameEmpty,
	user.ErrStickerPackNameTooLong,
	user.ErrStickerPackAboutTooLong,
	user.ErrStickerPackPrice,
	user.ErrStickerPackEmpty,
	user.ErrTooManyPackStickers,
}

// giftMarket are the errors of collectible gifts, their sales and
// auctions, with their codes.
var giftMarket = []struct {
	err  error
	code codes.Code
}{
	{user.ErrGiftCollectible, codes.FailedPrecondition},
	{user.ErrNotCollectible, codes.FailedPrecondition},
	{user.ErrNotGiftOwner, codes.PermissionDenied},
	{user.ErrGiftOnSale, codes.FailedPrecondition},
	{user.ErrGiftNotOnSale, codes.FailedPrecondition},
	{user.ErrGiftPriceChanged, codes.FailedPrecondition},
	{user.ErrGiftSalePrice, codes.InvalidArgument},
	{user.ErrOwnGift, codes.FailedPrecondition},
	{user.ErrGiftSoldOut, codes.FailedPrecondition},
	{user.ErrGiftSupplyInvalid, codes.InvalidArgument},
	{user.ErrUnknownTrait, codes.InvalidArgument},
	{user.ErrMarketSort, codes.InvalidArgument},
	{user.ErrAuctionHours, codes.InvalidArgument},
	{user.ErrAuctionNotFound, codes.NotFound},
	{user.ErrAuctionEnded, codes.FailedPrecondition},
	{user.ErrBidTooLow, codes.FailedPrecondition},
	{user.ErrAuctionHasBids, codes.FailedPrecondition},
	{service.ErrCursor, codes.InvalidArgument},
}

// petErrors are the errors of pets and boards, with their codes.
var petErrors = []struct {
	err  error
	code codes.Code
}{
	{user.ErrTitleNotFound, codes.NotFound},
	{user.ErrUnknownPetKind, codes.InvalidArgument},
	{user.ErrUnknownPetCase, codes.InvalidArgument},
	{user.ErrUnknownPetItem, codes.InvalidArgument},
	{user.ErrUnknownPetStance, codes.InvalidArgument},
	{user.ErrUnknownPetMove, codes.InvalidArgument},
	{user.ErrNotInPetShop, codes.InvalidArgument},
	{user.ErrPetNameEmpty, codes.InvalidArgument},
	{user.ErrPetNameTooLong, codes.InvalidArgument},
	{user.ErrPetNameLines, codes.InvalidArgument},
	{user.ErrPetShopPrice, codes.InvalidArgument},
	{user.ErrPetShopFights, codes.InvalidArgument},
	{user.ErrUnknownTop, codes.InvalidArgument},
	{user.ErrPetNotFound, codes.NotFound},
	{user.ErrTooManyPets, codes.FailedPrecondition},
	{user.ErrPetLocked, codes.FailedPrecondition},
	{user.ErrPetClaimed, codes.FailedPrecondition},
	{user.ErrPetFull, codes.FailedPrecondition},
	{user.ErrPetItemOwned, codes.FailedPrecondition},
	{user.ErrPetItemMissing, codes.FailedPrecondition},
	{user.ErrPetTired, codes.FailedPrecondition},
	{user.ErrPetHungry, codes.FailedPrecondition},
	{user.ErrPetSad, codes.FailedPrecondition},
	{user.ErrPetFightsOver, codes.FailedPrecondition},
	{user.ErrPetFightSelf, codes.FailedPrecondition},
	{user.ErrNoPetOpponent, codes.FailedPrecondition},
	{user.ErrPetInBattle, codes.FailedPrecondition},
	{user.ErrNoPetBattle, codes.FailedPrecondition},
	{user.ErrPetBattleOver, codes.FailedPrecondition},
	{user.ErrPetDropWaiting, codes.FailedPrecondition},
	{user.ErrNoPetDrop, codes.FailedPrecondition},
	{user.ErrPetCasesOver, codes.FailedPrecondition},
	{user.ErrNotEnoughShards, codes.FailedPrecondition},
	{service.ErrPetArenaAction, codes.InvalidArgument},
	{user.ErrPetBet, codes.InvalidArgument},
	{user.ErrPetSalePrice, codes.InvalidArgument},
	{user.ErrPetBossDown, codes.FailedPrecondition},
	{user.ErrPetBossRest, codes.FailedPrecondition},
	{user.ErrPetTournamentClosed, codes.FailedPrecondition},
	{user.ErrPetTournamentFull, codes.FailedPrecondition},
	{user.ErrPetTournamentJoined, codes.FailedPrecondition},
	{user.ErrPetNotInTournament, codes.FailedPrecondition},
	{user.ErrPetBetPlaced, codes.FailedPrecondition},
	{user.ErrPetOnSale, codes.FailedPrecondition},
	{user.ErrPetNotOnSale, codes.FailedPrecondition},
	{user.ErrPetResale, codes.FailedPrecondition},
	{user.ErrTooManyPetSales, codes.FailedPrecondition},
	{user.ErrPetOwnSale, codes.FailedPrecondition},
	{user.ErrCanvasCell, codes.InvalidArgument},
	{user.ErrCanvasColor, codes.InvalidArgument},
	{user.ErrCanvasCooldown, codes.FailedPrecondition},
	{user.ErrDatingGoal, codes.InvalidArgument},
	{user.ErrDatingSeek, codes.InvalidArgument},
	{user.ErrDatingAbout, codes.InvalidArgument},
	{user.ErrDatingSelf, codes.InvalidArgument},
	{user.ErrDatingOff, codes.FailedPrecondition},
	{user.ErrDatingTwice, codes.FailedPrecondition},
	{user.ErrDatingProfile, codes.FailedPrecondition},
}

// bankInvalid are the amounts and settings the bank does not take.
var bankInvalid = []error{user.ErrBankAmount, user.ErrBankRate, user.ErrBankMost, user.ErrBankDays}

// bankRefused are what the account of a page does not allow.
var bankRefused = []error{
	user.ErrDepositMost, user.ErrDepositShort, user.ErrLoanMost, user.ErrLoanActive, user.ErrNoLoan, user.ErrBankNotYet,
}

func toStatus(err error) (*status.Status, bool) {
	for _, invalid := range invalidStickerPack {
		if errors.Is(err, invalid) {
			return status.New(codes.InvalidArgument, invalid.Error()), true
		}
	}

	for _, known := range giftMarket {
		if errors.Is(err, known.err) {
			return status.New(known.code, known.err.Error()), true
		}
	}

	for _, known := range petErrors {
		if errors.Is(err, known.err) {
			return status.New(known.code, known.err.Error()), true
		}
	}

	switch {
	case errors.Is(err, user.ErrNotFound):
		return status.New(codes.NotFound, "user not found"), true
	case errors.Is(err, user.ErrAlreadyExists):
		return status.New(codes.AlreadyExists, "user already exists"), true
	case errors.Is(err, user.ErrNicknameTaken):
		return status.New(codes.AlreadyExists, "nickname is already taken"), true
	case errors.Is(err, user.ErrNameEmpty):
		// An address for a page that is not filled in yet.
		return status.New(codes.InvalidArgument, user.ErrNameEmpty.Error()), true
	case errors.Is(err, user.ErrFollowSelf):
		return status.New(codes.InvalidArgument, "you cannot follow yourself"), true
	case errors.Is(err, user.ErrFollowBlocked):
		return status.New(codes.PermissionDenied, "you cannot follow this user"), true
	case errors.Is(err, user.ErrInviteExpired):
		return status.New(codes.FailedPrecondition, user.ErrInviteExpired.Error()), true
	case errors.Is(err, user.ErrAlreadyInvited):
		return status.New(codes.FailedPrecondition, user.ErrAlreadyInvited.Error()), true
	case errors.Is(err, user.ErrBlockSelf):
		return status.New(codes.InvalidArgument, "you cannot blacklist yourself"), true
	case errors.Is(err, user.ErrIgnoreSelf):
		return status.New(codes.InvalidArgument, user.ErrIgnoreSelf.Error()), true
	case errors.Is(err, user.ErrTooManyIgnored):
		return status.New(codes.FailedPrecondition, user.ErrTooManyIgnored.Error()), true
	case errors.Is(err, user.ErrUnknownGift):
		return status.New(codes.InvalidArgument, user.ErrUnknownGift.Error()), true
	case errors.Is(err, user.ErrGiftNotFound):
		return status.New(codes.NotFound, user.ErrGiftNotFound.Error()), true
	case errors.Is(err, user.ErrGiftRefused):
		return status.New(codes.PermissionDenied, user.ErrGiftRefused.Error()), true
	case errors.Is(err, user.ErrGiftsHidden):
		return status.New(codes.PermissionDenied, user.ErrGiftsHidden.Error()), true
	case errors.Is(err, user.ErrCannotDeleteGift):
		return status.New(codes.PermissionDenied, user.ErrCannotDeleteGift.Error()), true
	case errors.Is(err, user.ErrUnknownStickerPack):
		return status.New(codes.InvalidArgument, user.ErrUnknownStickerPack.Error()), true
	case errors.Is(err, user.ErrTooManyStickerPacks):
		return status.New(codes.FailedPrecondition, user.ErrTooManyStickerPacks.Error()), true
	case errors.Is(err, user.ErrStickerPackTaken):
		return status.New(codes.FailedPrecondition, user.ErrStickerPackTaken.Error()), true
	case errors.Is(err, user.ErrCannotChangeStickerPack):
		return status.New(codes.PermissionDenied, user.ErrCannotChangeStickerPack.Error()), true
	case errors.Is(err, user.ErrBetAmount):
		return status.New(codes.InvalidArgument, user.ErrBetAmount.Error()), true
	case errors.Is(err, user.ErrBetMultiplier):
		return status.New(codes.InvalidArgument, user.ErrBetMultiplier.Error()), true
	case errors.Is(err, user.ErrEventOdds):
		return status.New(codes.InvalidArgument, user.ErrEventOdds.Error()), true
	case errors.Is(err, user.ErrEventOddsCount):
		return status.New(codes.InvalidArgument, user.ErrEventOddsCount.Error()), true
	case errors.Is(err, user.ErrAimMultipliers):
		return status.New(codes.InvalidArgument, user.ErrAimMultipliers.Error()), true
	case errors.Is(err, user.ErrAimMultiplier):
		return status.New(codes.InvalidArgument, user.ErrAimMultiplier.Error()), true
	case errors.Is(err, user.ErrAimChance):
		return status.New(codes.InvalidArgument, user.ErrAimChance.Error()), true
	case errors.Is(err, user.ErrAimMultiplierTwice):
		return status.New(codes.InvalidArgument, user.ErrAimMultiplierTwice.Error()), true
	case errors.Is(err, user.ErrEventPooled):
		return status.New(codes.FailedPrecondition, user.ErrEventPooled.Error()), true
	case errors.Is(err, user.ErrBetQuestionEmpty):
		return status.New(codes.InvalidArgument, user.ErrBetQuestionEmpty.Error()), true
	case errors.Is(err, user.ErrBetQuestionTooLong):
		return status.New(codes.InvalidArgument, user.ErrBetQuestionTooLong.Error()), true
	case errors.Is(err, user.ErrBetOutcomeEmpty):
		return status.New(codes.InvalidArgument, user.ErrBetOutcomeEmpty.Error()), true
	case errors.Is(err, user.ErrBetOutcomeTooLong):
		return status.New(codes.InvalidArgument, user.ErrBetOutcomeTooLong.Error()), true
	case errors.Is(err, user.ErrBetOutcomes):
		return status.New(codes.InvalidArgument, user.ErrBetOutcomes.Error()), true
	case errors.Is(err, user.ErrBetOutcomeTwice):
		return status.New(codes.InvalidArgument, user.ErrBetOutcomeTwice.Error()), true
	case errors.Is(err, user.ErrBetCloseTime):
		return status.New(codes.InvalidArgument, user.ErrBetCloseTime.Error()), true
	case errors.Is(err, user.ErrBetOutcome):
		return status.New(codes.InvalidArgument, user.ErrBetOutcome.Error()), true
	case errors.Is(err, user.ErrStakeAmount):
		return status.New(codes.InvalidArgument, user.ErrStakeAmount.Error()), true
	case errors.Is(err, user.ErrBetEventNotFound):
		return status.New(codes.NotFound, user.ErrBetEventNotFound.Error()), true
	case errors.Is(err, user.ErrBetEventClosed):
		return status.New(codes.FailedPrecondition, user.ErrBetEventClosed.Error()), true
	case errors.Is(err, user.ErrBetEventFinished):
		return status.New(codes.FailedPrecondition, user.ErrBetEventFinished.Error()), true
	case errors.Is(err, user.ErrBetEventActive):
		return status.New(codes.FailedPrecondition, user.ErrBetEventActive.Error()), true
	case errors.Is(err, user.ErrPokerAnte):
		return status.New(codes.InvalidArgument, user.ErrPokerAnte.Error()), true
	case errors.Is(err, user.ErrPokerNotFound):
		return status.New(codes.NotFound, user.ErrPokerNotFound.Error()), true
	case errors.Is(err, user.ErrPokerOver):
		return status.New(codes.FailedPrecondition, user.ErrPokerOver.Error()), true
	case errors.Is(err, user.ErrPokerOpen):
		return status.New(codes.FailedPrecondition, user.ErrPokerOpen.Error()), true
	case errors.Is(err, user.ErrRocketAmount):
		return status.New(codes.InvalidArgument, user.ErrRocketAmount.Error()), true
	case errors.Is(err, user.ErrRocketAuto):
		return status.New(codes.InvalidArgument, user.ErrRocketAuto.Error()), true
	case errors.Is(err, user.ErrRocketNotFound):
		return status.New(codes.NotFound, user.ErrRocketNotFound.Error()), true
	case errors.Is(err, user.ErrOreLocked):
		return status.New(codes.FailedPrecondition, user.ErrOreLocked.Error()), true
	case errors.Is(err, user.ErrNoEnergy):
		return status.New(codes.FailedPrecondition, user.ErrNoEnergy.Error()), true
	case errors.Is(err, user.ErrNothingToSell):
		return status.New(codes.FailedPrecondition, user.ErrNothingToSell.Error()), true
	case errors.Is(err, user.ErrMineNotYet):
		return status.New(codes.FailedPrecondition, user.ErrMineNotYet.Error()), true
	case errors.Is(err, user.ErrUnknownOre):
		return status.New(codes.InvalidArgument, user.ErrUnknownOre.Error()), true
	case sentinel(err, firmInvalid) != nil:
		return status.New(codes.InvalidArgument, sentinel(err, firmInvalid).Error()), true
	case sentinel(err, firmMissing) != nil:
		return status.New(codes.NotFound, sentinel(err, firmMissing).Error()), true
	case sentinel(err, firmRefused) != nil:
		return status.New(codes.FailedPrecondition, sentinel(err, firmRefused).Error()), true
	case sentinel(err, holdemInvalid) != nil:
		return status.New(codes.InvalidArgument, sentinel(err, holdemInvalid).Error()), true
	case errors.Is(err, user.ErrTableNotFound):
		return status.New(codes.NotFound, user.ErrTableNotFound.Error()), true
	case sentinel(err, holdemRefused) != nil:
		return status.New(codes.FailedPrecondition, sentinel(err, holdemRefused).Error()), true
	case sentinel(err, bankInvalid) != nil:
		return status.New(codes.InvalidArgument, sentinel(err, bankInvalid).Error()), true
	case sentinel(err, bankRefused) != nil:
		return status.New(codes.FailedPrecondition, sentinel(err, bankRefused).Error()), true
	case errors.Is(err, user.ErrRocketOver):
		return status.New(codes.FailedPrecondition, user.ErrRocketOver.Error()), true
	case errors.Is(err, user.ErrInvalidPostPrice), errors.Is(err, user.ErrMessagePriceChanged):
		return status.New(codes.FailedPrecondition, err.Error()), true
	case errors.Is(err, user.ErrCannotMessage):
		return status.New(codes.PermissionDenied, user.ErrCannotMessage.Error()), true
	case errors.Is(err, user.ErrPaymentNotFound):
		return status.New(codes.NotFound, user.ErrPaymentNotFound.Error()), true
	case errors.Is(err, service.ErrTooManyPosts), errors.Is(err, user.ErrInvalidMessagePrice):
		return status.New(codes.InvalidArgument, err.Error()), true
	case errors.Is(err, user.ErrNotEnoughVotes):
		return status.New(codes.FailedPrecondition, user.ErrNotEnoughVotes.Error()), true
	case errors.Is(err, user.ErrPrimeOnly):
		return status.New(codes.FailedPrecondition, user.ErrPrimeOnly.Error()), true
	case errors.Is(err, user.ErrPrimePeriodInvalid):
		return status.New(codes.InvalidArgument, user.ErrPrimePeriodInvalid.Error()), true
	case errors.Is(err, user.ErrPrimePriceInvalid):
		return status.New(codes.InvalidArgument, user.ErrPrimePriceInvalid.Error()), true
	case errors.Is(err, user.ErrRatingLimit):
		return status.New(codes.FailedPrecondition, user.ErrRatingLimit.Error()), true
	case errors.Is(err, user.ErrBonusTaken):
		return status.New(codes.FailedPrecondition, user.ErrBonusTaken.Error()), true
	case errors.Is(err, user.ErrBonusNotYet):
		return status.New(codes.FailedPrecondition, user.ErrBonusNotYet.Error()), true
	case errors.Is(err, user.ErrEconomyBonus):
		return status.New(codes.InvalidArgument, user.ErrEconomyBonus.Error()), true
	case errors.Is(err, user.ErrEconomyPercent):
		return status.New(codes.InvalidArgument, user.ErrEconomyPercent.Error()), true
	case errors.Is(err, user.ErrVoucherUnlimitedVotes):
		return status.New(codes.InvalidArgument, user.ErrVoucherUnlimitedVotes.Error()), true
	case errors.Is(err, user.ErrTransferSelf):
		return status.New(codes.InvalidArgument, user.ErrTransferSelf.Error()), true
	case errors.Is(err, user.ErrTransferRefused):
		return status.New(codes.PermissionDenied, user.ErrTransferRefused.Error()), true
	case errors.Is(err, user.ErrVoucherInvalid):
		return status.New(codes.FailedPrecondition, user.ErrVoucherInvalid.Error()), true
	case errors.Is(err, user.ErrVoucherExists):
		return status.New(codes.AlreadyExists, user.ErrVoucherExists.Error()), true
	case errors.Is(err, user.ErrVoucherEmpty):
		return status.New(codes.InvalidArgument, user.ErrVoucherEmpty.Error()), true
	case errors.Is(err, user.ErrVoucherAmountInvalid):
		return status.New(codes.InvalidArgument, user.ErrVoucherAmountInvalid.Error()), true
	case errors.Is(err, user.ErrVoucherUsagesInvalid):
		return status.New(codes.InvalidArgument, user.ErrVoucherUsagesInvalid.Error()), true
	case errors.Is(err, user.ErrBanUntilPast):
		return status.New(codes.InvalidArgument, user.ErrBanUntilPast.Error()), true
	case errors.Is(err, user.ErrBanSelf):
		return status.New(codes.FailedPrecondition, user.ErrBanSelf.Error()), true
	case errors.Is(err, user.ErrBanAdmin):
		return status.New(codes.FailedPrecondition, user.ErrBanAdmin.Error()), true
	case errors.Is(err, user.ErrAdNotFound):
		return status.New(codes.NotFound, user.ErrAdNotFound.Error()), true
	case errors.Is(err, user.ErrTooManyAds):
		return status.New(codes.FailedPrecondition, user.ErrTooManyAds.Error()), true
	case errors.Is(err, user.ErrNotAdmin):
		return status.New(codes.PermissionDenied, user.ErrNotAdmin.Error()), true
	case errors.Is(err, user.ErrDeactivated):
		return status.New(codes.PermissionDenied, user.ErrDeactivated.Error()), true
	case errors.Is(err, user.ErrNotDeactivated):
		return status.New(codes.FailedPrecondition, user.ErrNotDeactivated.Error()), true
	case errors.Is(err, user.ErrRestoreExpired):
		return status.New(codes.FailedPrecondition, user.ErrRestoreExpired.Error()), true
	case errors.Is(err, user.ErrPartnerNotFound):
		return status.New(codes.InvalidArgument, user.ErrPartnerNotFound.Error()), true
	case errors.Is(err, user.ErrPartnerIsSelf):
		return status.New(codes.InvalidArgument, user.ErrPartnerIsSelf.Error()), true
	case errors.Is(err, user.ErrTooManyExtraFields):
		return status.New(codes.InvalidArgument, user.ErrTooManyExtraFields.Error()), true
	default:
		return nil, false
	}
}

// firmInvalid, firmMissing and firmRefused are the errors of the
// enterprises the client is told as they are.
var (
	firmInvalid = []error{
		user.ErrUnknownFirm, user.ErrUnknownGood, user.ErrUnknownFirmAction, user.ErrFirmName, user.ErrFirmStaff,
		user.ErrFirmGood, user.ErrFirmUnits, user.ErrFirmVotes, user.ErrFirmAuto, user.ErrOrder, user.ErrStockOrder,
		user.ErrStockBand, user.ErrRoute, user.ErrIssue, user.ErrDeliver, user.ErrTender, user.ErrFirmPlans, user.ErrFirmPlan, user.ErrGoodPlan, user.ErrFirmPercents, user.ErrFirmCapital, user.ErrFirmLimits,
	}
	firmMissing = []error{
		user.ErrFirmNotFound, user.ErrOrderNotFound, user.ErrStockOrderNotFound, user.ErrContractNotFound, user.ErrRouteNotFound,
		user.ErrTenderNotFound,
	}
	firmRefused = []error{
		user.ErrNotDirector, user.ErrTooManyFirms, user.ErrFirmTopLevel, user.ErrFirmCash, user.ErrFirmStock,
		user.ErrFirmStorage, user.ErrFirmNotYet, user.ErrFirmNoDeal, user.ErrFirmShortCash, user.ErrOwnOrder,
		user.ErrTooManyOrders, user.ErrFirmWorn, user.ErrImprovement, user.ErrStockShares, user.ErrAccount,
		user.ErrStockNoDeal, user.ErrOwnStockOrder, user.ErrTooManyStockOrders, user.ErrContractTaken,
		user.ErrTooManyContracts, user.ErrOwnTender, user.ErrTooManyTenders, user.ErrTenderToYourself, user.ErrTooManyRoutes,
	}
)

// sentinel is the one of known that err wraps, or nil: its text, not the
// chain of calls around it, is what the client may read.
func sentinel(err error, known []error) error {
	for _, k := range known {
		if errors.Is(err, k) {
			return k
		}
	}

	return nil
}

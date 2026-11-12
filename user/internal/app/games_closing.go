package app

import (
	"context"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
)

func (a *App) closeGames(ctx context.Context) error {
	if a.closingMode == "" {
		return nil
	}
	report, err := a.closing.Close(ctx, a.closingMode == "run", time.Now)
	if err != nil {
		return errs.Wrap("a.closing.Close", err)
	}

	a.logger.InfoContext(ctx, "game settlement", "mode", a.closingMode,
		"users", len(report.Accounts), "votes", report.Total, "by_reason", report.ByReason)
	for _, account := range report.Accounts[:min(10, len(report.Accounts))] {
		a.logger.InfoContext(ctx, "game settlement account", "mode", a.closingMode,
			"user_id", account.UserID, "votes", account.Total)
	}

	return nil
}

func (a *App) withGamesOpen(ctx context.Context, fn func(context.Context) error) error {
	if a.closingStore == nil {
		return fn(ctx)
	}
	return errs.Wrap("a.closingStore.WhileOpen", a.closingStore.WhileOpen(ctx, fn))
}

func (a *App) gameRequests(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if !isGameMethod(info.FullMethod) {
		return handler(ctx, req)
	}

	var reply any
	err := a.withGamesOpen(ctx, func(ctx context.Context) error {
		var err error
		reply, err = handler(ctx, req)
		return err
	})

	return reply, errs.Wrap("a.withGamesOpen", err)
}

func isGameMethod(method string) bool {
	switch method {
	case
		userv1.UserService_ActHoldemTable_FullMethodName,
		userv1.UserService_AdoptPet_FullMethodName,
		userv1.UserService_BackHoldemTable_FullMethodName,
		userv1.UserService_BetCasino_FullMethodName,
		userv1.UserService_BuyPetItem_FullMethodName,
		userv1.UserService_CallPoker_FullMethodName,
		userv1.UserService_CancelBetEvent_FullMethodName,
		userv1.UserService_CashRocket_FullMethodName,
		userv1.UserService_ClaimNamedPet_FullMethodName,
		userv1.UserService_CloseBetEvent_FullMethodName,
		userv1.UserService_CreateBetEvent_FullMethodName,
		userv1.UserService_CreateHoldemTable_FullMethodName,
		userv1.UserService_DealPoker_FullMethodName,
		userv1.UserService_DeleteBetEvent_FullMethodName,
		userv1.UserService_DigOre_FullMethodName,
		userv1.UserService_DressPet_FullMethodName,
		userv1.UserService_FeedPet_FullMethodName,
		userv1.UserService_FightPet_FullMethodName,
		userv1.UserService_FoldPoker_FullMethodName,
		userv1.UserService_GetCasino_FullMethodName,
		userv1.UserService_GetCasinoSettings_FullMethodName,
		userv1.UserService_GetFirmSettings_FullMethodName,
		userv1.UserService_GetFirms_FullMethodName,
		userv1.UserService_GetHoldemTable_FullMethodName,
		userv1.UserService_GetMine_FullMethodName,
		userv1.UserService_GetMineSettings_FullMethodName,
		userv1.UserService_GetNamedPet_FullMethodName,
		userv1.UserService_GetPet_FullMethodName,
		userv1.UserService_GetPetArena_FullMethodName,
		userv1.UserService_GetPetSettings_FullMethodName,
		userv1.UserService_GetPoker_FullMethodName,
		userv1.UserService_GetPokerSettings_FullMethodName,
		userv1.UserService_GetRocket_FullMethodName,
		userv1.UserService_GetStocks_FullMethodName,
		userv1.UserService_GetUserFirms_FullMethodName,
		userv1.UserService_GetUserPet_FullMethodName,
		userv1.UserService_InviteHoldemTable_FullMethodName,
		userv1.UserService_LaunchRocket_FullMethodName,
		userv1.UserService_LeaveHoldemTable_FullMethodName,
		userv1.UserService_LeavePetDrop_FullMethodName,
		userv1.UserService_ListBetEvents_FullMethodName,
		userv1.UserService_ListHoldemTables_FullMethodName,
		userv1.UserService_ListPetFights_FullMethodName,
		userv1.UserService_ListTop_FullMethodName,
		userv1.UserService_MovePetBattle_FullMethodName,
		userv1.UserService_OpenPetCase_FullMethodName,
		userv1.UserService_PlayPet_FullMethodName,
		userv1.UserService_QuickHoldem_FullMethodName,
		userv1.UserService_ReleasePet_FullMethodName,
		userv1.UserService_RenamePet_FullMethodName,
		userv1.UserService_ResolveBetEvent_FullMethodName,
		userv1.UserService_RunFirms_FullMethodName,
		userv1.UserService_RunPetArena_FullMethodName,
		userv1.UserService_RunStocks_FullMethodName,
		userv1.UserService_SellOre_FullMethodName,
		userv1.UserService_SetBetEventOdds_FullMethodName,
		userv1.UserService_SetCasino_FullMethodName,
		userv1.UserService_SetCasinoBets_FullMethodName,
		userv1.UserService_SetCasinoJackpot_FullMethodName,
		userv1.UserService_SetCasinoRocket_FullMethodName,
		userv1.UserService_SetFirmSettings_FullMethodName,
		userv1.UserService_SetMine_FullMethodName,
		userv1.UserService_SetPetSettings_FullMethodName,
		userv1.UserService_SetPetStance_FullMethodName,
		userv1.UserService_SetPokerSettings_FullMethodName,
		userv1.UserService_SitHoldemTable_FullMethodName,
		userv1.UserService_SpinCasino_FullMethodName,
		userv1.UserService_StakeBet_FullMethodName,
		userv1.UserService_StartPetBattle_FullMethodName,
		userv1.UserService_TakePetDrop_FullMethodName:
		return true
	default:
		return false
	}
}

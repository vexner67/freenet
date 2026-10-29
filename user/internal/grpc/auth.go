package grpc

import (
	"github.com/vexner67/freenet/pkg/mtls"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor).
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		userv1.UserService_GetProfile_FullMethodName,
		userv1.UserService_GetProfiles_FullMethodName,
		userv1.UserService_GetStatistics_FullMethodName,
		userv1.UserService_SearchUsers_FullMethodName,
		userv1.UserService_GetFollowStats_FullMethodName,
		userv1.UserService_ListFollowers_FullMethodName,
		userv1.UserService_ListFollowing_FullMethodName,
		userv1.UserService_GetAccess_FullMethodName,
		userv1.UserService_GetGiftCatalog_FullMethodName,
		userv1.UserService_GetVoteRate_FullMethodName,
		userv1.UserService_GetTreasury_FullMethodName,
		userv1.UserService_GetDecor_FullMethodName,
		userv1.UserService_GetPrime_FullMethodName,
		userv1.UserService_GetCasino_FullMethodName,
		userv1.UserService_GetPoker_FullMethodName,
		userv1.UserService_ListHoldemTables_FullMethodName,
		userv1.UserService_GetHoldemTable_FullMethodName,
		userv1.UserService_ListBetEvents_FullMethodName,
		userv1.UserService_ListAds_FullMethodName,
		userv1.UserService_GetStickerCatalog_FullMethodName,
		userv1.UserService_GetStickerAuthors_FullMethodName,
		userv1.UserService_ListGifts_FullMethodName,
		userv1.UserService_GetCollectible_FullMethodName,
		userv1.UserService_ListGiftMarket_FullMethodName,
		userv1.UserService_ListGiftAuctions_FullMethodName,
		userv1.UserService_FindNicknames_FullMethodName,
		userv1.UserService_ListTop_FullMethodName,
		userv1.UserService_GetUserPet_FullMethodName,
		userv1.UserService_GetCanvas_FullMethodName,
		userv1.UserService_GetCanvasCell_FullMethodName,
		userv1.UserService_GetUserFirms_FullMethodName,
		userv1.UserService_GetUserTitles_FullMethodName,
		userv1.UserService_GetOrCreateByEmail_FullMethodName,
		userv1.UserService_GetAccountState_FullMethodName,
	}
}

// DeactivatedMethods take the access token of a deleted page: its owner
// sees and restores it (see jwtauth.Access).
func DeactivatedMethods() []string {
	return []string{
		userv1.UserService_GetMe_FullMethodName,
		userv1.UserService_ReactivateAccount_FullMethodName,
	}
}

// BannedMethods take the access token of a page the administrators
// blocked: its owner reads why (see jwtauth.Access).
func BannedMethods() []string {
	return []string{
		userv1.UserService_GetMe_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		userv1.UserService_GetOrCreateByEmail_FullMethodName: {mtls.Auth},
		userv1.UserService_GetAccountState_FullMethodName:    {mtls.Auth},
		userv1.UserService_BuyPost_FullMethodName:            {mtls.Gateway},
		// A refund after a message that was sent would make it free: only
		// the gateway, which sends it, tells whether it was.
		userv1.UserService_PayMessage_FullMethodName:    {mtls.Gateway},
		userv1.UserService_RefundMessage_FullMethodName: {mtls.Gateway},
	}
}

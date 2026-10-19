package grpc

import (
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). A club's page, its
// members and the names of clubs are public, as a user's profile is; the
// internal methods carry no user token: other services call them, checked
// by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		clubv1.ClubService_GetClub_FullMethodName,
		clubv1.ClubService_GetClubs_FullMethodName,
		clubv1.ClubService_ListMembers_FullMethodName,
		clubv1.ClubService_ListUserClubs_FullMethodName,
		clubv1.ClubService_SearchClubs_FullMethodName,
		clubv1.ClubService_CountClubs_FullMethodName,
		clubv1.ClubService_CanModerateWall_FullMethodName,
		clubv1.ClubService_ListWallModerators_FullMethodName,
		clubv1.ClubService_ListHiddenManagers_FullMethodName,
		clubv1.ClubService_PurgeUser_FullMethodName,
		clubv1.ClubService_SetClubVerified_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		clubv1.ClubService_CanModerateWall_FullMethodName:    {mtls.Post},
		clubv1.ClubService_ListWallModerators_FullMethodName: {mtls.Post},
		clubv1.ClubService_ListHiddenManagers_FullMethodName: {mtls.Gateway},
		clubv1.ClubService_PurgeUser_FullMethodName:          {mtls.User},
		clubv1.ClubService_SetClubVerified_FullMethodName:    {mtls.Gateway},
	}
}

package grpc

import (
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The internal methods
// carry no user token: other services call them, checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		notificationv1.NotificationService_Notify_FullMethodName,
		notificationv1.NotificationService_Retract_FullMethodName,
		notificationv1.NotificationService_ForgetPost_FullMethodName,
		notificationv1.NotificationService_ForgetComment_FullMethodName,
		notificationv1.NotificationService_PurgeUser_FullMethodName,
		notificationv1.NotificationService_PushMessage_FullMethodName,
		notificationv1.NotificationService_GetPushKey_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		notificationv1.NotificationService_Notify_FullMethodName:        {mtls.Post, mtls.User, mtls.Club},
		notificationv1.NotificationService_Retract_FullMethodName:       {mtls.Post, mtls.User, mtls.Club},
		notificationv1.NotificationService_ForgetPost_FullMethodName:    {mtls.Post},
		notificationv1.NotificationService_ForgetComment_FullMethodName: {mtls.Post},
		notificationv1.NotificationService_PurgeUser_FullMethodName:     {mtls.User},
		notificationv1.NotificationService_PushMessage_FullMethodName:   {mtls.Message},
	}
}

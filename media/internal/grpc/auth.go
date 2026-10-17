package grpc

import (
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The internal methods
// carry no user token: other services call them, checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		mediav1.MediaService_GetAvatar_FullMethodName,
		mediav1.MediaService_GetPhoto_FullMethodName,
		mediav1.MediaService_PublishPhotos_FullMethodName,
		mediav1.MediaService_DeletePublishedPhotos_FullMethodName,
		mediav1.MediaService_CopyPhotos_FullMethodName,
		mediav1.MediaService_PurgeUser_FullMethodName,
		mediav1.MediaService_GetDocument_FullMethodName,
		mediav1.MediaService_DeleteDocuments_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		mediav1.MediaService_PublishPhotos_FullMethodName:         {mtls.Post, mtls.Message, mtls.Gateway},
		mediav1.MediaService_DeletePublishedPhotos_FullMethodName: {mtls.Post, mtls.Message, mtls.Gateway},
		mediav1.MediaService_CopyPhotos_FullMethodName:            {mtls.Post, mtls.Message},
		mediav1.MediaService_PurgeUser_FullMethodName:             {mtls.User, mtls.Post},
		mediav1.MediaService_UploadClubAvatar_FullMethodName:      {mtls.Gateway},
		mediav1.MediaService_DeleteClubAvatars_FullMethodName:     {mtls.Gateway},
		mediav1.MediaService_UploadChatAvatar_FullMethodName:      {mtls.Gateway},
		mediav1.MediaService_DeleteChatAvatars_FullMethodName:     {mtls.Gateway},
		mediav1.MediaService_UploadClubPhoto_FullMethodName:       {mtls.Gateway},
		mediav1.MediaService_DeleteClubPhotos_FullMethodName:      {mtls.Gateway},
		mediav1.MediaService_DeleteDocuments_FullMethodName:       {mtls.Post, mtls.Gateway},
	}
}
